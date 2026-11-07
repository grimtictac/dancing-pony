package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
)

type Dish struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Image       string  `json:"image"`
}

var (
	dishes = map[string]Dish{
		"1": {
			ID:          "1",
			Name:        "Seed Cake",
			Description: "A warm hobbit-style honey cake with butter and seeds.",
			Price:       8.5,
			Image:       "https://example.com/seed-cake.jpg",
		},
		"2": {
			ID:          "2",
			Name:        "Mushroom Pie",
			Description: "A savory pie filled with woodland mushrooms and herbs.",
			Price:       12.0,
			Image:       "https://example.com/mushroom-pie.jpg",
		},
	}
	dishesMu sync.RWMutex
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/dishes", dishesHandler)
	mux.HandleFunc("/dishes/", dishHandler)
	mux.HandleFunc("/", rootHandler)

	log.Println("The Orc Shack API listening on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"service": "The Orc Shack API", "status": "running"})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func dishesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listDishes(w, r)
	case http.MethodPost:
		createDish(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func dishHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/dishes/")
	if id == "" || id == "/dishes/" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		getDishByID(w, r, id)
	case http.MethodPut:
		updateDish(w, r, id)
	case http.MethodDelete:
		deleteDish(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func listDishes(w http.ResponseWriter, r *http.Request) {
	dishesMu.RLock()
	defer dishesMu.RUnlock()

	items := make([]Dish, 0, len(dishes))
	for _, dish := range dishes {
		items = append(items, dish)
	}

	writeJSON(w, http.StatusOK, items)
}

func getDishByID(w http.ResponseWriter, r *http.Request, id string) {
	dishesMu.RLock()
	defer dishesMu.RUnlock()

	dish, ok := dishes[id]
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, dish)
}

func createDish(w http.ResponseWriter, r *http.Request) {
	var input Dish
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := validateDishInput(input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id := fmt.Sprintf("%d", len(dishes)+1)
	input.ID = id

	dishesMu.Lock()
	defer dishesMu.Unlock()
	dishes[id] = input

	writeJSON(w, http.StatusCreated, input)
}

func updateDish(w http.ResponseWriter, r *http.Request, id string) {
	var input Dish
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if err := validateDishInput(input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	dishesMu.Lock()
	defer dishesMu.Unlock()

	current, ok := dishes[id]
	if !ok {
		http.NotFound(w, r)
		return
	}

	input.ID = current.ID
	dishes[id] = input
	writeJSON(w, http.StatusOK, input)
}

func deleteDish(w http.ResponseWriter, r *http.Request, id string) {
	dishesMu.Lock()
	defer dishesMu.Unlock()

	if _, exists := dishes[id]; !exists {
		http.NotFound(w, r)
		return
	}

	delete(dishes, id)
	w.WriteHeader(http.StatusNoContent)
}

func validateDishInput(input Dish) error {
	if strings.TrimSpace(input.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(input.Description) == "" {
		return errors.New("description is required")
	}
	if input.Price <= 0 {
		return errors.New("price must be greater than zero")
	}
	if strings.TrimSpace(input.Image) == "" {
		return errors.New("image is required")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("failed to write JSON: %v", err)
	}
}
