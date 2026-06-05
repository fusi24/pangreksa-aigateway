// Package repository provides PostgreSQL data access for the Central Server.
// All repositories use a shared pgxpool.Pool for connection management.
// Migrations are executed automatically on startup using golang-migrate.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	// blank import: registers the "postgres" database driver for golang-migrate.
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	// blank import: registers the "file://" source driver for golang-migrate.
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool.Pool and provides migration support.
// All repositories in this package accept a *DB and use its Pool directly.
//
// Thread-safety: safe for concurrent use; pgxpool.Pool is goroutine-safe.
type DB struct {
	// Pool is the underlying pgx connection pool.
	// Repositories access it directly for query execution.
	Pool *pgxpool.Pool
}

// New creates a DB by connecting to PostgreSQL using databaseURL, then running
// all pending migrations found under migrationsDir.
//
// ctx controls the initial connection attempt timeout.
// databaseURL must be a valid libpq-compatible DSN, e.g.:
//
//	postgres://user:pass@host:5432/dbname?sslmode=disable
//
// migrationsDir is the path to the directory containing *.up.sql and *.down.sql
// migration files, e.g. "migrations" or "/app/migrations".
//
// Returns an error if the pool cannot be established or migrations fail.
func New(ctx context.Context, databaseURL, migrationsDir string) (*DB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("repository.New: create pgxpool: %w", err)
	}

	// verify connectivity before running migrations.
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("repository.New: ping database: %w", err)
	}

	db := &DB{Pool: pool}

	if err = runMigrations(databaseURL, migrationsDir); err != nil {
		pool.Close()
		return nil, fmt.Errorf("repository.New: run migrations: %w", err)
	}

	return db, nil
}

// Close shuts down the underlying connection pool.
// It is safe to call Close multiple times; subsequent calls are no-ops.
func (db *DB) Close() {
	db.Pool.Close()
}

// runMigrations applies all pending up-migrations from the given directory.
// It uses golang-migrate with the postgres database driver and file source.
// If migrations are already at the latest version, the function returns nil.
func runMigrations(databaseURL, migrationsDir string) error {
	sourceURL := fmt.Sprintf("file://%s", migrationsDir)

	m, err := migrate.New(sourceURL, databaseURL)
	if err != nil {
		return fmt.Errorf("runMigrations: create migrate instance: %w", err)
	}
	defer m.Close()

	if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("runMigrations: apply migrations: %w", err)
	}

	return nil
}
