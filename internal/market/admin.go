package market

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"time"
)

const maxAdminRequestSize = 1024

//go:embed admin.html
var adminFiles embed.FS

type adminStock struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Min  int64  `json:"min"`
	Max  int64  `json:"max"`
}

type updateStockRangeRequest struct {
	Min int64 `json:"min"`
	Max int64 `json:"max"`
}

func RunAdmin(ctx context.Context) error {
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
	mux.HandleFunc("GET /api/stocks", func(w http.ResponseWriter, r *http.Request) {
		stocks, err := readStocks(r.Context(), db)
		if err != nil {
			log.Printf("read stock ranges: %v", err)
			http.Error(w, "Unable to load stock ranges", http.StatusInternalServerError)
			return
		}
		writeJSON(w, stocks)
	})
	mux.HandleFunc("PUT /api/stocks/{id}/range", func(w http.ResponseWriter, r *http.Request) {
		var request updateStockRangeRequest
		r.Body = http.MaxBytesReader(w, r.Body, maxAdminRequestSize)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, "Provide valid JSON with integer min and max values", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			http.Error(w, "Request body must contain one JSON object", http.StatusBadRequest)
			return
		}
		if request.Min < 0 || request.Max < 0 || request.Min > request.Max || request.Max > math.MaxInt32 {
			http.Error(w, "Price range must satisfy 0 <= min <= max <= 2147483647", http.StatusBadRequest)
			return
		}

		stock, err := updateStockRange(r.Context(), db, r.PathValue("id"), request)
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Stock not found", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("update stock %q range: %v", r.PathValue("id"), err)
			http.Error(w, "Unable to update stock range", http.StatusInternalServerError)
			return
		}
		writeJSON(w, stock)
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, adminFiles, "admin.html")
	})

	server := &http.Server{
		Addr:              ":" + envOrDefault("ADMIN_PORT", "8081"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Bharat Index admin console listening on %s", server.Addr)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down admin server: %w", err)
		}
		return nil
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve admin console: %w", err)
	}
}

func readStocks(ctx context.Context, db *sql.DB) ([]adminStock, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT s_id, s_name, s_min, s_max FROM s_detail ORDER BY s_name, s_id",
	)
	if err != nil {
		return nil, fmt.Errorf("query stock ranges: %w", err)
	}
	defer rows.Close()

	stocks := make([]adminStock, 0)
	for rows.Next() {
		var item adminStock
		if err := rows.Scan(&item.ID, &item.Name, &item.Min, &item.Max); err != nil {
			return nil, fmt.Errorf("read stock range: %w", err)
		}
		stocks = append(stocks, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stock ranges: %w", err)
	}
	return stocks, nil
}

func updateStockRange(ctx context.Context, db *sql.DB, stockID string, request updateStockRangeRequest) (adminStock, error) {
	var item adminStock
	err := db.QueryRowContext(ctx,
		`UPDATE s_detail
		 SET s_min = $1, s_max = $2
		 WHERE s_id = $3
		 RETURNING s_id, s_name, s_min, s_max`,
		request.Min, request.Max, stockID,
	).Scan(&item.ID, &item.Name, &item.Min, &item.Max)
	if err != nil {
		return adminStock{}, fmt.Errorf("update s_detail: %w", err)
	}
	return item, nil
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}
