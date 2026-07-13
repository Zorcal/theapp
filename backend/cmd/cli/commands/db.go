package commands

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/zorcal/theapp/backend/internal/data/pgschema"
)

// NewDBCommand returns the "db" command group, backed by the database connection string pgDBConnStr.
func NewDBCommand(pgDBConnStr string) *cli.Command {
	return &cli.Command{
		Name:  "db",
		Usage: "Manage the database",
		Commands: []*cli.Command{
			newDBMigrateCommand(pgDBConnStr),
		},
	}
}

func newDBMigrateCommand(pgDBConnStr string) *cli.Command {
	return &cli.Command{
		Name:  "migrate",
		Usage: "Apply pending database migrations",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if err := pgschema.Migrate(ctx, pgDBConnStr); err != nil {
				return fmt.Errorf("migrate: %w", err)
			}
			return nil
		},
	}
}
