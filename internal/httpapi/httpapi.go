// Package httpapi exposes restaurants' menus as a JSON HTTP API.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"the-dancing-pony-v2/internal/auth"
	"the-dancing-pony-v2/internal/dish"
	"the-dancing-pony-v2/internal/id"
	"the-dancing-pony-v2/internal/review"
	"the-dancing-pony-v2/internal/sentiment"
	"the-dancing-pony-v2/internal/store"
	"the-dancing-pony-v2/internal/tenant"
)

// Store is the persistence the API needs; *store.Store implements it.
// Every dish method is scoped to one restaurant.
type Store interface {
	Ping(ctx context.Context) error
	CreateUser(ctx context.Context, email, passwordHash string) (auth.User, error)
	UserByEmail(ctx context.Context, email string) (auth.User, string, error)
	CreateSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, expires time.Time) error
	UserBySession(ctx context.Context, tokenHash []byte) (auth.User, error)
	DeleteSession(ctx context.Context, tokenHash []byte) error
	RestaurantBySlug(ctx context.Context, slug string) (tenant.Restaurant, error)
	ListDishes(ctx context.Context, restaurantID uuid.UUID, opts store.ListOptions) ([]dish.Dish, string, error)
	GetDish(ctx context.Context, restaurantID, dishID uuid.UUID) (dish.Dish, error)
	CreateDish(ctx context.Context, restaurantID uuid.UUID, d dish.Dish) (dish.Dish, error)
	UpdateDish(ctx context.Context, restaurantID, dishID uuid.UUID, d dish.Dish) (dish.Dish, error)
	DeleteDish(ctx context.Context, restaurantID, dishID uuid.UUID) error
	PutReview(ctx context.Context, restaurantID, dishID, userID uuid.UUID, in review.Input, s *sentiment.Sentiment) (review.Review, bool, error)
	DeleteReview(ctx context.Context, restaurantID, dishID, userID uuid.UUID) error
	ListReviews(ctx context.Context, restaurantID, dishID uuid.UUID, limit int) ([]review.Review, error)
}

type api struct {
	store   Store
	scorer  sentiment.Scorer
	metrics *Metrics
}

// New returns the HTTP handler for the API. Routes use the standard library's
// method and wildcard patterns, so unsupported methods get a 405 with an
// Allow header and unknown paths get a 404. All errors are RFC 9457
// application/problem+json.
//
// m may be nil, to record no metrics.
func New(s Store, logger *slog.Logger, m *Metrics) http.Handler {
	a := &api{store: s, scorer: sentiment.WordList{}, metrics: m}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.root)
	mux.HandleFunc("GET /health", a.livez) // kept for existing clients; same as /livez
	mux.HandleFunc("GET /livez", a.livez)
	mux.HandleFunc("GET /readyz", a.readyz)
	// Everything below needs a logged-in user, except register and login.
	mux.HandleFunc("POST /auth/register", a.register)
	mux.HandleFunc("POST /auth/login", a.login)
	mux.HandleFunc("POST /auth/logout", a.authed(a.logout))
	mux.HandleFunc("GET /auth/me", a.authed(a.me))
	for _, rt := range a.tenantRoutes() {
		mux.HandleFunc(rt.pattern, a.inRestaurant(rt.handler))
	}
	return accessLog(logger, m, router{mux: mux})
}

// tenantRoute is a route within one restaurant.
type tenantRoute struct {
	pattern string
	handler restaurantHandler
}

// tenantRoutes lists every route that works within a restaurant. The
// tenant-isolation tests fail for any route here they don't cover.
func (a *api) tenantRoutes() []tenantRoute {
	return []tenantRoute{
		{"GET /restaurants/{slug}/dishes", a.listDishes},
		{"POST /restaurants/{slug}/dishes", a.createDish},
		{"GET /restaurants/{slug}/dishes/{id}", a.getDish},
		{"PUT /restaurants/{slug}/dishes/{id}", a.updateDish},
		{"DELETE /restaurants/{slug}/dishes/{id}", a.deleteDish},
		{"GET /restaurants/{slug}/dishes/{id}/reviews", a.listReviews},
		{"PUT /restaurants/{slug}/dishes/{id}/reviews/me", a.putReview},
		{"DELETE /restaurants/{slug}/dishes/{id}/reviews/me", a.deleteReview},
	}
}

// restaurantHandler handles a request within one restaurant (tenant).
type restaurantHandler func(http.ResponseWriter, *http.Request, tenant.Restaurant)

// inRestaurant authenticates the caller, then resolves the {slug} in the
// path to the restaurant that every query in h is scoped to, and records
// both in the request context. Authentication comes first, so anonymous
// callers can't even probe which restaurants exist. An unknown slug is a
// 404.
func (a *api) inRestaurant(h restaurantHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := a.authenticate(w, r)
		if !ok {
			return
		}
		r = r.WithContext(withUser(r.Context(), u))

		slug := r.PathValue("slug")
		info(r.Context()).Restaurant = slug
		rest, err := a.store.RestaurantBySlug(r.Context(), slug)
		if errors.Is(err, store.ErrNotFound) {
			writeProblem(w, http.StatusNotFound, errRestaurantNotFound)
			return
		}
		if err != nil {
			internalError(w, r, err)
			return
		}
		h(w, r.WithContext(tenant.WithRestaurant(r.Context(), rest)), rest)
	}
}

func (a *api) root(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"service": "The Orc Shack API", "status": "running"})
}

// livez reports that the process is up and serving. It checks nothing
// else, so a supervisor restarts the process only when it is truly stuck.
func (a *api) livez(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readyz reports whether the API can serve requests: here, whether the
// database answers within a second. A load balancer stops sending
// traffic while it fails, without restarting anything.
func (a *api) readyz(w http.ResponseWriter, r *http.Request) {
	// Go note: context.WithTimeout derives a context that is cancelled
	// after 1s; defer cancel() releases its timer when readyz returns,
	// however it returns.
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	if err := a.store.Ping(ctx); err != nil {
		info(r.Context()).Err = err
		writeProblem(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "database": "ok"})
}

func (a *api) getDish(w http.ResponseWriter, r *http.Request, rest tenant.Restaurant) {
	dishID, ok := pathDishID(w, r)
	if !ok {
		return
	}
	d, err := a.store.GetDish(r.Context(), rest.ID, dishID)
	if !dishFound(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (a *api) createDish(w http.ResponseWriter, r *http.Request, rest tenant.Restaurant) {
	input, ok := decodeDish(w, r, rest)
	if !ok {
		return
	}
	d, err := a.store.CreateDish(r.Context(), rest.ID, input)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (a *api) updateDish(w http.ResponseWriter, r *http.Request, rest tenant.Restaurant) {
	dishID, ok := pathDishID(w, r)
	if !ok {
		return
	}
	input, ok := decodeDish(w, r, rest)
	if !ok {
		return
	}
	d, err := a.store.UpdateDish(r.Context(), rest.ID, dishID, input)
	if !dishFound(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (a *api) deleteDish(w http.ResponseWriter, r *http.Request, rest tenant.Restaurant) {
	dishID, ok := pathDishID(w, r)
	if !ok {
		return
	}
	if !dishFound(w, r, a.store.DeleteDish(r.Context(), rest.ID, dishID)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

const (
	errRestaurantNotFound = "restaurant not found"
	errDishNotFound       = "dish not found"
)

// pathDishID parses the {id} in the path. A malformed ID can't name a dish,
// so it gets the same 404 as one that doesn't exist.
func pathDishID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	u, err := id.Decode(r.PathValue("id"))
	if err != nil {
		writeProblem(w, http.StatusNotFound, errDishNotFound)
		return uuid.Nil, false
	}
	return u, true
}

// dishFound writes a 404 or 500 for err, and reports whether err was nil.
func dishFound(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, http.StatusNotFound, errDishNotFound)
	default:
		internalError(w, r, err)
	}
	return false
}

// internalError sends a 500 without the error's details; the access log
// records err on the request's log line, with its request ID.
func internalError(w http.ResponseWriter, r *http.Request, err error) {
	info(r.Context()).Err = err
	writeProblem(w, http.StatusInternalServerError, "")
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	writeBody(w, status, "application/json", payload)
}

func writeBody(w http.ResponseWriter, status int, contentType string, payload any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("failed to write JSON response", "err", err)
	}
}
