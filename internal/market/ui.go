package market

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

const chartPointLimit = 120

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

func RunUI(ctx context.Context) error {
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
		timer := time.NewTimer(dependencyRetryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
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

	server := &http.Server{
		Addr:              ":" + envOrDefault("UI_PORT", "8080"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Bharat Index web console listening on %s", server.Addr)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down UI server: %w", err)
		}
		return nil
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve UI: %w", err)
	}
}

func readIndexSnapshot(ctx context.Context, db *sql.DB) (indexSnapshot, error) {
	var snapshot indexSnapshot
	if err := db.QueryRowContext(ctx,
		"SELECT i_id, i_name FROM i_detail ORDER BY i_id LIMIT 1",
	).Scan(&snapshot.ID, &snapshot.Name); err != nil {
		return indexSnapshot{}, fmt.Errorf("load index details: %w", err)
	}

	rows, err := db.QueryContext(ctx,
		`SELECT i_ttime, i_price
		 FROM i_price
		 WHERE i_id = $1
		 ORDER BY i_ttime DESC
		 LIMIT $2`,
		snapshot.ID, chartPointLimit,
	)
	if err != nil {
		return indexSnapshot{}, fmt.Errorf("query index prices: %w", err)
	}
	defer rows.Close()

	snapshot.Prices = make([]indexPoint, 0, chartPointLimit)
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
