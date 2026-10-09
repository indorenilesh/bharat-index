package market

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultStockIDs = "BT001,TT001"

func RunIndex(ctx context.Context) error {
	deps, err := connect(ctx)
	if err != nil {
		return fmt.Errorf("connect to dependencies: %w", err)
	}
	defer deps.close()

	indexID, err := loadIndexID(ctx, deps.db)
	if err != nil {
		return err
	}
	stockIDs := configuredStockIDs()
	if len(stockIDs) == 0 {
		return fmt.Errorf("STOCK_IDS must contain at least one stock ID")
	}
	log.Printf("calculating index %s from latest prices for %s", indexID, strings.Join(stockIDs, ", "))

	calculate := func() {
		prices := make([]float64, 0, len(stockIDs))
		for _, stockID := range stockIDs {
			fields, err := deps.redis.HGetAll(ctx, stockPriceKey(stockID)).Result()
			if err != nil {
				log.Printf("read latest %s price from Redis: %v", stockID, err)
				return
			}
			priceText, ok := fields["price"]
			if !ok {
				log.Printf("no price tick yet for %s; skipping this index tick", stockID)
				return
			}
			price, err := strconv.ParseFloat(priceText, 64)
			if err != nil {
				log.Printf("invalid latest %s price %q in Redis: %v", stockID, priceText, err)
				return
			}
			prices = append(prices, price)
		}

		var total float64
		for _, price := range prices {
			total += price
		}
		indexPrice := math.Round(total/float64(len(prices))*100) / 100
		timestamp := time.Now().UTC()
		if _, err := deps.db.ExecContext(ctx,
			"INSERT INTO i_price (i_id, i_ttime, i_price) VALUES ($1, $2, $3)",
			indexID, timestamp, indexPrice); err != nil {
			log.Printf("store index tick in PostgreSQL: %v", err)
			return
		}
		if err := deps.redis.HSet(ctx, indexPriceKey(indexID),
			"price", strconv.FormatFloat(indexPrice, 'f', 2, 64),
			"timestamp", timestamp.Format(time.RFC3339Nano),
		).Err(); err != nil {
			log.Printf("store index tick in Redis: %v", err)
			return
		}
		log.Printf("%s index=%0.2f timestamp=%s", indexID, indexPrice, timestamp.Format(time.RFC3339Nano))
	}

	calculate()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			calculate()
		}
	}
}

func loadIndexID(ctx context.Context, db *sql.DB) (string, error) {
	var indexID string
	if err := db.QueryRowContext(ctx,
		"SELECT i_id FROM i_detail ORDER BY i_id LIMIT 1",
	).Scan(&indexID); err != nil {
		return "", fmt.Errorf("load index from i_detail: %w", err)
	}
	return indexID, nil
}

func configuredStockIDs() []string {
	value := os.Getenv("STOCK_IDS")
	if value == "" {
		value = defaultStockIDs
	}

	parts := strings.Split(value, ",")
	stockIDs := make([]string, 0, len(parts))
	for _, part := range parts {
		if stockID := strings.TrimSpace(part); stockID != "" {
			stockIDs = append(stockIDs, stockID)
		}
	}
	return stockIDs
}

func stockPriceKey(stockID string) string {
	return "stock:" + stockID + ":latest"
}

func indexPriceKey(indexID string) string {
	return "index:" + indexID + ":latest"
}
