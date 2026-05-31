package pgtest

import (
	"context"
	"testing"
)

func TestNew(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for _, name := range []string{"first", "second"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			pool := New(t, ctx)

			var exists bool
			const sql = `SELECT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = 'theapp')`
			if err := pool.QueryRow(ctx, sql).Scan(&exists); err != nil {
				t.Fatalf("query schema: %v", err)
			}
			if !exists {
				t.Fatal("theapp schema not found in cloned database")
			}
		})
	}
}

func TestNewWithoutTemplate(t *testing.T) {
	ctx := context.Background()

	pool := NewWithoutTemplate(t, ctx)

	var exists bool
	const sql = `SELECT EXISTS (SELECT 1 FROM information_schema.schemata WHERE schema_name = 'theapp')`
	if err := pool.QueryRow(ctx, sql).Scan(&exists); err != nil {
		t.Fatalf("query schema: %v", err)
	}
	if !exists {
		t.Fatal("theapp schema not found in database built from migrations")
	}
}
