package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"whatsappconverty/internal/config"
	"whatsappconverty/internal/converty"
	"whatsappconverty/internal/database"
	"whatsappconverty/internal/logutil"
)

func main() {
	lookupRef := flag.Int64("ref", 0, "when set, only print orders with this reference and touch nothing")
	flag.Parse()

	cfg := config.Load()
	logger := logutil.New(cfg)
	slog.SetDefault(logger)

	ctx := context.Background()
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	svc := converty.NewService(cfg, pool, logger)
	integrations, err := svc.IntegrationsAll(ctx)
	if err != nil {
		logger.Error("list integrations failed", "error", err)
		os.Exit(1)
	}

	updated, already, skipped := 0, 0, 0
	for _, integ := range integrations {
		err := svc.WithAccessToken(ctx, integ.ShopID, integ.ID, func(ctx context.Context, token string) error {
			orders, oErr := svc.ListOrders(ctx, token)
			if oErr != nil {
				return oErr
			}
			if *lookupRef > 0 {
				for _, o := range orders {
					if o.Reference == *lookupRef {
						logger.Info("order lookup", "reference", o.Reference,
							"order_id", o.ID, "barcode", o.Barcode, "status", o.Status,
							"customer_name", o.CustomerName, "customer_phone", o.CustomerPhone)
					}
				}
				return nil
			}
			for _, o := range orders {
				if o.Reference <= 0 || o.ID == "" {
					continue
				}
				ref := fmt.Sprintf("%d", o.Reference)
				tag, uErr := pool.Exec(ctx, `
					UPDATE delivery_orders
					SET order_id = $1, updated_at = now()
					WHERE shop_id = $2 AND (order_id = $3 OR barcode = $4) AND order_id <> $1`,
					ref, integ.ShopID, o.ID, o.Barcode)
				if uErr != nil {
					logger.Warn("update failed", "barcode", o.Barcode, "error", uErr)
					continue
				}
				if tag.RowsAffected() > 0 {
					updated++
					logger.Info("set order reference",
						"shop", integ.ShopID, "barcode", o.Barcode, "order_id", o.ID, "reference", ref)
				} else {
					already++
				}
			}
			return nil
		})
		if err != nil {
			skipped++
			logger.Warn("integration skipped", "integration_id", integ.ID, "error", err)
		}
	}
	logger.Info("backfill complete", "updated", updated, "already_correct", already, "integrations_skipped", skipped)
	if errors.Is(err, context.Canceled) {
		os.Exit(1)
	}
}