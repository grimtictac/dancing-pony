package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"the-dancing-pony-v2/internal/review"
	"the-dancing-pony-v2/internal/sentiment"
	"the-dancing-pony-v2/internal/store"
	"the-dancing-pony-v2/internal/tenant"
)

// demoUserHeader names the reviewer until real login exists (Round 4).
// It is NOT authentication: anyone can claim any name. It only lets the
// review endpoints be built and tried with several "users" now; Round 4
// replaces it with the session's user.
const demoUserHeader = "X-Demo-User"

// demoUserNamespace turns a demo user name into a stable UUID (UUIDv5).
var demoUserNamespace = uuid.MustParse("6f1d1f8e-3c2a-4b7e-9d65-2a1c0e5b7f10")

const errReviewNotFound = "review not found"

func demoUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	name := strings.TrimSpace(r.Header.Get(demoUserHeader))
	if name == "" || len(name) > 64 {
		writeProblem(w, http.StatusBadRequest, demoUserHeader+" header (1-64 characters) is required until login exists")
		return uuid.Nil, false
	}
	return uuid.NewSHA1(demoUserNamespace, []byte(name)), true
}

// putReview handles PUT /restaurants/{slug}/dishes/{id}/reviews/me: create
// or replace the caller's review. PUT fits because each user has exactly
// one review per dish, so repeating the request has the same effect.
func (a *api) putReview(w http.ResponseWriter, r *http.Request, rest tenant.Restaurant) {
	dishID, ok := pathDishID(w, r)
	if !ok {
		return
	}
	userID, ok := demoUser(w, r)
	if !ok {
		return
	}
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
	userID, ok := demoUser(w, r)
	if !ok {
		return
	}
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
