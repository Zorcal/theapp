// Package pgtest provides PostgreSQL test helpers. All database operations assume PostgreSQL is running via
// docker-compose.
package pgtest

import (
	"cmp"
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/schema"
)

// Static database configuration. See docker-compose.yml at repository root.
const (
	host       = "theapp-postgres"
	port       = 5432
	username   = "postgres"
	password   = "postgres"
	sslEnabled = false
)

// New creates a temporary test database with migrations applied and returns a
// connection pool to the database.
func New(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()

	dbName, teardown, err := setupDB(ctx)
	if err != nil {
		if err := teardown(); err != nil {
			t.Errorf("error tearing down db: %v", err)
		}
		t.Fatalf("error setting up db: %v", err)
	}
	t.Cleanup(func() {
		if err := teardown(); err != nil {
			t.Fatalf("error tearing down db: %v", err)
		}
	})

	if err := schema.Migrate(ctx, connStr(dbName)); err != nil {
		t.Fatalf("migrate database %s: %s", dbName, err)
	}

	poolCfg, err := poolConfig(dbName)
	if err != nil {
		t.Fatalf("create pool config: %v", err)
	}

	pool, err := pgdb.NewPool(ctx, poolCfg)
	if err != nil {
		t.Fatalf("create database pool %s: %s", dbName, err)
	}
	t.Cleanup(func() { pool.Close() })

	if err := pgdb.StatusCheck(ctx, pool); err != nil {
		t.Fatalf("status check database: %s", err)
	}

	return pool
}

func setupDB(ctx context.Context) (dbName string, teardown func() error, err error) {
	teardown = func() error {
		return nil
	}

	poolCfg, err := poolConfig("postgres")
	if err != nil {
		return "", teardown, fmt.Errorf("create pool config: %w", err)
	}

	pool, err := pgdb.NewPool(ctx, poolCfg)
	if err != nil {
		return "", teardown, fmt.Errorf("create database manager pool: %w", err)
	}

	teardown = func() error {
		pool.Close()
		return nil
	}

	if err := pgdb.StatusCheck(ctx, pool); err != nil {
		return "", teardown, fmt.Errorf(`status check database manager: %w

		Did you remember to run 'docker-compose up -d' at the repository root?
		`, err)
	}

	dbName = randDBName()

	if _, err := pool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %q", dbName)); err != nil {
		return "", teardown, fmt.Errorf("drop database %q: %w", dbName, err)
	}

	if _, err := pool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %q", dbName)); err != nil {
		return "", teardown, fmt.Errorf("create database %q: %w", dbName, err)
	}

	teardown = func() error {
		if _, err := pool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %q", dbName)); err != nil {
			pool.Close()
			return fmt.Errorf("teardown: drop database %q: %w", dbName, err)
		}
		pool.Close()
		return nil
	}

	return dbName, teardown, nil
}

func randDBName() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, len(alphabet))
	for i := range len(alphabet) {
		b[i] = alphabet[rand.Intn(len(alphabet))] //nolint:gosec // Weak randomness is fine for test database names
	}
	return string(b)
}

func poolConfig(dbName string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(connStr(dbName))
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

func connStr(dbName string) string {
	return pgdb.ConnStr(
		cmp.Or(os.Getenv("POSTGRES_HOST"), host),
		cmp.Or(parsePortFromEnv(), port),
		cmp.Or(os.Getenv("POSTGRES_USER"), username),
		cmp.Or(os.Getenv("POSTGRES_PASSWORD"), password),
		dbName,
		sslEnabled,
	)
}

func parsePortFromEnv() int {
	if p := os.Getenv("POSTGRES_PORT"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil {
			return parsed
		}
	}
	return 0
}
