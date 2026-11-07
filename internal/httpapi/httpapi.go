// Package httpapi exposes the dish store as a JSON HTTP API.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"the-dancing-pony-v2/internal/dish"
)

type api struct {
	dishes *dish.MemoryStore
}

// New returns the HTTP handler for the API. Routes use the standard library's
// method and wildcard patterns, so unsupported methods get a 405 with an
// Allow header and unknown paths get a 404. All errors are RFC 9457
// application/problem+json.
func New(dishes *dish.MemoryStore) http.Handler {
	a := &api{dishes: dishes}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.root)
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("GET /dishes", a.listDishes)
	mux.HandleFunc("POST /dishes", a.createDish)
	mux.HandleFunc("GET /dishes/{id}", a.getDish)
	mux.HandleFunc("PUT /dishes/{id}", a.updateDish)
	mux.HandleFunc("DELETE /dishes/{id}", a.deleteDish)
	return router{mux: mux}
}

func (a *api) root(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"service": "The Orc Shack API", "status": "running"})
}

func (a *api) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *api) listDishes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.dishes.List())
}

func (a *api) getDish(w http.ResponseWriter, r *http.Request) {
	d, ok := a.dishes.Get(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, errDishNotFound)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (a *api) createDish(w http.ResponseWriter, r *http.Request) {
	input, ok := decodeDish(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusCreated, a.dishes.Create(input))
}

func (a *api) updateDish(w http.ResponseWriter, r *http.Request) {
	input, ok := decodeDish(w, r)
	if !ok {
		return
	}
	updated, found := a.dishes.Update(r.PathValue("id"), input)
	if !found {
		writeProblem(w, http.StatusNotFound, errDishNotFound)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (a *api) deleteDish(w http.ResponseWriter, r *http.Request) {
	if !a.dishes.Delete(r.PathValue("id")) {
		writeProblem(w, http.StatusNotFound, errDishNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

const errDishNotFound = "dish not found"

// decodeDish reads and validates a dish from the request body. On failure it
// writes a 400 problem response and returns false.
func decodeDish(w http.ResponseWriter, r *http.Request) (dish.Dish, bool) {
	var input dish.Dish
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid JSON")
		return dish.Dish{}, false
	}
	if err := dish.Validate(input); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return dish.Dish{}, false
	}
	return input, true
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
