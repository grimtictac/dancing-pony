package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"the-dancing-pony-v2/internal/dish"
	"the-dancing-pony-v2/internal/review"
)

const validReviewJSON = `{"rating":5,"body":"Delicious, best pie in Bree!"}`

func TestReviews(t *testing.T) {
	h := newAPI(t)
	pie := dishNamed(t, h, "orc-shack", "Mushroom Pie")
	path := shackDishes + "/" + pie.ID + "/reviews"

	rec := doAs(t, h, "frodo", http.MethodPut, path+"/me", validReviewJSON)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first PUT: status %d, want 201; body %s", rec.Code, rec.Body)
	}
	created := decode[review.Review](t, rec)
	if created.Rating != 5 || created.Sentiment == nil || created.Sentiment.Label != "positive" {
		t.Errorf("created = %+v, want rating 5 with positive sentiment", created)
	}

	// Same user again: replaces the review rather than adding one.
	rec = doAs(t, h, "frodo", http.MethodPut, path+"/me", `{"rating":2,"body":"Cold and soggy."}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("second PUT: status %d, want 200", rec.Code)
	}
	if got := decode[review.Review](t, rec); got.ID != created.ID || got.Sentiment.Label != "negative" {
		t.Errorf("updated = %+v, want same id with negative sentiment", got)
	}

	// A review with no text has no sentiment.
	if rec := doAs(t, h, "sam", http.MethodPut, path+"/me", `{"rating":4}`); rec.Code != http.StatusCreated {
		t.Fatalf("sam's PUT: status %d", rec.Code)
	} else if got := decode[review.Review](t, rec); got.Sentiment != nil {
		t.Errorf("sentiment = %+v, want none for an empty body", got.Sentiment)
	}

	if rec := doAs(t, h, "", http.MethodPut, path+"/me", validReviewJSON); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous PUT: status %d, want 401", rec.Code)
	}

	list := decode[[]review.Review](t, do(t, h, http.MethodGet, path, ""))
	if len(list) != 2 {
		t.Errorf("listed %d reviews, want 2", len(list))
	}
	got := decode[dish.Dish](t, do(t, h, http.MethodGet, shackDishes+"/"+pie.ID, ""))
	if got.Rating.Count != 2 || got.Rating.Average == nil || *got.Rating.Average != 3 {
		t.Errorf("rating = %+v, want average 3 over 2 reviews", got.Rating)
	}

	if rec := doAs(t, h, "frodo", http.MethodDelete, path+"/me", ""); rec.Code != http.StatusNoContent {
		t.Errorf("DELETE: status %d, want 204", rec.Code)
	}
	if rec := doAs(t, h, "frodo", http.MethodDelete, path+"/me", ""); rec.Code != http.StatusNotFound {
		t.Errorf("second DELETE: status %d, want 404", rec.Code)
	}

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"rating too high": doAs(t, h, "frodo", http.MethodPut, path+"/me", `{"rating":6}`),
		"rating missing":  doAs(t, h, "frodo", http.MethodPut, path+"/me", `{"body":"hmm"}`),
	} {
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
	}
}
