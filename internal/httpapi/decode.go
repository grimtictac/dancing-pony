package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"the-dancing-pony-v2/internal/dish"
	"the-dancing-pony-v2/internal/tenant"
)

// maxBodyBytes caps every JSON request body, so a client can't make the
// server buffer an arbitrarily large payload.
const maxBodyBytes = 64 << 10 // 64 KiB

// Go note: decodeJSON is generic. [T any] declares a type parameter, so one
// function decodes any request type: decodeJSON[dish.Dish](w, r).
//
// decodeJSON reads exactly one JSON value of type T from the request body.
// Unknown fields, trailing data and oversized bodies are rejected. On
// failure it writes a problem response and returns false.
func decodeJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var v T
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields() // a typo like "prise" is an error, not silently ignored

	if err := dec.Decode(&v); err != nil {
		// Go note: errors.As checks whether err (or anything it wraps) is
		// of a given type, and if so fills in the variable.
		var tooBig *http.MaxBytesError
		switch {
		case errors.As(err, &tooBig):
			writeProblem(w, http.StatusRequestEntityTooLarge, "request body must be at most 64 KiB")
		case strings.HasPrefix(err.Error(), "json: unknown field"):
			writeProblem(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), "json: "))
		default:
			writeProblem(w, http.StatusBadRequest, "invalid JSON")
		}
		return v, false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProblem(w, http.StatusBadRequest, "invalid JSON: unexpected data after the object")
		return v, false
	}
	return v, true
}

// decodeDish reads and validates a dish from the request body. On failure it
// writes a problem response and returns false.
func decodeDish(w http.ResponseWriter, r *http.Request, rest tenant.Restaurant) (dish.Dish, bool) {
	input, ok := decodeJSON[dish.Dish](w, r)
	if !ok {
		return input, false
	}
	if err := dish.Validate(input, rest.Currency); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return dish.Dish{}, false
	}
	return input, true
}
