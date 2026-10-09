package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	retryInterval = 2 * time.Second
	pointLimit    = 120
)

//go:embed ui.html
var uiFiles embed.FS

type indexSnapshot struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	Prices []indexPoint `json:"prices"`
}

type indexPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Price     float64   `json:"price"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := openPostgres()
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("close PostgreSQL connection: %v", err)
		}
	}()

	for {
		if err := db.PingContext(ctx); err == nil {
			break
		} else {
			log.Printf("waiting for PostgreSQL: %v", err)
		}
		if !wait(ctx, retryInterval) {
			return nil
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/index", func(w http.ResponseWriter, r *http.Request) {
		snapshot, err := readIndexSnapshot(r.Context(), db)
		if err != nil {
			log.Printf("read index snapshot: %v", err)
			http.Error(w, "Unable to load index prices", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(snapshot); err != nil {
			log.Printf("write index snapshot: %v", err)
		}
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, uiFiles, "ui.html")
	})

	return serve(ctx, envOrDefault("UI_PORT", "8080"), mux, "Bharat Index dashboard")
}

func readIndexSnapshot(ctx context.Context, db *sql.DB) (indexSnapshot, error) {
	var snapshot indexSnapshot
	if err := db.QueryRowContext(ctx,
		"SELECT i_id, i_name FROM i_detail ORDER BY i_id LIMIT 1",
	).Scan(&snapshot.ID, &snapshot.Name); err != nil {
		return indexSnapshot{}, fmt.Errorf("load index details: %w", err)
	}

	rows, err := db.QueryContext(ctx,
		`SELECT i_ttime, i_price FROM i_price
		 WHERE i_id = $1 ORDER BY i_ttime DESC LIMIT $2`,
		snapshot.ID, pointLimit,
	)
	if err != nil {
		return indexSnapshot{}, fmt.Errorf("query index prices: %w", err)
	}
	defer rows.Close()

	snapshot.Prices = make([]indexPoint, 0, pointLimit)
	for rows.Next() {
		var point indexPoint
		if err := rows.Scan(&point.Timestamp, &point.Price); err != nil {
			return indexSnapshot{}, fmt.Errorf("read index price: %w", err)
		}
		snapshot.Prices = append(snapshot.Prices, point)
	}
	if err := rows.Err(); err != nil {
		return indexSnapshot{}, fmt.Errorf("iterate index prices: %w", err)
	}
	for left, right := 0, len(snapshot.Prices)-1; left < right; left, right = left+1, right-1 {
		snapshot.Prices[left], snapshot.Prices[right] = snapshot.Prices[right], snapshot.Prices[left]
	}
	return snapshot, nil
}

func openPostgres() (*sql.DB, error) {
	dbName := os.Getenv("PGDATABASE")
	if dbName == "" {
		appEnv := os.Getenv("APP_ENV")
		if appEnv == "" {
			return nil, fmt.Errorf("PGDATABASE or APP_ENV must be configured")
		}
		dbName = appEnv + "-bharat-index"
	}
	address := net.JoinHostPort(envOrDefault("PGHOST", "postgres"), envOrDefault("PGPORT", "5432"))
	connectionURL := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(envOrDefault("PGUSER", "bharatindex"), os.Getenv("PGPASSWORD")),
		Host:   address,
		Path:   "/" + dbName,
	}
	query := connectionURL.Query()
	query.Set("sslmode", envOrDefault("PGSSLMODE", "disable"))
	connectionURL.RawQuery = query.Encode()
	db, err := sql.Open("pgx", connectionURL.String())
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL connection: %w", err)
	}
	return db, nil
}

func serve(ctx context.Context, port string, handler http.Handler, name string) error {
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("%s listening on %s", name, server.Addr)
		serverErr <- server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down %s: %w", name, err)
		}
		return nil
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve %s: %w", name, err)
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
