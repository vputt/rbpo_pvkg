//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testDSNEnvironment = "PVKG_TEST_DSN"

func TestInitialSchema(t *testing.T) {
	dsn := os.Getenv(testDSNEnvironment)
	if dsn == "" {
		t.Fatalf("%s is required and must point to a separate test database", testDSNEnvironment)
	}

	source, err := iofs.New(os.DirFS(migrationsPath(t)), ".")
	if err != nil {
		t.Fatalf("open migrations: %v", err)
	}
	migration, err := migrate.NewWithSourceInstance("iofs", source, migrationDatabaseURL(t, dsn))
	if err != nil {
		t.Fatalf("create migration runner: %v", err)
	}

	if err := migration.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("reset test database: %v", err)
	}
	if err := migration.Up(); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	t.Cleanup(func() {
		if err := migration.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			t.Errorf("roll back migrations: %v", err)
		}
		closeMigration(t, migration)
	})

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("create test database pool: %v", err)
	}
	defer pool.Close()

	var tableCount int
	err = pool.QueryRow(context.Background(), `
		SELECT count(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name IN ('users', 'sessions', 'items', 'claims', 'issuances')
	`).Scan(&tableCount)
	if err != nil {
		t.Fatalf("count application tables: %v", err)
	}
	if tableCount != 5 {
		t.Fatalf("application table count = %d, want 5", tableCount)
	}

	var indexExists bool
	err = pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1
			FROM pg_indexes
			WHERE schemaname = 'public'
			  AND indexname = 'claims_one_approved_per_item'
			  AND indexdef LIKE 'CREATE UNIQUE INDEX%'
			  AND indexdef LIKE '%WHERE%approved%'
		)
	`).Scan(&indexExists)
	if err != nil {
		t.Fatalf("check partial unique index: %v", err)
	}
	if !indexExists {
		t.Fatal("partial unique index claims_one_approved_per_item was not created")
	}
}

func migrationsPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate integration test file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations")
}

func migrationDatabaseURL(t *testing.T, dsn string) string {
	t.Helper()
	databaseURL, err := url.Parse(dsn)
	if err != nil {
		t.Fatal("parse test database connection string")
	}
	if databaseURL.Scheme != "postgres" && databaseURL.Scheme != "postgresql" {
		t.Fatalf("%s must use postgres or postgresql scheme", testDSNEnvironment)
	}
	databaseURL.Scheme = "pgx5"
	query := databaseURL.Query()
	query.Set("x-multi-statement", "true")
	databaseURL.RawQuery = query.Encode()
	return databaseURL.String()
}

func closeMigration(t *testing.T, migration *migrate.Migrate) {
	t.Helper()
	sourceError, databaseError := migration.Close()
	if sourceError != nil {
		t.Errorf("close migration source: %v", sourceError)
	}
	if databaseError != nil {
		t.Errorf("close migration database: %v", databaseError)
	}
}
