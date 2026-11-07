// Package httpapi exposes restaurants' menus as a JSON HTTP API.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"the-dancing-pony-v2/internal/dish"
	"the-dancing-pony-v2/internal/id"
	"the-dancing-pony-v2/internal/store"
	"the-dancing-pony-v2/internal/tenant"
)

// Store is the persistence the API needs; *store.Store implements it.
// Every dish method is scoped to one restaurant.
type Store interface {
	RestaurantBySlug(ctx context.Context, slug string) (tenant.Restaurant, error)
	ListDishes(ctx context.Context, restaurantID uuid.UUID) ([]dish.Dish, error)
	GetDish(ctx context.Context, restaurantID, dishID uuid.UUID) (dish.Dish, error)
	CreateDish(ctx context.Context, restaurantID uuid.UUID, d dish.Dish) (dish.Dish, error)
	UpdateDish(ctx context.Context, restaurantID, dishID uuid.UUID, d dish.Dish) (dish.Dish, error)
	DeleteDish(ctx context.Context, restaurantID, dishID uuid.UUID) error
}

type api struct {
	store Store
}

// New returns the HTTP handler for the API. Routes use the standard library's
// method and wildcard patterns, so unsupported methods get a 405 with an
// Allow header and unknown paths get a 404. All errors are RFC 9457
// application/problem+json.
func New(s Store) http.Handler {
	a := &api{store: s}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.root)
	mux.HandleFunc("GET /health", a.health)
	for _, rt := range a.tenantRoutes() {
		mux.HandleFunc(rt.pattern, a.inRestaurant(rt.handler))
	}
	return router{mux: mux}
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
	}
}

// restaurantHandler handles a request within one restaurant (tenant).
type restaurantHandler func(http.ResponseWriter, *http.Request, tenant.Restaurant)

// inRestaurant resolves the {slug} in the path to the restaurant that every
// query in h is scoped to, and records it in the request context. An unknown
// slug is a 404.
func (a *api) inRestaurant(h restaurantHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rest, err := a.store.RestaurantBySlug(r.Context(), r.PathValue("slug"))
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

func (a *api) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *api) listDishes(w http.ResponseWriter, r *http.Request, rest tenant.Restaurant) {
	dishes, err := a.store.ListDishes(r.Context(), rest.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dishes)
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

// decodeDish reads and validates a dish from the request body. On failure it
// writes a 400 problem response and returns false.
func decodeDish(w http.ResponseWriter, r *http.Request, rest tenant.Restaurant) (dish.Dish, bool) {
	var input dish.Dish
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid JSON")
		return dish.Dish{}, false
	}
	if err := dish.Validate(input, rest.Currency); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return dish.Dish{}, false
	}
	return input, true
}

// internalError logs err and sends a 500 without its details.
func internalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
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
