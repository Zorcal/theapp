// Package pgdb provides utility functions for interacting with the PostgreSQL
// database.
package pgdb

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a new pgx connection pool using the given pgxpool.Config.
// The caller is responsible for closing the returned pool. Returns an error
// if the pool cannot be created.
func NewPool(ctx context.Context, cfg *pgxpool.Config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect to pool: %w", err)
	}
	return pool, nil
}

// StatusCheck checks whether the database is reachable and responsive.
// It first attempts a Ping, retrying with exponential backoff (100ms × attempts)
// until the context is done. Then it executes a simple `SELECT TRUE` query
// to force a round-trip to the database.
// Returns nil if the database is reachable and responsive, or a non-nil
// error otherwise. If ctx has no deadline, a 1-second timeout is used.
func StatusCheck(ctx context.Context, db *pgxpool.Pool) error {
	// If the caller doesn't give us a deadline set 1 second.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Second)
		defer cancel()
	}

	for attempts := 1; ; attempts++ {
		if err := db.Ping(ctx); err == nil {
			break
		}

		time.Sleep(time.Duration(attempts) * 100 * time.Millisecond)

		if ctx.Err() != nil {
			return fmt.Errorf("context error: %w", ctx.Err())
		}
	}

	if ctx.Err() != nil {
		return fmt.Errorf("context error: %w", ctx.Err())
	}

	// Run a simple query to determine connectivity.
	// Running this query forces a round trip through the database.
	const q = `SELECT TRUE`
	var tmp bool
	if err := db.QueryRow(ctx, q).Scan(&tmp); err != nil {
		return fmt.Errorf("query row: %w", err)
	}

	return nil
}

// ConnStr builds a PostgreSQL connection string from the given parameters.
func ConnStr(host string, port int, user, password, dbName string, sslEnabled bool) string {
	sslMode := "require"
	if !sslEnabled {
		sslMode = "disable"
	}
	return fmt.Sprintf(
		"postgres://%s:%s@%s/%s?&sslmode=%s",
		user,
		password,
		net.JoinHostPort(host, strconv.Itoa(port)),
		dbName,
		sslMode,
	)
}
