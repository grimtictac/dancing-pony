package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSeedDishPriceIsInMinorUnits(t *testing.T) {
	rec := httptest.NewRecorder()
	dishHandler(rec, httptest.NewRequest(http.MethodGet, "/dishes/1", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var got Dish
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := Money{Amount: 850, Currency: "GBP"}
	if got.Price != want {
		t.Errorf("price = %+v, want %+v", got.Price, want)
	}
}

func TestCreateDishAcceptsMinorUnitPrice(t *testing.T) {
	body := `{"name":"Lembas","description":"Elven waybread.","price":{"amount":1250,"currency":"GBP"},"image":"https://example.com/lembas.jpg"}`
	rec := httptest.NewRecorder()
	dishesHandler(rec, httptest.NewRequest(http.MethodPost, "/dishes", strings.NewReader(body)))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body)
	}
	var got Dish
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := Money{Amount: 1250, Currency: "GBP"}
	if got.Price != want {
		t.Errorf("price = %+v, want %+v", got.Price, want)
	}
}

func TestCreateDishRejectsInvalidPrice(t *testing.T) {
	tests := []struct {
		name  string
		price string
	}{
		{"float price", `8.5`},
		{"fractional amount", `{"amount":8.5,"currency":"GBP"}`},
		{"zero amount", `{"amount":0,"currency":"GBP"}`},
		{"negative amount", `{"amount":-100,"currency":"GBP"}`},
		{"missing currency", `{"amount":850}`},
		{"unsupported currency", `{"amount":850,"currency":"USD"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := `{"name":"Stew","description":"Rabbit stew.","price":` + tt.price + `,"image":"https://example.com/stew.jpg"}`
			rec := httptest.NewRecorder()
			dishesHandler(rec, httptest.NewRequest(http.MethodPost, "/dishes", strings.NewReader(body)))

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusBadRequest, rec.Body)
			}
		})
	}
}
