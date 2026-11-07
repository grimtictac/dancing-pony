package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"the-dancing-pony-v2/internal/id"
)

// requestIDHeader carries the request ID in and out. A caller (or a proxy)
// may supply one so its logs and ours can be joined; otherwise we make one.
const requestIDHeader = "X-Request-ID"

// reqInfo collects what the access log reports about one request. The
// middleware puts a pointer to it in the request context, and code further
// in fills it in: the router sets the route, the tenant middleware the
// restaurant, internalError the error.
type reqInfo struct {
	ID         string
	Route      string
	Restaurant string
	User       string
	Err        error
	RetryAfter time.Duration // set when the request was throttled (429)
}

type reqInfoKey struct{}

// info returns the request's reqInfo, or a throwaway one when there is none
// (e.g. a handler called directly in a test), so callers never check nil.
func info(ctx context.Context) *reqInfo {
	if ri, ok := ctx.Value(reqInfoKey{}).(*reqInfo); ok {
		return ri
	}
	return &reqInfo{}
}

// accessLog logs one line per request, after it completes. Writes (POST,
// PUT, DELETE) log at INFO; reads at DEBUG, so they appear only with
// PONY_LOG_LEVEL=debug; server errors at ERROR. Query strings, bodies and
// headers are never logged: they may carry search terms, tokens or
// passwords.
//
// It also records the request in m (which may be nil).
func accessLog(logger *slog.Logger, m *Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ri := &reqInfo{ID: requestID(r)}
		w.Header().Set(requestIDHeader, ri.ID)

		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), reqInfoKey{}, ri)))

		elapsed := time.Since(start)
		m.observe(r.Method, ri.Route, sw.status, elapsed)

		attrs := []slog.Attr{
			slog.String("request_id", ri.ID),
			slog.String("method", r.Method),
			slog.String("route", ri.Route),
			slog.String("path", r.URL.Path),
			slog.Int("status", sw.status),
			slog.Float64("duration_ms", float64(elapsed.Microseconds())/1000),
			slog.Int("bytes", sw.bytes),
		}
		if ri.User != "" {
			attrs = append(attrs, slog.String("user_id", ri.User))
		}
		if ri.Restaurant != "" {
			attrs = append(attrs, slog.String("restaurant", ri.Restaurant))
		}
		if ri.Err != nil {
			attrs = append(attrs, slog.String("err", ri.Err.Error()))
		}
		if ri.RetryAfter > 0 {
			attrs = append(attrs, slog.Float64("retry_after_s", ri.RetryAfter.Seconds()))
		}
		logger.LogAttrs(r.Context(), logLevel(r.Method, sw.status), "request", attrs...)
	})
}

func logLevel(method string, status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case method == http.MethodGet || method == http.MethodHead:
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

// requestID returns the caller's X-Request-ID if it looks sane, otherwise a
// new one. Untrusted input goes into logs, so it is length- and
// character-limited.
func requestID(r *http.Request) string {
	if v := r.Header.Get(requestIDHeader); v != "" && len(v) <= 64 && isPrintableASCII(v) {
		return v
	}
	return id.Encode(uuid.Must(uuid.NewV7()))
}

func isPrintableASCII(s string) bool {
	for i := range len(s) {
		if s[i] < 0x21 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

// Go note: statusWriter embeds http.ResponseWriter. Embedding promotes the
// embedded type's methods, so statusWriter is itself a ResponseWriter and
// only needs to override the two methods it wants to observe.

// statusWriter records the status code and body size a handler writes.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer (for
// flushing, deadlines and so on) through this wrapper.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
