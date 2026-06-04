package server

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const migrationTableName = "schema_migrations"

//go:embed migrations/*.sql
var migrationFS embed.FS

// MigrateDSN applies the embedded core schema migrations to the target database.
func MigrateDSN(dsn string) error {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return err
	}
	return MigratePool(ctx, pool)
}

// MigratePool applies the embedded core schema migrations using the shared pool.
func MigratePool(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return nil
	}

	subFS, err := fs.Sub(migrationFS, "migrations")
	if err != nil {
		return err
	}

	db, err := sql.Open("pgx", pool.Config().ConnString())
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return err
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, db, subFS, goose.WithTableName(migrationTableName))
	if err != nil {
		return err
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply core migrations: %w", err)
	}
	return nil
}
