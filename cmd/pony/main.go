// Command pony runs and administers The Dancing Pony API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"the-dancing-pony-v2/internal/httpapi"
	"the-dancing-pony-v2/internal/store"
)

const usage = `Usage: pony <command> [flags]

Commands:
  serve     run the HTTP API
  migrate   apply or inspect database migrations (pony migrate help)
  seed      add the demo restaurants and dishes (pony seed help)
  help      show this help

Environment:
  PONY_ADDR                   listen address for serve (default 127.0.0.1:8080)
  PONY_DATABASE_URL           Postgres URL for serve, as the pony_app role
  PONY_MIGRATE_DATABASE_URL   Postgres URL for migrate and seed, as the schema owner
`

const (
	defaultAddr     = "127.0.0.1:8080"
	shutdownTimeout = 15 * time.Second
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// run dispatches to a subcommand and returns the process exit code.
func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage)
		return 2
	}

	switch args[0] {
	case "serve":
		return serve(args[1:], getenv, stderr)
	case "migrate":
		return migrate(args[1:], getenv, stdout, stderr)
	case "seed":
		return seed(args[1:], getenv, stdout, stderr)
	case "help", "-h", "--help":
		_, _ = fmt.Fprint(stdout, usage)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "pony: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

type config struct {
	// Addr is the TCP address to listen on. It defaults to loopback so the
	// API is only reachable through the reverse proxy.
	Addr string
	// DatabaseURL connects as pony_app, which Row-Level Security restricts.
	DatabaseURL string
}

func loadConfig(getenv func(string) string) config {
	cfg := config{
		Addr:        defaultAddr,
		DatabaseURL: getenv("PONY_DATABASE_URL"),
	}
	if v := getenv("PONY_ADDR"); v != "" {
		cfg.Addr = v
	}
	return cfg
}

func serve(args []string, getenv func(string) string, stderr io.Writer) int {
	cfg := loadConfig(getenv)

	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "listen address (overrides PONY_ADDR)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	logger := slog.New(slog.NewJSONHandler(stderr, nil))
	slog.SetDefault(logger)

	if cfg.DatabaseURL == "" {
		logger.Error("PONY_DATABASE_URL is not set")
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", cfg.Addr)
	if err != nil {
		logger.Error("listen failed", "addr", cfg.Addr, "err", err)
		return 1
	}
	logger.Info("listening", "addr", ln.Addr().String())

	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		_ = ln.Close()
		logger.Error("cannot use database", "err", err)
		return 1
	}
	defer db.Close()

	if err := runServer(ctx, newServer(httpapi.New(db), logger), ln, logger); err != nil {
		logger.Error("server stopped with error", "err", err)
		return 1
	}
	logger.Info("server stopped")
	return 0
}

// newServer returns an HTTP server with timeouts, so slow or idle clients
// cannot hold connections open indefinitely.
func newServer(handler http.Handler, logger *slog.Logger) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
}

// runServer serves on ln until ctx is cancelled, then stops accepting new
// connections and waits (up to shutdownTimeout) for in-flight requests.
func runServer(ctx context.Context, srv *http.Server, ln net.Listener, logger *slog.Logger) error {
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()

	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}

	logger.Info("shutting down", "timeout", shutdownTimeout.String())
	// ctx is already cancelled here; keep its values but give shutdown its
	// own deadline.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
