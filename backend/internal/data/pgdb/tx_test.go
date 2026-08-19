package pgdb_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
)

func TestRunTx(t *testing.T) {
	sentinel := errors.New("sentinel")

	t.Run("commits on success", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		setupTxTable(t, ctx, pool)

		doInTx := func(ctx context.Context) error {
			return insertInTx(t, ctx)
		}
		if err := pgdb.RunTx(ctx, pool, doInTx); err != nil {
			t.Fatalf("RunTx() error = %v", err)
		}

		if got, want := countTxRows(t, ctx, pool), 1; got != want {
			t.Errorf("RunTx() rows = %d, want %d", got, want)
		}
	})

	t.Run("nested inner joins outer transaction", func(t *testing.T) {
		// The inner RunTx finds a transaction already in ctx and reuses it instead
		// of opening its own. Only the outer RunTx commits. The insert must be
		// visible exactly once — two commits would mean two separate transactions
		// were opened, which would be a bug.
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		setupTxTable(t, ctx, pool)

		doInTx := func(ctx context.Context) error {
			innerDoInTx := func(ctx context.Context) error {
				return insertInTx(t, ctx)
			}
			return pgdb.RunTx(ctx, pool, innerDoInTx)
		}
		if err := pgdb.RunTx(ctx, pool, doInTx); err != nil {
			t.Fatalf("RunTx() error = %v", err)
		}

		if got, want := countTxRows(t, ctx, pool), 1; got != want {
			t.Errorf("RunTx() rows = %d, want %d", got, want)
		}
	})

	t.Run("rolls back on error", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		setupTxTable(t, ctx, pool)

		doInTx := func(ctx context.Context) error {
			if err := insertInTx(t, ctx); err != nil {
				return err
			}
			return sentinel
		}
		if err := pgdb.RunTx(ctx, pool, doInTx); !errors.Is(err, sentinel) {
			t.Fatalf("RunTx() error = %v, want %v", err, sentinel)
		}

		if got, want := countTxRows(t, ctx, pool), 0; got != want {
			t.Errorf("RunTx() rows after rollback = %d, want %d", got, want)
		}
	})

	t.Run("nested outer error rolls back inner writes", func(t *testing.T) {
		// The inner RunTx returns nil (its work succeeded), but the outer RunTx
		// returns an error afterward. Because the inner call is not the transaction
		// owner it did not commit, so the outer rollback covers the inner write too.
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		setupTxTable(t, ctx, pool)

		doInTx := func(ctx context.Context) error {
			nestedDoInTx := func(ctx context.Context) error {
				return insertInTx(t, ctx)
			}
			if err := pgdb.RunTx(ctx, pool, nestedDoInTx); err != nil {
				return err
			}
			return sentinel
		}
		if err := pgdb.RunTx(ctx, pool, doInTx); !errors.Is(err, sentinel) {
			t.Fatalf("RunTx() error = %v, want %v", err, sentinel)
		}

		if got, want := countTxRows(t, ctx, pool), 0; got != want {
			t.Errorf("RunTx() rows after outer rollback = %d, want %d", got, want)
		}
	})
}

func TestTransactor_RunTx(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)
	setupTxTable(t, ctx, pool)

	doInTx := func(ctx context.Context) error {
		return insertInTx(t, ctx)
	}
	if err := pgdb.NewTransactor(pool).RunTx(ctx, doInTx); err != nil {
		t.Fatalf("Transactor.RunTx() error = %v", err)
	}

	if got, want := countTxRows(t, ctx, pool), 1; got != want {
		t.Errorf("Transactor.RunTx() rows = %d, want %d", got, want)
	}
}

func TestRunTx_transactionLocalSettings(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.New(t, ctx)

	projectID := 42
	userID := uuid.New()
	traceID := trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	spanID := trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8}

	ctx = mdl.ContextWithAuthSession(ctx, mdl.AuthSession{
		User:    mdl.AuthUser{UserID: userID},
		Project: &mdl.AuthProject{ID: projectID},
	})
	ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
	}))

	if err := pgdb.RunTx(ctx, pool, func(ctx context.Context) error {
		tx := pgdb.TxFromCtx(ctx)
		var (
			gotProjectID string
			gotUserID    string
			gotTraceID   string
		)
		if err := tx.QueryRow(ctx, `SELECT
			current_setting('app.project_id'),
			current_setting('app.user_id'),
			current_setting('app.trace_id')`).Scan(&gotProjectID, &gotUserID, &gotTraceID); err != nil {
			return err
		}

		if got, want := gotProjectID, strconv.Itoa(projectID); got != want {
			t.Errorf("current_setting(app.project_id) = %q, want %q", got, want)
		}
		if got, want := gotUserID, userID.String(); got != want {
			t.Errorf("current_setting(app.user_id) = %q, want %q", got, want)
		}
		if got, want := gotTraceID, traceID.String(); got != want {
			t.Errorf("current_setting(app.trace_id) = %q, want %q", got, want)
		}
		return nil
	}); err != nil {
		t.Fatalf("RunTx() error = %v", err)
	}

	assertTxSettingsReset(t, ctx, pool)
}

func TestRunTx_transactionLocalSettingsResetOnRollback(t *testing.T) {
	ctx := mdl.ContextWithAuthSession(context.Background(), mdl.AuthSession{
		User:    mdl.AuthUser{UserID: uuid.New()},
		Project: &mdl.AuthProject{ID: 42},
	})
	pool := pgtest.New(t, ctx)

	sentinel := errors.New("sentinel")

	if err := pgdb.RunTx(ctx, pool, func(context.Context) error {
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("RunTx() error = %v, want %v", err, sentinel)
	}

	assertTxSettingsReset(t, ctx, pool)
}

func TestRunExec(t *testing.T) {
	t.Run("commits on success", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		setupTxTable(t, ctx, pool)

		if err := pgdb.RunExec(ctx, pool, "INSERT INTO _pgdb_tx_test (val) VALUES (1)"); err != nil {
			t.Fatalf("RunExec() error = %v", err)
		}

		if got, want := countTxRows(t, ctx, pool), 1; got != want {
			t.Errorf("RunExec() rows = %d, want %d", got, want)
		}
	})

	t.Run("joins transaction and rolls back on error", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		setupTxTable(t, ctx, pool)

		sentinel := errors.New("sentinel")

		err := pgdb.RunTx(ctx, pool, func(ctx context.Context) error {
			if err := pgdb.RunExec(ctx, pool, "INSERT INTO _pgdb_tx_test (val) VALUES (1)"); err != nil {
				return err
			}
			return sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Fatalf("RunTx() error = %v, want %v", err, sentinel)
		}

		if got, want := countTxRows(t, ctx, pool), 0; got != want {
			t.Errorf("RunExec in RunTx: rows after rollback = %d, want %d", got, want)
		}
	})
}

func TestRunBatch(t *testing.T) {
	q := pgdb.TypedQuery[int]{
		SQL:    "INSERT INTO _pgdb_tx_test (val) VALUES (1) RETURNING val",
		Scan:   func(row pgx.CollectableRow) (int, error) { var v int; return v, row.Scan(&v) },
		Expect: pgdb.ExpectOne,
	}

	t.Run("commits on success", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		setupTxTable(t, ctx, pool)

		var val int
		if err := pgdb.RunBatch(ctx, pool, func(ctx context.Context, b *pgdb.Batch) error {
			return q.Queue(ctx, b, &val)
		}); err != nil {
			t.Fatalf("RunBatch() error = %v", err)
		}

		if got, want := countTxRows(t, ctx, pool), 1; got != want {
			t.Errorf("RunBatch() rows = %d, want %d", got, want)
		}
	})

	t.Run("joins transaction and rolls back on error", func(t *testing.T) {
		ctx := context.Background()
		pool := pgtest.New(t, ctx)
		setupTxTable(t, ctx, pool)

		sentinel := errors.New("sentinel")

		err := pgdb.RunTx(ctx, pool, func(ctx context.Context) error {
			var val int
			if err := pgdb.RunBatch(ctx, pool, func(ctx context.Context, b *pgdb.Batch) error {
				return q.Queue(ctx, b, &val)
			}); err != nil {
				return err
			}
			return sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Fatalf("RunTx() error = %v, want %v", err, sentinel)
		}

		if got, want := countTxRows(t, ctx, pool), 0; got != want {
			t.Errorf("RunBatch in RunTx: rows after rollback = %d, want %d", got, want)
		}
	})
}

// setupTxTable creates a scratch table for transaction tests. Each call to
// pgtest.New gets a fresh isolated database, so the table name does not collide
// across tests.
func setupTxTable(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, "CREATE TABLE _pgdb_tx_test (val int)"); err != nil {
		t.Fatalf("create test table: %v", err)
	}
}

func countTxRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM _pgdb_tx_test").Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

func assertTxSettingsReset(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	var (
		projectID string
		userID    string
		traceID   string
	)
	if err := pool.QueryRow(ctx, `SELECT
		COALESCE(current_setting('app.project_id', true), ''),
		COALESCE(current_setting('app.user_id', true), ''),
		COALESCE(current_setting('app.trace_id', true), '')`).Scan(&projectID, &userID, &traceID); err != nil {
		t.Fatalf("query transaction-local settings after transaction: %v", err)
	}

	if projectID != "" {
		t.Errorf("current_setting(app.project_id) after transaction = %q, want empty", projectID)
	}
	if userID != "" {
		t.Errorf("current_setting(app.user_id) after transaction = %q, want empty", userID)
	}
	if traceID != "" {
		t.Errorf("current_setting(app.trace_id) after transaction = %q, want empty", traceID)
	}
}

// insertInTx executes an INSERT on the transaction carried in ctx. It is the
// simplest way to write a row that participates in the current transaction
// without going through the batch layer.
func insertInTx(t *testing.T, ctx context.Context) error {
	t.Helper()
	tx := pgdb.TxFromCtx(ctx)
	if tx == nil {
		t.Fatal("insertInTx: no transaction in context")
	}
	_, err := tx.Exec(ctx, "INSERT INTO _pgdb_tx_test (val) VALUES (1)")
	return err
}
