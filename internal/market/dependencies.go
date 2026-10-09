package market

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

const dependencyRetryInterval = 2 * time.Second

type dependencies struct {
	db    *sql.DB
	redis *redis.Client
}

func connect(ctx context.Context) (*dependencies, error) {
	db, err := openPostgres()
	if err != nil {
		return nil, err
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr: envOrDefault("REDIS_ADDR", "redis:6379"),
	})

	deps := &dependencies{db: db, redis: redisClient}
	for {
		if err := db.PingContext(ctx); err == nil {
			if err := redisClient.Ping(ctx).Err(); err == nil {
				return deps, nil
			} else {
				log.Printf("waiting for Redis: %v", err)
			}
		} else {
			log.Printf("waiting for PostgreSQL: %v", err)
		}

		timer := time.NewTimer(dependencyRetryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			deps.close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func openPostgres() (*sql.DB, error) {
	host := envOrDefault("PGHOST", "postgres")
	port := envOrDefault("PGPORT", "5432")
	user := envOrDefault("PGUSER", "bharatindex")
	password := os.Getenv("PGPASSWORD")
	database := envOrDefault("PGDATABASE", envOrDefault("DB_ENV_PREFIX", "dev")+"-bharat-index")

	dbURL := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + database,
	}
	db, err := sql.Open("pgx", dbURL.String()+"?sslmode=disable")
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL connection: %w", err)
	}
	return db, nil
}

func (d *dependencies) close() {
	if err := d.redis.Close(); err != nil {
		log.Printf("close Redis connection: %v", err)
	}
	if err := d.db.Close(); err != nil {
		log.Printf("close PostgreSQL connection: %v", err)
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
