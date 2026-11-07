// Package httpapi exposes the dish store as a JSON HTTP API.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"the-dancing-pony-v2/internal/dish"
)

type api struct {
	dishes *dish.MemoryStore
}

// New returns the HTTP handler for the API.
func New(dishes *dish.MemoryStore) http.Handler {
	a := &api{dishes: dishes}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", a.health)
	mux.HandleFunc("/dishes", a.dishCollection)
	mux.HandleFunc("/dishes/", a.dishItem)
	mux.HandleFunc("/", a.root)
	return mux
}

func (a *api) root(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"service": "The Orc Shack API", "status": "running"})
}

func (a *api) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *api) dishCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, a.dishes.List())
	case http.MethodPost:
		a.createDish(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *api) dishItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/dishes/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		a.getDish(w, r, id)
	case http.MethodPut:
		a.updateDish(w, r, id)
	case http.MethodDelete:
		a.deleteDish(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *api) getDish(w http.ResponseWriter, r *http.Request, id string) {
	d, ok := a.dishes.Get(id)
	if !ok {
		http.NotFound(w, r)
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

func (a *api) updateDish(w http.ResponseWriter, r *http.Request, id string) {
	input, ok := decodeDish(w, r)
	if !ok {
		return
	}
	updated, found := a.dishes.Update(id, input)
	if !found {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (a *api) deleteDish(w http.ResponseWriter, r *http.Request, id string) {
	if !a.dishes.Delete(id) {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// decodeDish reads and validates a dish from the request body. On failure it
// writes a 400 response and returns false.
func decodeDish(w http.ResponseWriter, r *http.Request) (dish.Dish, bool) {
	var input dish.Dish
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return dish.Dish{}, false
	}
	if err := dish.Validate(input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return dish.Dish{}, false
	}
	return input, true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("failed to write JSON response", "err", err)
	}
}
