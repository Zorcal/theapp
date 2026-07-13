package pgschema_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/data/pgschema"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
)

func TestSeed(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.NewWithoutSeed(t, ctx)

	if err := pgschema.Seed(ctx, pool); err != nil {
		t.Fatalf("Seed() first call error = %v", err)
	}
	before := totalRowCount(t, ctx, pool)

	if err := pgschema.Seed(ctx, pool); err != nil {
		t.Fatalf("Seed() second call error = %v", err)
	}
	after := totalRowCount(t, ctx, pool)

	if after != before {
		t.Errorf("total row count after second Seed() = %d, want %d (unchanged)", after, before)
	}
}

// totalRowCount sums the row count of every table in the database, discovered dynamically rather
// than by name, so it stays a valid idempotency check regardless of what seed.sql seeds.
func totalRowCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int {
	t.Helper()

	rows, err := pool.Query(ctx, `
		SELECT table_schema, table_name
		FROM information_schema.tables
		WHERE table_type = 'BASE TABLE' AND table_schema NOT IN ('pg_catalog', 'information_schema')`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()

	type table struct{ schema, name string }
	var tables []table
	for rows.Next() {
		var tbl table
		if err := rows.Scan(&tbl.schema, &tbl.name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		tables = append(tables, tbl)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("list tables: %v", err)
	}

	var total int
	for _, tbl := range tables {
		var count int
		sql := fmt.Sprintf("SELECT COUNT(*) FROM %q.%q", tbl.schema, tbl.name)
		if err := pool.QueryRow(ctx, sql).Scan(&count); err != nil {
			t.Fatalf("count rows in %s.%s: %v", tbl.schema, tbl.name, err)
		}
		total += count
	}
	return total
}
