package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

const retryInterval = 2 * time.Second

type stock struct {
	id   string
	name string
	min  int
	max  int
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	stockID := os.Getenv("STOCK_ID")
	if stockID == "" {
		return fmt.Errorf("STOCK_ID must be configured")
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

	tick(ctx, db, client, stockID)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			tick(ctx, db, client, stockID)
		}
	}
}

func tick(ctx context.Context, db *sql.DB, client *redis.Client, stockID string) {
	item, err := loadStock(ctx, db, stockID)
	if err != nil {
		log.Printf("load %s price range: %v", stockID, err)
		return
	}
	if item.min < 0 || item.max < item.min {
		log.Printf("invalid price range for %s: minimum %d, maximum %d", item.id, item.min, item.max)
		return
	}

	price := item.min + rand.Intn(item.max-item.min+1)
	timestamp := time.Now().UTC()
	if _, err := db.ExecContext(ctx,
		"INSERT INTO s_price (s_id, s_ttime, s_price) VALUES ($1, $2, $3)",
		item.id, timestamp, price); err != nil {
		log.Printf("store %s tick in PostgreSQL: %v", item.id, err)
		return
	}
	if err := client.HSet(ctx, "stock:"+item.id+":latest",
		"price", strconv.Itoa(price),
		"timestamp", timestamp.Format(time.RFC3339Nano),
	).Err(); err != nil {
		log.Printf("store %s tick in Redis: %v", item.id, err)
		return
	}
	log.Printf("%s (%s) price=%d timestamp=%s", item.id, item.name, price, timestamp.Format(time.RFC3339Nano))
}

func loadStock(ctx context.Context, db *sql.DB, stockID string) (stock, error) {
	var item stock
	err := db.QueryRowContext(ctx,
		"SELECT s_id, s_name, s_min, s_max FROM s_detail WHERE s_id = $1",
		stockID,
	).Scan(&item.id, &item.name, &item.min, &item.max)
	if err != nil {
		return stock{}, fmt.Errorf("read s_detail for %s: %w", stockID, err)
	}
	return item, nil
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
