package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"the-dancing-pony-v2/internal/dish"
	"the-dancing-pony-v2/internal/httpapi"
)

func noEnv(string) string { return "" }

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{"no command", nil, 2, "", "Usage: pony"},
		{"help", []string{"help"}, 0, "Usage: pony", ""},
		{"help flag", []string{"--help"}, 0, "Usage: pony", ""},
		{"unknown command", []string{"gallop"}, 2, "", `unknown command "gallop"`},
		{"bad serve flag", []string{"serve", "-nope"}, 2, "", "flag provided but not defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tt.args, noEnv, &stdout, &stderr)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestServeFailsWhenAddressInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"serve", "-addr", ln.Addr().String()}, noEnv, &stdout, &stderr)

	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "listen failed") {
		t.Errorf("stderr = %q, want it to mention listen failed", stderr.String())
	}
}

func TestLoadConfig(t *testing.T) {
	if got := loadConfig(noEnv).Addr; got != "127.0.0.1:8080" {
		t.Errorf("default addr = %q, want 127.0.0.1:8080", got)
	}

	env := map[string]string{"PONY_ADDR": "0.0.0.0:9000"}
	if got := loadConfig(func(k string) string { return env[k] }).Addr; got != "0.0.0.0:9000" {
		t.Errorf("addr from env = %q, want 0.0.0.0:9000", got)
	}
}

func TestNewServerSetsTimeouts(t *testing.T) {
	srv := newServer(http.NotFoundHandler(), discardLogger())

	for name, d := range map[string]time.Duration{
		"ReadHeaderTimeout": srv.ReadHeaderTimeout,
		"ReadTimeout":       srv.ReadTimeout,
		"WriteTimeout":      srv.WriteTimeout,
		"IdleTimeout":       srv.IdleTimeout,
	} {
		if d <= 0 {
			t.Errorf("%s = %v, want > 0", name, d)
		}
	}
}

func TestRunServerServesUntilCancelled(t *testing.T) {
	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := startServer(ctx, httpapi.New(dish.NewMemoryStore(dish.Seed()...)), ln)

	resp, err := http.Get("http://" + ln.Addr().String() + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	cancel()
	if err := waitFor(t, done); err != nil {
		t.Errorf("runServer returned %v, want nil", err)
	}
}

func TestRunServerFinishesInFlightRequestsOnShutdown(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	})

	ln := listen(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := startServer(ctx, slow, ln)

	status := make(chan int, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/")
		if err != nil {
			status <- 0
			return
		}
		resp.Body.Close()
		status <- resp.StatusCode
	}()

	<-started
	cancel()
	select {
	case err := <-done:
		t.Fatalf("runServer returned %v before the in-flight request finished", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	if got := <-status; got != http.StatusOK {
		t.Errorf("in-flight request status = %d, want %d", got, http.StatusOK)
	}
	if err := waitFor(t, done); err != nil {
		t.Errorf("runServer returned %v, want nil", err)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func startServer(ctx context.Context, h http.Handler, ln net.Listener) <-chan error {
	done := make(chan error, 1)
	go func() { done <- runServer(ctx, newServer(h, discardLogger()), ln, discardLogger()) }()
	return done
}

func waitFor(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("runServer did not return within 5s")
		return nil
	}
}
