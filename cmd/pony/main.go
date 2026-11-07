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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"the-dancing-pony-v2/internal/httpapi"
	"the-dancing-pony-v2/internal/photo"
	"the-dancing-pony-v2/internal/store"
)

const usage = `Usage: pony <command> [flags]

Commands:
  serve     run the HTTP API
  migrate   apply or inspect database migrations (pony migrate help)
  seed      add the demo users, restaurants and dishes (pony seed help)
  admin     create platform admins, grant restaurant roles (pony admin help)
  help      show this help

Environment:
  PONY_ADDR                   listen address for serve (default 127.0.0.1:8080)
  PONY_DATABASE_URL           Postgres URL for serve, as the pony_app role
  PONY_LOG_LEVEL              debug, info (default), warn or error
  PONY_TRUSTED_PROXIES        CIDRs of reverse proxies whose X-Real-IP and
                              X-Forwarded-For are believed, e.g. 127.0.0.1/32
                              behind Nginx (default: none)
  PONY_INTERNAL_ADDR          listen address for /metrics (default 127.0.0.1:9091);
                              never expose it publicly
  PONY_MIGRATE_DATABASE_URL   Postgres URL for migrate and seed, as the schema owner
  PONY_PHOTO_DIR              directory for uploaded dish photos (default data/photos)
`

const (
	defaultAddr         = "127.0.0.1:8080"
	defaultPhotoDir     = "data/photos"
	defaultInternalAddr = "127.0.0.1:9091"
	shutdownTimeout     = 15 * time.Second
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
	case "admin":
		return admin(args[1:], getenv, stdout, stderr)
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
	// LogLevel is the minimum level logged. At info, reads (GETs) are not
	// logged; debug logs every request.
	LogLevel string
	// InternalAddr serves operational endpoints (/metrics) on a separate
	// listener that the reverse proxy never forwards to.
	InternalAddr string
	// TrustedProxies is PONY_TRUSTED_PROXIES: comma-separated CIDRs.
	TrustedProxies string
	// PhotoDir is where uploaded dish photos are stored.
	PhotoDir string
}

func loadConfig(getenv func(string) string) config {
	cfg := config{
		Addr:           defaultAddr,
		DatabaseURL:    getenv("PONY_DATABASE_URL"),
		LogLevel:       getenv("PONY_LOG_LEVEL"),
		InternalAddr:   defaultInternalAddr,
		TrustedProxies: getenv("PONY_TRUSTED_PROXIES"),
		PhotoDir:       defaultPhotoDir,
	}
	if v := getenv("PONY_PHOTO_DIR"); v != "" {
		cfg.PhotoDir = v
	}
	if v := getenv("PONY_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := getenv("PONY_INTERNAL_ADDR"); v != "" {
		cfg.InternalAddr = v
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

	var level slog.Level // zero value: info
	if cfg.LogLevel != "" {
		if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
			_, _ = fmt.Fprintf(stderr, "pony serve: PONY_LOG_LEVEL: %v\n", err)
			return 2
		}
	}
	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	trusted, err := httpapi.ParseTrustedProxies(cfg.TrustedProxies)
	if err != nil {
		logger.Error("PONY_TRUSTED_PROXIES is invalid", "err", err)
		return 2
	}

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

	photos, err := photo.NewDiskStore(cfg.PhotoDir)
	if err != nil {
		_ = ln.Close()
		logger.Error("cannot use photo directory", "dir", cfg.PhotoDir, "err", err)
		return 1
	}
	defer func() { _ = photos.Close() }()

	internalLn, err := lc.Listen(ctx, "tcp", cfg.InternalAddr)
	if err != nil {
		_ = ln.Close()
		logger.Error("listen failed", "addr", cfg.InternalAddr, "err", err)
		return 1
	}
	logger.Info("internal listener (metrics)", "addr", internalLn.Addr().String())

	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	api := httpapi.New(db, photos, logger, httpapi.NewMetrics(reg), trusted)
	internalMux := http.NewServeMux()
	internalMux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	// Go note: "go f()" runs f concurrently in a goroutine. The buffered
	// channel carries its result back; receiving from it (<-internalDone)
	// waits until the goroutine has sent.
	internalDone := make(chan error, 1)
	go func() { internalDone <- runServer(ctx, newServer(internalMux, logger), internalLn, logger) }()

	err = runServer(ctx, newServer(api, logger), ln, logger)
	stop() // if the API server failed, stop the internal one too
	if ierr := <-internalDone; err == nil {
		err = ierr
	}
	if err != nil {
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
