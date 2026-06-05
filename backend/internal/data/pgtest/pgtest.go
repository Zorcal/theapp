// Package pgtest provides PostgreSQL test helpers. All database operations assume PostgreSQL is running via
// docker-compose.
package pgtest

import (
	"cmp"
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"os"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgschema"
)

// New returns a connection pool to a fresh, isolated test database.
func New(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()

	// The schema is built once into a template database and each call hands out a fast low-level clone of that
	// template, so tests do not pay the migration cost individually.

	admin := adminPool(t, ctx)

	tmpl, err := templateName()
	if err != nil {
		t.Fatalf("derive template name: %v", err)
	}
	if err := ensureTemplate(ctx, admin, tmpl); err != nil {
		t.Fatalf("ensure template %q: %v", tmpl, err)
	}

	dbName := randDBName()
	if _, err := admin.Exec(ctx, fmt.Sprintf("CREATE DATABASE %q TEMPLATE %q", dbName, tmpl)); err != nil {
		t.Fatalf("clone template %q into %q: %v", tmpl, dbName, err)
	}
	// Registered before the pool's Close below so cleanups run in the right order: the test pool closes first (no
	// connections left), then the database is dropped, then the admin pool closes last.
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %q", dbName)); err != nil {
			t.Errorf("drop db %q: %v", dbName, err)
		}
	})

	// There are currently no database-level settings (ALTER DATABASE ... SET) to re-apply here; such settings are not
	// inherited when cloning, so if any are added they must be applied to dbName at this point.

	return openPool(t, ctx, dbName)
}

// NewWithoutTemplate returns a connection pool to a fresh, isolated test database built directly from the migrations,
// without going through the template. It is slower than New and exists for tests that exercise the schema/migration
// build itself rather than the resulting schema.
func NewWithoutTemplate(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()

	admin := adminPool(t, ctx)

	dbName := randDBName()
	if err := buildDatabase(ctx, admin, dbName); err != nil {
		t.Fatalf("build db %q: %v", dbName, err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %q", dbName)); err != nil {
			t.Errorf("drop db %q: %v", dbName, err)
		}
	})

	return openPool(t, ctx, dbName)
}

// adminPool opens a pool to the postgres maintenance database, used to create and drop test databases. It registers the
// pool's Close as a cleanup.
func adminPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()

	poolCfg, err := poolConfig("postgres")
	if err != nil {
		t.Fatalf("create admin pool config: %v", err)
	}
	pool, err := pgdb.NewPool(ctx, poolCfg)
	if err != nil {
		t.Fatalf("create admin pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := pgdb.StatusCheck(ctx, pool); err != nil {
		t.Fatalf(`status check admin pool: %v

		Did you remember to run 'docker-compose up -d' at the repository root?
		`, err)
	}

	return pool
}

// openPool opens a pool to dbName, status-checks it, and registers its Close as a cleanup.
func openPool(t *testing.T, ctx context.Context, dbName string) *pgxpool.Pool {
	t.Helper()

	poolCfg, err := poolConfig(dbName)
	if err != nil {
		t.Fatalf("create pool config: %v", err)
	}
	pool, err := pgdb.NewPool(ctx, poolCfg)
	if err != nil {
		t.Fatalf("create db pool %q: %v", dbName, err)
	}
	t.Cleanup(pool.Close)

	if err := pgdb.StatusCheck(ctx, pool); err != nil {
		t.Fatalf("status check db %q: %v", dbName, err)
	}

	return pool
}

// templateName builds the template database name from the schema version. The version in the name makes the template
// self-invalidating: a schema change produces a new name, so a stale template is never silently reused.
func templateName() (string, error) {
	version, err := pgschema.Version()
	if err != nil {
		return "", fmt.Errorf("schema version: %w", err)
	}
	return "theapp_tmpl_" + version, nil
}

// ensureTemplate makes sure the template database exists and is fully built. It is cheap on the common path (a single
// existence check) and safe when multiple test processes run at once: building is serialized by a PostgreSQL advisory
// lock and a database is only marked as a template once its build has fully succeeded.
func ensureTemplate(ctx context.Context, admin *pgxpool.Pool, tmpl string) error {
	// Quick check before taking the lock, to avoid contention on the common path where the template already exists.
	ready, err := templateReady(ctx, admin, tmpl)
	if err != nil {
		return fmt.Errorf("check template: %w", err)
	}
	if ready {
		return nil
	}

	// A dedicated connection (not a pooled one) so that closing it ends the session and releases the advisory lock; a
	// session-level lock taken on a pooled connection would outlive the Exec and linger.
	conn, err := pgx.Connect(ctx, connStr("postgres"))
	if err != nil {
		return fmt.Errorf("connect for template build: %w", err)
	}
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", advisoryKey(tmpl)); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}

	// Double-check now that we hold the lock: another process may have built the template while we waited.
	ready, err = templateReady(ctx, conn, tmpl)
	if err != nil {
		return fmt.Errorf("re-check template: %w", err)
	}
	if ready {
		return nil
	}

	if err := buildDatabase(ctx, conn, tmpl); err != nil {
		return fmt.Errorf("build template: %w", err)
	}

	// Mark ready last. datistemplate is the readiness signal, so it must only be set after the build has fully
	// succeeded; otherwise a parallel process could clone a half-built database.
	if _, err := conn.Exec(ctx, fmt.Sprintf("ALTER DATABASE %q WITH IS_TEMPLATE true", tmpl)); err != nil {
		return fmt.Errorf("mark template ready: %w", err)
	}

	return nil
}

// execer and queryer are the subsets of the maintenance connection's API used here. Both *pgxpool.Pool and *pgx.Conn
// satisfy them, so the same helpers work whether called on the admin pool or the dedicated build connection.
type (
	execer interface {
		Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	}
	queryer interface {
		QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	}
)

// buildDatabase (re)creates dbName from scratch and installs the full schema by running the migrations against it. A
// half-built database is never marked as a template, so the readiness checks never observe it and DROP ... IF EXISTS is
// safe.
func buildDatabase(ctx context.Context, e execer, dbName string) error {
	if _, err := e.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %q", dbName)); err != nil {
		return fmt.Errorf("drop db %q: %w", dbName, err)
	}
	if _, err := e.Exec(ctx, fmt.Sprintf("CREATE DATABASE %q", dbName)); err != nil {
		return fmt.Errorf("create db %q: %w", dbName, err)
	}
	if err := pgschema.Migrate(ctx, connStr(dbName)); err != nil {
		return fmt.Errorf("migrate db %q: %w", dbName, err)
	}
	return nil
}

// templateReady reports whether tmpl exists and is marked as a template.
func templateReady(ctx context.Context, q queryer, tmpl string) (bool, error) {
	const sql = `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1 AND datistemplate)`
	var ready bool
	if err := q.QueryRow(ctx, sql, tmpl).Scan(&ready); err != nil {
		return false, err
	}
	return ready, nil
}

// advisoryKey derives a stable PostgreSQL advisory lock key from the template name, so all processes building the same
// template contend on the same lock.
func advisoryKey(tmpl string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(tmpl))
	return int64(h.Sum64()) //nolint:gosec // Wrap-around is fine; we only need a stable key
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

	// Each test gets its own pool (admin + test-DB), so keep MaxConns small to
	// avoid exhausting the Postgres instance's max_connections when many tests
	// run in parallel. 2 is enough: store methods are sequential within a test,
	// and a single extra slot covers any incidental concurrency. If a future
	// test needs more concurrent connections, increase it locally for that test.
	cfg.MaxConns = 2

	return cfg, nil
}

func connStr(dbName string) string {
	// Static database configuration. Defaults target the host-published port from
	// infra/docker-compose.yml; the container name resolves only inside the compose
	// network, so host-run tests reach Postgres via localhost. Override with the
	// POSTGRES_* env vars when running inside the network.
	const (
		host       = "localhost"
		port       = 5433
		username   = "postgres"
		password   = "postgres"
		sslEnabled = false
	)

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
