package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"whatsappconverty/migrations"
)

// Migrate applies all pending migrations using goose.
//
// Every instance migrates on boot, and a deploy can have two of them starting at
// once, so the same migration can reach Postgres twice. Goose takes no lock
// itself, and a losing racer fails on the duplicate object. Rather than fail the
// boot, the duplicate is treated as "another instance got there first": wait
// briefly, then re-read the version and carry on with whatever is left.
func Migrate(ctx context.Context, databaseURL string, logger *slog.Logger) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open database for migrations: %w", err)
	}
	defer db.Close()

	goose.SetBaseFS(migrations.FS)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	const attempts = 5
	for attempt := 1; ; attempt++ {
		err = goose.UpContext(ctx, db, ".")
		if err == nil {
			break
		}
		if !isDuplicateObject(err) {
			return fmt.Errorf("run migrations: %w", err)
		}
		if attempt == attempts {
			return fmt.Errorf("run migrations: still colliding with another instance after %d attempts: %w", attempts, err)
		}
		logger.Warn("another instance is applying the same migration; waiting for it",
			"attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * time.Second):
		}
	}

	version, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return fmt.Errorf("get db version: %w", err)
	}
	logger.Info("migrations up to date", "version", version)
	return nil
}

// Postgres error codes for the object-duplication family. They are named here
// rather than pulled from a package so a migration collision can be recognised
// without a dependency.
const (
	pgDuplicateTable  = "42P07" // relation already exists
	pgDuplicateObject = "42710" // object already exists
	pgUniqueViolation = "23505" // the pg_type catalog race between two runners
)

// isDuplicateObject reports whether err is the error Postgres raises when a
// migration that another instance is applying at the same time lands on the same
// object: a table, an index or a type.
func isDuplicateObject(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.Code {
	case pgDuplicateTable, pgDuplicateObject, pgUniqueViolation:
		return true
	}
	return false
}
