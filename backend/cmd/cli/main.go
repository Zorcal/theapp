// Command cli is a general-purpose operator CLI for theapp, run alongside cmd/server rather than through it.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ardanlabs/conf/v3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/urfave/cli/v3"

	"github.com/zorcal/theapp/backend/cmd/cli/commands"
	"github.com/zorcal/theapp/backend/internal/core/org"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/core/rbac"
	"github.com/zorcal/theapp/backend/internal/core/user"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

const appName = "theappcli"

// appVersion should be set at build time using -ldflags.
var appVersion = "dev"

type Config struct {
	PgDB struct {
		User       string `conf:"default:postgres"`
		Password   string `conf:"default:postgres,mask"`
		Host       string `conf:"default:127.0.0.1"`
		Port       int    `conf:"default:5433"`
		Name       string `conf:"default:theapp"`
		SSLEnabled bool   `conf:"default:false"`
	}
}

func main() {
	ctx := context.Background()

	if err := run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	var cfg Config
	if err := parseConfigFromEnv(&cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	pgDBConnStr := pgdb.ConnStr(cfg.PgDB.Host, cfg.PgDB.Port, cfg.PgDB.User, cfg.PgDB.Password, cfg.PgDB.Name, cfg.PgDB.SSLEnabled)

	pgPoolCfg, err := pgxpool.ParseConfig(pgDBConnStr)
	if err != nil {
		return fmt.Errorf("parse pg db pool config: %w", err)
	}

	pgPool, err := pgdb.NewPool(ctx, pgPoolCfg)
	if err != nil {
		return fmt.Errorf("new pg db pool: %w", err)
	}
	defer pgPool.Close()

	pgUserStore := pguser.NewStore(pgPool)
	pgRBACStore := pgrbac.NewStore(pgPool)
	pgOrgStore := pgorg.NewStore(pgPool)
	userCore := user.NewCore(pgUserStore)
	rbacCore := rbac.NewCore(pgRBACStore, pgUserStore, pgOrgStore, pgdb.NewTransactor(pgPool))
	orgCore := org.NewCore(pgOrgStore, pgdb.NewTransactor(pgPool))

	cmd := &cli.Command{
		Name:                  "cli",
		Usage:                 "Administer theapp",
		EnableShellCompletion: true,
		Suggest:               true,
		Version:               appVersion,
		Commands: []*cli.Command{
			commands.NewDBCommand(pgDBConnStr),
			commands.NewUserCommand(userCore),
			commands.NewRoleCommand(userCore, rbacCore),
			commands.NewOrgCommand(userCore, orgCore),
		},
	}

	return cmd.Run(ctx, os.Args)
}

// parseConfigFromEnv populates cfg from environment variables and defaults only. conf.Parse always parses
// os.Args itself, which would collide with urfave/cli's own parsing of this CLI's subcommands and flags; os.Args
// is swapped out for the duration of the call so conf sees no arguments, then restored before returning. Since
// os.Args is a process-global, this must not run concurrently with any other code reading it.
func parseConfigFromEnv(cfg *Config) error {
	realArgs := os.Args
	os.Args = realArgs[:1]
	defer func() { os.Args = realArgs }()

	if _, err := conf.Parse(strings.ToUpper(appName), cfg); err != nil {
		return err
	}

	return nil
}
