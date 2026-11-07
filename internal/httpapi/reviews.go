package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"the-dancing-pony-v2/internal/review"
	"the-dancing-pony-v2/internal/sentiment"
	"the-dancing-pony-v2/internal/store"
	"the-dancing-pony-v2/internal/tenant"
)

const errReviewNotFound = "review not found"

// putReview handles PUT /restaurants/{slug}/dishes/{id}/reviews/me: create
// or replace the caller's review. PUT fits because each user has exactly
// one review per dish, so repeating the request has the same effect.
func (a *api) putReview(w http.ResponseWriter, r *http.Request, rest tenant.Restaurant) {
	dishID, ok := pathDishID(w, r)
	if !ok {
		return
	}
	userID := currentUser(r.Context()).ID
	in, ok := decodeJSON[review.Input](w, r)
	if !ok {
		return
	}
	if err := review.Validate(in); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}

	var sent *sentiment.Sentiment
	if strings.TrimSpace(in.Body) != "" {
		// Scored inline: the word-list scorer takes microseconds (D5). A
		// failing scorer shouldn't lose the review, so it's stored unscored.
		s, err := a.scorer.Score(r.Context(), in.Body)
		if err != nil {
			slog.WarnContext(r.Context(), "sentiment scoring failed", "err", err)
		} else {
			sent = &s
		}
	}

	rv, created, err := a.store.PutReview(r.Context(), rest.ID, dishID, userID, in, sent)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, errDishNotFound)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	label := ""
	if sent != nil {
		label = sent.Label
	}
	a.metrics.reviewWritten(created, label)

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, rv)
}

// deleteReview handles DELETE /restaurants/{slug}/dishes/{id}/reviews/me.
func (a *api) deleteReview(w http.ResponseWriter, r *http.Request, rest tenant.Restaurant) {
	dishID, ok := pathDishID(w, r)
	if !ok {
		return
	}
	userID := currentUser(r.Context()).ID
	err := a.store.DeleteReview(r.Context(), rest.ID, dishID, userID)
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, errReviewNotFound)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listReviews handles GET /restaurants/{slug}/dishes/{id}/reviews: the most
// recent reviews first (limit 1-100, default 20).
func (a *api) listReviews(w http.ResponseWriter, r *http.Request, rest tenant.Restaurant) {
	dishID, ok := pathDishID(w, r)
	if !ok {
		return
	}
	limit, err := intParam(r.URL.Query(), "limit")
	if err != nil || limit > store.MaxLimit {
		writeProblem(w, http.StatusBadRequest, "limit must be a whole number from 1 to 100")
		return
	}
	if limit == 0 {
		limit = store.DefaultLimit
	}
	reviews, err := a.store.ListReviews(r.Context(), rest.ID, dishID, int(limit))
	if errors.Is(err, store.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, errDishNotFound)
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, reviews)
}
