package market

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"
)

type stock struct {
	id   string
	name string
	min  int
	max  int
}

func RunStock(ctx context.Context) error {
	stockID := os.Getenv("STOCK_ID")
	if stockID == "" {
		return fmt.Errorf("STOCK_ID must be configured")
	}

	deps, err := connect(ctx)
	if err != nil {
		return fmt.Errorf("connect to dependencies: %w", err)
	}
	defer deps.close()

	item, err := loadStock(ctx, deps.db, stockID)
	if err != nil {
		return err
	}
	if err := validateStockRange(item); err != nil {
		return err
	}
	log.Printf("starting price ticks for %s (%s), range %d-%d", item.id, item.name, item.min, item.max)

	tick := func() {
		item, err := loadStock(ctx, deps.db, stockID)
		if err != nil {
			log.Printf("reload %s price range: %v", stockID, err)
			return
		}
		if err := validateStockRange(item); err != nil {
			log.Printf("reload %s price range: %v", stockID, err)
			return
		}
		price := item.min + rand.Intn(item.max-item.min+1)
		timestamp := time.Now().UTC()
		if _, err := deps.db.ExecContext(ctx,
			"INSERT INTO s_price (s_id, s_ttime, s_price) VALUES ($1, $2, $3)",
			item.id, timestamp, price); err != nil {
			log.Printf("store %s tick in PostgreSQL: %v", item.id, err)
			return
		}

		key := stockPriceKey(item.id)
		if err := deps.redis.HSet(ctx, key,
			"price", strconv.Itoa(price),
			"timestamp", timestamp.Format(time.RFC3339Nano),
		).Err(); err != nil {
			log.Printf("store %s tick in Redis: %v", item.id, err)
			return
		}
		log.Printf("%s price=%d timestamp=%s", item.id, price, timestamp.Format(time.RFC3339Nano))
	}

	tick()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			tick()
		}
	}
}

func loadStock(ctx context.Context, db *sql.DB, stockID string) (stock, error) {
	var item stock
	err := db.QueryRowContext(ctx,
		"SELECT s_id, s_name, s_min, s_max FROM s_detail WHERE s_id = $1",
		stockID,
	).Scan(&item.id, &item.name, &item.min, &item.max)
	if err != nil {
		return stock{}, fmt.Errorf("load stock %q from s_detail: %w", stockID, err)
	}
	return item, nil
}

func validateStockRange(item stock) error {
	if item.min < 0 || item.max < 0 || item.min > item.max {
		return fmt.Errorf("invalid price range for %s: minimum %d, maximum %d", item.id, item.min, item.max)
	}
	return nil
}
