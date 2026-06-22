package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/ardanlabs/conf/v3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lmittmann/tint"
	"github.com/pgx-contrib/pgxotel"

	"github.com/zorcal/theapp/backend/internal/api/grpc"
	"github.com/zorcal/theapp/backend/internal/clients/resend"
	"github.com/zorcal/theapp/backend/internal/core/auth"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgauth"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/core/user"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgschema"
	"github.com/zorcal/theapp/backend/internal/email"
	"github.com/zorcal/theapp/backend/internal/telemetry"
	"github.com/zorcal/theapp/backend/pkg/slogctx"
)

const appName = "theapp"

// appVersion should be set at build time using -ldflags.
var appVersion = "dev"

type Config struct {
	conf.Version

	Net struct {
		Address string `conf:"default:127.0.0.1:5051"`
	}
	Environment string `conf:"default:local"`
	Telemetry   struct {
		Enabled  bool   `conf:"default:true"`
		Endpoint string `conf:"default:127.0.0.1:4317"`
		Insecure bool   `conf:"default:true"`
	}
	PgDB struct {
		User       string `conf:"default:postgres"`
		Password   string `conf:"default:postgres,mask"`
		Host       string `conf:"default:127.0.0.1"`
		Port       int    `conf:"default:5433"`
		Name       string `conf:"default:theapp"`
		SSLEnabled bool   `conf:"default:false"`
		Pool       struct {
			// MaxConns is bounded by Postgres max_connections (default 100) shared across all app instances plus
			// migrations and admin clients. pgx's own default of 4 bottlenecks a concurrent gRPC server, so we raise it
			// to 10: enough concurrency while leaving ample headroom under the cap.
			MaxConns int32 `conf:"default:10"`
			// MinConns 0 lets the pool drain to nothing when idle; raise it in production to avoid connection-setup
			// latency on the first requests after a quiet period. Matches pgx's default.
			MinConns          int32         `conf:"default:0"`
			MaxConnLifetime   time.Duration `conf:"default:1h"`
			MaxConnIdleTime   time.Duration `conf:"default:30m"`
			HealthCheckPeriod time.Duration `conf:"default:60s"`
			// MaxConnLifetimeJitter spreads out connection recycling. Without it, connections opened together at
			// startup all reach MaxConnLifetime at once and reconnect in a herd; 5m staggers them. pgx defaults this to
			// 0 (no jitter).
			MaxConnLifetimeJitter time.Duration `conf:"default:5m"`
		}
	}
	Client struct {
		Resend struct {
			APIKey string `conf:"mask"`
		}
	}
	Auth struct {
		// JWTSecret is the HMAC-SHA256 signing key for access tokens. Must be changed from the default before deploying
		// outside local environments.
		JWTSecret   string `conf:"default:dev-secret-change-me,mask"`
		JWTIssuer   string `conf:"default:theapp"`
		JWTAudience string `conf:"default:theapp-api"`
		// FromEmail is the sender address used for magic-link emails.
		FromEmail string `conf:"default:noreply@theapp.com"`
		// MagicLinkBaseURL is the frontend URL that receives the token query parameter, e.g.
		// "https://theapp.com/auth/verify".
		MagicLinkBaseURL   string        `conf:"default:http://localhost:3000/auth/verify"`
		MagicLinkTTL       time.Duration `conf:"default:15m"`
		MagicLinkRateLimit time.Duration `conf:"default:1m"`
		AccessTokenTTL     time.Duration `conf:"default:15m"`
		RefreshTokenTTL    time.Duration `conf:"default:720h"` // 30 days
	}
}

func (c Config) IsLocalEnvironment() bool {
	return strings.EqualFold(c.Environment, "local")
}

func (c Config) Validate() error {
	if !c.IsLocalEnvironment() && c.Auth.JWTSecret == "dev-secret-change-me" {
		return errors.New("Auth.JWTSecret must be changed from the default before running outside local environments")
	}

	return nil
}

func main() {
	ctx := context.Background()

	cfg := Config{
		Version: conf.Version{
			Build: appVersion,
			Desc:  "The app",
		},
	}
	if help, err := conf.Parse(strings.ToUpper(appName), &cfg); err != nil {
		if errors.Is(err, conf.ErrHelpWanted) {
			fmt.Fprint(os.Stdout, help)
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error parsing config: %v\n", err)
		os.Exit(1)
	}

	if err := run(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Run error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}

	// Setup open telemetry.

	// Bootstrap logger is stdout-only; telemetry init can't log through
	// itself before its own pipeline is up.
	bootstrapLogger := configureLogger(cfg, nil)

	telemetryConfig := telemetry.Config{
		Enabled:  cfg.Telemetry.Enabled,
		Endpoint: cfg.Telemetry.Endpoint,
		Insecure: cfg.Telemetry.Insecure,
	}
	cleanupTracing, err := telemetry.InitTracing(ctx, appName, appVersion, telemetryConfig, bootstrapLogger)
	if err != nil {
		return fmt.Errorf("init telemetry tracing: %w", err)
	}
	defer cleanupTracing()

	otelSlogHandler, cleanupLogging, err := telemetry.InitLogging(ctx, appName, appVersion, telemetryConfig, logLevel(cfg), bootstrapLogger)
	if err != nil {
		return fmt.Errorf("init telemetry logging: %w", err)
	}
	defer cleanupLogging()

	cleanupMetrics, err := telemetry.InitMetrics(ctx, appName, appVersion, telemetryConfig, bootstrapLogger)
	if err != nil {
		return fmt.Errorf("init telemetry metrics: %w", err)
	}
	defer cleanupMetrics()

	log := configureLogger(cfg, otelSlogHandler)

	strCfg, err := conf.String(&cfg)
	if err != nil {
		return fmt.Errorf("generate config for output: %w", err)
	}
	log.InfoContext(ctx, "Starting...", "config", strCfg)

	// Migrate PostgreSQL database.

	log.InfoContext(ctx, "Migrating PostgreSQL database")

	pgDBConnStr := pgdb.ConnStr(cfg.PgDB.Host, cfg.PgDB.Port, cfg.PgDB.User, cfg.PgDB.Password, cfg.PgDB.Name, cfg.PgDB.SSLEnabled)

	if err := pgschema.Migrate(ctx, pgDBConnStr); err != nil {
		return fmt.Errorf("migrate pg db: %w", err)
	}

	// Setup database connection pool.

	log.InfoContext(ctx, "Setting up PostgreSQL database connection pool")

	pgPoolCfg, err := pgxpool.ParseConfig(pgDBConnStr)
	if err != nil {
		return fmt.Errorf("parse pg db pool config: %w", err)
	}
	pgPoolCfg.MaxConns = cfg.PgDB.Pool.MaxConns
	pgPoolCfg.MinConns = cfg.PgDB.Pool.MinConns
	pgPoolCfg.MaxConnLifetime = cfg.PgDB.Pool.MaxConnLifetime
	pgPoolCfg.MaxConnIdleTime = cfg.PgDB.Pool.MaxConnIdleTime
	pgPoolCfg.HealthCheckPeriod = cfg.PgDB.Pool.HealthCheckPeriod
	pgPoolCfg.MaxConnLifetimeJitter = cfg.PgDB.Pool.MaxConnLifetimeJitter
	pgPoolCfg.ConnConfig.Tracer = &pgxotel.QueryTracer{
		Name: cfg.PgDB.Name + "-postgres",
	}

	pgPool, err := pgdb.NewPool(ctx, pgPoolCfg)
	if err != nil {
		return fmt.Errorf("new pg db pool: %w", err)
	}
	defer pgPool.Close()

	if err := pgdb.StatusCheck(ctx, pgPool); err != nil {
		return fmt.Errorf("status check pg db connection: %w", err)
	}

	// Setup clients.

	var emailSender email.Sender = email.NewLogSender(log)
	if cfg.Client.Resend.APIKey != "" {
		emailSender = resend.NewEmailClient(cfg.Client.Resend.APIKey)
	}

	// Setup pg stores.

	pgUserStore := pguser.NewStore(pgPool)
	pgAuthStore := pgauth.NewStore(pgPool)

	// Setup cores.

	userCore := user.NewCore(pgUserStore)
	authCore := auth.NewCore(pgAuthStore, pgUserStore, emailSender, pgdb.NewTransactor(pgPool), auth.Config{
		JWTKey:             []byte(cfg.Auth.JWTSecret),
		JWTIssuer:          cfg.Auth.JWTIssuer,
		JWTAudience:        cfg.Auth.JWTAudience,
		MagicLinkFromEmail: cfg.Auth.FromEmail,
		MagicLinkBaseURL:   cfg.Auth.MagicLinkBaseURL,
		MagicLinkTTL:       cfg.Auth.MagicLinkTTL,
		MagicLinkRateLimit: cfg.Auth.MagicLinkRateLimit,
		AccessTokenTTL:     cfg.Auth.AccessTokenTTL,
		RefreshTokenTTL:    cfg.Auth.RefreshTokenTTL,
	})

	// Setup gRPC server.

	log.InfoContext(ctx, "Setting up gRPC server")
	defer log.InfoContext(ctx, "gRPC server stopped")

	srv := grpc.NewServer(grpc.ServerConfig{
		Log:         log,
		UserCore:    userCore,
		AuthCore:    authCore,
		JWTKey:      []byte(cfg.Auth.JWTSecret),
		JWTIssuer:   cfg.Auth.JWTIssuer,
		JWTAudience: cfg.Auth.JWTAudience,
		Reflection:  cfg.IsLocalEnvironment(),
	})

	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", cfg.Net.Address)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- srv.Serve(lis)
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		return fmt.Errorf("server error: %w", err)

	case <-shutdown:
		srv.GracefulStop()
	}

	return nil
}

func configureLogger(cfg Config, otelHandler slog.Handler) *slog.Logger {
	h := configureSlogHandler(cfg, otelHandler)
	buildInfo, _ := debug.ReadBuildInfo()
	return slog.New(h).
		With(slog.Group(
			"program_info",
			"build", appVersion,
			"pid", os.Getpid(),
			"go_version", buildInfo.GoVersion,
		))
}

func configureSlogHandler(cfg Config, otelHandler slog.Handler) slog.Handler {
	logLvl := logLevel(cfg)
	var h slog.Handler
	if cfg.IsLocalEnvironment() {
		h = tint.NewHandler(os.Stdout, &tint.Options{Level: logLvl, TimeFormat: time.Kitchen})
	} else {
		h = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLvl})
	}
	if otelHandler != nil {
		h = slog.NewMultiHandler(h, otelHandler)
	}
	h = slogctx.NewHandler(h)
	return h
}

func logLevel(cfg Config) slog.Level {
	if cfg.IsLocalEnvironment() {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}
