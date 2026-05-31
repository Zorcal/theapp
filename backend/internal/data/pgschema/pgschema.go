// Package pgschema contains the database schema, migrations and seeding data.
package pgschema

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/url"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"

	_ "github.com/amacneil/dbmate/v2/pkg/driver/postgres"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate attempts to bring the database up to date with the migrations defined in this package.
func Migrate(ctx context.Context, connStr string) error {
	connURL, err := url.Parse(connStr)
	if err != nil {
		return fmt.Errorf("parse conn URL: %w", err)
	}

	db := dbmate.New(connURL)
	db.FS = migrationsFS
	db.MigrationsDir = []string{"./migrations"}
	db.AutoDumpSchema = false

	if err := db.CreateAndMigrate(); err != nil {
		return fmt.Errorf("create and migrate: %w", err)
	}

	return nil
}

// Version returns a short deterministic digest of the embedded migrations. It hashes every migration's name and
// contents, so any schema change—including an edit to an existing migration—yields a new version. Without this, a stale
// template database built from an older schema could be silently reused in tests.
func Version() (string, error) {
	// fs.Glob returns names in sorted order, so the digest is stable.
	names, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return "", fmt.Errorf("glob migrations: %w", err)
	}

	h := sha256.New()
	for _, name := range names {
		b, err := migrationsFS.ReadFile(name)
		if err != nil {
			return "", fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := fmt.Fprintf(h, "%s\x00%s\x00", name, b); err != nil {
			return "", fmt.Errorf("write to hash: %w", err)
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}
