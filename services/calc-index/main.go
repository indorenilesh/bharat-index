package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

const (
	retryInterval    = 2 * time.Second
	defaultStockIDs  = "BT001,TT001"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	stockIDs := configuredStockIDs()
	if len(stockIDs) == 0 {
		return fmt.Errorf("STOCK_IDS must contain at least one stock ID")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := openPostgres()
	if err != nil {
		return err
	}
	defer closeDatabase(db)
	client := redis.NewClient(&redis.Options{Addr: envOrDefault("REDIS_ADDR", "redis:6379")})
	defer closeRedis(client)

	for {
		if err := db.PingContext(ctx); err == nil {
			if err := client.Ping(ctx).Err(); err == nil {
				break
			} else {
				log.Printf("waiting for Redis: %v", err)
			}
		} else {
			log.Printf("waiting for PostgreSQL: %v", err)
		}
		if !wait(ctx, retryInterval) {
			return nil
		}
	}

	var indexID string
	if err := db.QueryRowContext(ctx,
		"SELECT i_id FROM i_detail ORDER BY i_id LIMIT 1",
	).Scan(&indexID); err != nil {
		return fmt.Errorf("load index from i_detail: %w", err)
	}
	log.Printf("calculating index %s from latest prices for %s", indexID, strings.Join(stockIDs, ", "))

	calculate(ctx, db, client, indexID, stockIDs)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			calculate(ctx, db, client, indexID, stockIDs)
		}
	}
}

func calculate(ctx context.Context, db *sql.DB, client *redis.Client, indexID string, stockIDs []string) {
	var total float64
	for _, stockID := range stockIDs {
		fields, err := client.HGetAll(ctx, "stock:"+stockID+":latest").Result()
		if err != nil {
			log.Printf("read latest %s price from Redis: %v", stockID, err)
			return
		}
		priceText, ok := fields["price"]
		if !ok {
			log.Printf("no price tick yet for %s; skipping index tick", stockID)
			return
		}
		price, err := strconv.ParseFloat(priceText, 64)
		if err != nil {
			log.Printf("invalid latest %s price %q in Redis: %v", stockID, priceText, err)
			return
		}
		total += price
	}

	indexPrice := math.Round(total/float64(len(stockIDs))*100) / 100
	timestamp := time.Now().UTC()
	if _, err := db.ExecContext(ctx,
		"INSERT INTO i_price (i_id, i_ttime, i_price) VALUES ($1, $2, $3)",
		indexID, timestamp, indexPrice); err != nil {
		log.Printf("store index tick in PostgreSQL: %v", err)
		return
	}
	if err := client.HSet(ctx, "index:"+indexID+":latest",
		"price", strconv.FormatFloat(indexPrice, 'f', 2, 64),
		"timestamp", timestamp.Format(time.RFC3339Nano),
	).Err(); err != nil {
		log.Printf("store index tick in Redis: %v", err)
		return
	}
	log.Printf("%s index=%.2f timestamp=%s", indexID, indexPrice, timestamp.Format(time.RFC3339Nano))
}

func configuredStockIDs() []string {
	value := envOrDefault("STOCK_IDS", defaultStockIDs)
	stockIDs := make([]string, 0)
	for _, part := range strings.Split(value, ",") {
		if stockID := strings.TrimSpace(part); stockID != "" {
			stockIDs = append(stockIDs, stockID)
		}
	}
	return stockIDs
}

func openPostgres() (*sql.DB, error) {
	dbName := envOrDefault("PGDATABASE", envOrDefault("DB_ENV_PREFIX", "dev")+"-bharat-index")
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

func closeDatabase(db *sql.DB) {
	if err := db.Close(); err != nil {
		log.Printf("close PostgreSQL connection: %v", err)
	}
}

func closeRedis(client *redis.Client) {
	if err := client.Close(); err != nil {
		log.Printf("close Redis connection: %v", err)
	}
}
