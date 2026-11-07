package dish

import (
	"sync"
	"testing"
)

func validDish(name string) Dish {
	return Dish{
		Name:        name,
		Description: "Tasty.",
		Price:       Money{Amount: 5000, Currency: DefaultCurrency},
		Image:       "https://example.com/" + name + ".jpg",
	}
}

func TestCreateAfterDeleteDoesNotOverwriteExistingDish(t *testing.T) {
	s := NewMemoryStore(Seed()...)
	pie, _ := s.Get("2")

	s.Delete("1")
	created := s.Create(validDish("lembas"))

	if created.ID == "2" {
		t.Fatalf("new dish reused existing ID %q", created.ID)
	}
	if got, _ := s.Get("2"); got != pie {
		t.Errorf("dish 2 = %+v, want unchanged %+v", got, pie)
	}
	if got := len(s.List()); got != 2 {
		t.Errorf("store has %d dishes, want 2", got)
	}
}

func TestConcurrentCreatesGetUniqueIDs(t *testing.T) {
	s := NewMemoryStore()
	const n = 100

	ids := make(chan string, n)
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids <- s.Create(validDish("stew")).ID
		}()
	}
	wg.Wait()
	close(ids)

	seen := make(map[string]bool, n)
	for id := range ids {
		if seen[id] {
			t.Fatalf("duplicate ID %q", id)
		}
		seen[id] = true
	}
	if got := len(s.List()); got != n {
		t.Errorf("store has %d dishes, want %d", got, n)
	}
}
