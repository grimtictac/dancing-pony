package httpapi

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics are the API's Prometheus metrics. Routes are labelled by pattern
// ("GET /restaurants/{slug}/dishes"), never by raw path, so the number of
// time series stays bounded however many dishes and restaurants exist.
type Metrics struct {
	requests       *prometheus.CounterVec
	duration       *prometheus.HistogramVec
	reviewsWritten *prometheus.CounterVec
}

// NewMetrics creates the API's metrics and registers them with reg.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pony_http_requests_total",
			Help: "HTTP requests, by method, route pattern and status code.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pony_http_request_duration_seconds",
			Help:    "HTTP request latency, by method and route pattern.",
			Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5},
		}, []string{"method", "route"}),
		reviewsWritten: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pony_reviews_written_total",
			Help: "Reviews written, by result (created or updated) and sentiment label.",
		}, []string{"result", "sentiment"}),
	}
	reg.MustRegister(m.requests, m.duration, m.reviewsWritten)
	// Start every review series at 0. Prometheus only counts increases it
	// sees, so a series that first appears at 1 would hide that first review.
	for _, result := range []string{"created", "updated"} {
		for _, label := range []string{"positive", "neutral", "negative", "none"} {
			m.reviewsWritten.WithLabelValues(result, label)
		}
	}
	return m
}

// observe records one finished request. A nil *Metrics records nothing,
// so tests and callers that don't want metrics can pass nil.
func (m *Metrics) observe(method, route string, status int, elapsed time.Duration) {
	if m == nil {
		return
	}
	if route == "" {
		route = "unmatched" // 404s for arbitrary paths share one series
	}
	m.requests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
	m.duration.WithLabelValues(method, route).Observe(elapsed.Seconds())
}

func (m *Metrics) reviewWritten(created bool, sentiment string) {
	if m == nil {
		return
	}
	result := "updated"
	if created {
		result = "created"
	}
	if sentiment == "" {
		sentiment = "none"
	}
	m.reviewsWritten.WithLabelValues(result, sentiment).Inc()
}

// Go note: methods on a pointer receiver can be called on a nil pointer;
// observe and reviewWritten check for that, making nil a valid "no
// metrics" value.
