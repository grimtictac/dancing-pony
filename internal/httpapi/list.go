package httpapi

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"the-dancing-pony-v2/internal/store"
	"the-dancing-pony-v2/internal/tenant"
)

// listDishes handles GET /restaurants/{slug}/dishes with optional query
// parameters: q (search), min_price, max_price, sort, limit and cursor.
//
// The body stays a plain JSON array. When there is a next page, its URL is
// in a Link header (RFC 8288), as GitHub's API does:
//
//	Link: </restaurants/orc-shack/dishes?cursor=...&limit=20>; rel="next"
func (a *api) listDishes(w http.ResponseWriter, r *http.Request, rest tenant.Restaurant) {
	q := r.URL.Query()
	opts := store.ListOptions{Search: q.Get("q"), Sort: q.Get("sort"), Cursor: q.Get("cursor")}

	// Go note: "if x, err = f(); err != nil" runs a statement before the
	// condition. Checking err straight after each call is Go's error
	// handling: there are no exceptions.
	var err error
	if opts.MinPrice, err = intParam(q, "min_price"); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if opts.MaxPrice, err = intParam(q, "max_price"); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := intParam(q, "limit")
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	opts.Limit = int(limit)

	dishes, next, err := a.store.ListDishes(r.Context(), rest.ID, opts)
	if errors.Is(err, store.ErrInvalidListOptions) {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}

	if next != "" {
		q.Set("cursor", next)
		nextURL := url.URL{Path: r.URL.Path, RawQuery: q.Encode()}
		w.Header().Set("Link", "<"+nextURL.String()+`>; rel="next"`)
	}
	writeJSON(w, http.StatusOK, dishes)
}

// intParam parses an optional non-negative integer query parameter; absent
// means 0.
func intParam(q url.Values, name string) (int64, error) {
	v := q.Get(name)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0, errors.New(name + " must be a non-negative whole number")
	}
	return n, nil
}
