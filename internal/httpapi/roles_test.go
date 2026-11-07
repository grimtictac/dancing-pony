package httpapi

import (
	"net/http"
	"testing"
)

func TestRolesAtARestaurant(t *testing.T) {
	h := newAPI(t)
	cake := shackDishes + "/" + dishNamed(t, h, "orc-shack", "Seed Cake").ID
	tests := []struct {
		name, user, method, path, body string
		want                           int
	}{
		{"customer views the menu", "frodo", http.MethodGet, shackDishes, "", http.StatusOK},
		{"customer can't add a dish", "frodo", http.MethodPost, shackDishes, validDishJSON, http.StatusForbidden},
		{"customer can't delete a dish", "frodo", http.MethodDelete, cake, "", http.StatusForbidden},
		{"owner adds a dish", "orc-owner", http.MethodPost, shackDishes, validDishJSON, http.StatusCreated},
		{"owner can't review their own restaurant", "orc-owner", http.MethodPut, cake + "/reviews/me", validReviewJSON, http.StatusForbidden},
		{"another restaurant's owner is a customer here", "dragon-owner", http.MethodPost, shackDishes, validDishJSON, http.StatusForbidden},
		{"another restaurant's owner may review here", "dragon-owner", http.MethodPut, cake + "/reviews/me", validReviewJSON, http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := doAs(t, h, tt.user, tt.method, tt.path, tt.body); rec.Code != tt.want {
				t.Errorf("status %d, want %d; body %s", rec.Code, tt.want, rec.Body)
			}
		})
	}
}
