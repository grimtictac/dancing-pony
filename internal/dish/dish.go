// Package dish contains the dish model, its validation rules and an
// in-memory store.
package dish

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// DefaultCurrency is the only supported currency until restaurants
// (tenants) exist and carry their own currency.
const DefaultCurrency = "ZAR"

// Money is an amount in the currency's minor unit (e.g. cents) plus its
// ISO 4217 code. Integers avoid floating-point rounding errors.
type Money struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type Dish struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       Money  `json:"price"`
	Image       string `json:"image"`
}

// Validate returns the first problem found with d, or nil if it is valid.
func Validate(d Dish) error {
	if strings.TrimSpace(d.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(d.Description) == "" {
		return errors.New("description is required")
	}
	if d.Price.Amount <= 0 {
		return errors.New("price.amount must be a positive whole number of minor units")
	}
	if d.Price.Currency == "" {
		return errors.New("price.currency is required")
	}
	if d.Price.Currency != DefaultCurrency {
		return fmt.Errorf("price.currency must be %s", DefaultCurrency)
	}
	if strings.TrimSpace(d.Image) == "" {
		return errors.New("image is required")
	}
	return nil
}

// Seed returns the dishes the API starts with.
func Seed() []Dish {
	return []Dish{
		{
			ID:          "1",
			Name:        "Seed Cake",
			Description: "A warm hobbit-style honey cake with butter and seeds.",
			Price:       Money{Amount: 8500, Currency: DefaultCurrency},
			Image:       "https://example.com/seed-cake.jpg",
		},
		{
			ID:          "2",
			Name:        "Mushroom Pie",
			Description: "A savoury pie filled with woodland mushrooms and herbs.",
			Price:       Money{Amount: 12000, Currency: DefaultCurrency},
			Image:       "https://example.com/mushroom-pie.jpg",
		},
	}
}

// MemoryStore is an in-memory dish store that is safe for concurrent use.
type MemoryStore struct {
	mu     sync.RWMutex
	dishes map[string]Dish
	nextID int
}

// NewMemoryStore returns a store containing the given dishes. New dishes get
// IDs above the highest numeric seed ID, so they never collide.
func NewMemoryStore(seed ...Dish) *MemoryStore {
	s := &MemoryStore{dishes: make(map[string]Dish, len(seed)), nextID: 1}
	for _, d := range seed {
		s.dishes[d.ID] = d
		if n, err := strconv.Atoi(d.ID); err == nil && n >= s.nextID {
			s.nextID = n + 1
		}
	}
	return s
}

// List returns all dishes in no particular order.
func (s *MemoryStore) List() []Dish {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]Dish, 0, len(s.dishes))
	for _, d := range s.dishes {
		items = append(items, d)
	}
	return items
}

// Get returns the dish with the given ID, and whether it exists.
func (s *MemoryStore) Get(id string) (Dish, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	d, ok := s.dishes[id]
	return d, ok
}

// Create stores d under a new, never-reused ID and returns the stored dish.
func (s *MemoryStore) Create(d Dish) Dish {
	s.mu.Lock()
	defer s.mu.Unlock()

	d.ID = strconv.Itoa(s.nextID)
	s.nextID++
	s.dishes[d.ID] = d
	return d
}

// Update replaces the dish with the given ID. It reports false if the dish
// does not exist.
func (s *MemoryStore) Update(id string, d Dish) (Dish, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.dishes[id]; !ok {
		return Dish{}, false
	}
	d.ID = id
	s.dishes[id] = d
	return d, true
}

// Delete removes the dish with the given ID. It reports false if the dish
// does not exist.
func (s *MemoryStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.dishes[id]; !ok {
		return false
	}
	delete(s.dishes, id)
	return true
}
