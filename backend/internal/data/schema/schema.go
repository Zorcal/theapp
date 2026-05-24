// Package schema contains the database schema, migrations and seeding data.
package schema

import (
	"context"
	"embed"
	"fmt"
	"net/url"

	"github.com/amacneil/dbmate/v2/pkg/dbmate"

	_ "github.com/amacneil/dbmate/v2/pkg/driver/postgres"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate attempts to bring the database up to date with the migrations
// defined in this package.
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
