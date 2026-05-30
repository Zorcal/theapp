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
	"github.com/lmittmann/tint"
	"github.com/zorcal/theapp/backend/internal/api/grpc"
	"github.com/zorcal/theapp/backend/internal/core/user"
	"github.com/zorcal/theapp/backend/internal/telemetry"
	"github.com/zorcal/theapp/backend/pkg/slogctx"
)

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
}

func (c Config) IsLocalEnvironment() bool {
	return strings.EqualFold(c.Environment, "local")
}

func main() {
	ctx := context.Background()

	cfg := Config{
		Version: conf.Version{
			Build: appVersion,
			Desc:  "The app",
		},
	}
	if help, err := conf.Parse("THEAPP", &cfg); err != nil {
		if errors.Is(err, conf.ErrHelpWanted) {
			fmt.Fprint(os.Stdout, help)
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error parsing config: %v\n", err)
		os.Exit(1)
	}

	log := configureLogger(cfg)

	if err := run(ctx, cfg, log); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg Config, log *slog.Logger) error {
	strCfg, err := conf.String(&cfg)
	if err != nil {
		return fmt.Errorf("generate config for output: %w", err)
	}

	log.InfoContext(ctx, "Starting...", "config", strCfg)

	// Setup open telemetry.

	telemetryConfig := telemetry.Config{
		Enabled:  cfg.Telemetry.Enabled,
		Endpoint: cfg.Telemetry.Endpoint,
		Insecure: cfg.Telemetry.Insecure,
	}
	cleanupTracing, err := telemetry.InitTracing(ctx, "theapp", appVersion, telemetryConfig, log)
	if err != nil {
		return fmt.Errorf("init telemetry tracing: %w", err)
	}
	defer cleanupTracing()

	// Setup cores.

	userCore := user.NewCore()

	// Setup gRPC server.

	log.InfoContext(ctx, "Setting up gRPC server")
	defer log.InfoContext(ctx, "gRPC server stopped")

	srv := grpc.NewServer(grpc.ServerConfig{
		Log:        log,
		UserCore:   userCore,
		Reflection: cfg.IsLocalEnvironment(),
	})

	lis, err := net.Listen("tcp", cfg.Net.Address)
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

func configureLogger(cfg Config) *slog.Logger {
	h := configureSlogHandler(cfg)
	buildInfo, _ := debug.ReadBuildInfo()
	return slog.New(h).
		With(slog.Group(
			"program_info",
			"build", appVersion,
			"pid", os.Getpid(),
			"go_version", buildInfo.GoVersion,
		))
}

func configureSlogHandler(cfg Config) slog.Handler {
	var h slog.Handler
	if cfg.IsLocalEnvironment() {
		h = tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug, TimeFormat: time.Kitchen})
	} else {
		h = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	h = slogctx.NewHandler(h)
	return h
}
