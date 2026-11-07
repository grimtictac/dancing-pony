package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"the-dancing-pony-v2/internal/auth"
	"the-dancing-pony-v2/internal/id"
	"the-dancing-pony-v2/internal/store"
)

type userKey struct{}

func withUser(ctx context.Context, u auth.User) context.Context {
	return context.WithValue(ctx, userKey{}, u)
}

// currentUser returns the authenticated user. Handlers behind authed or
// inRestaurant can rely on it being set.
func currentUser(ctx context.Context) auth.User {
	u, _ := ctx.Value(userKey{}).(auth.User)
	return u
}

// errBadCredentials is deliberately the same for an unknown email and a
// wrong password, so the response doesn't reveal which accounts exist.
const errBadCredentials = "invalid email or password"

// authenticate resolves the Bearer token to a user, or writes a 401.
func (a *api) authenticate(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" || len(token) > 128 {
		unauthenticated(w)
		return auth.User{}, false
	}
	u, err := a.store.UserBySession(r.Context(), auth.HashToken(token))
	if errors.Is(err, store.ErrNotFound) {
		unauthenticated(w)
		return auth.User{}, false
	}
	if err != nil {
		internalError(w, r, err)
		return auth.User{}, false
	}
	info(r.Context()).User = id.Encode(u.ID)
	return u, true
}

func unauthenticated(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="pony"`)
	writeProblem(w, http.StatusUnauthorized, "log in and send the token as Authorization: Bearer <token>")
}

// authed wraps a handler that needs a logged-in user.
func (a *api) authed(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := a.authenticate(w, r)
		if !ok {
			return
		}
		h(w, r.WithContext(withUser(r.Context(), u)))
	}
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// normaliseEmail trims and lower-cases an address and checks it is a bare
// address (no display name).
func normaliseEmail(s string) (string, bool) {
	e := strings.ToLower(strings.TrimSpace(s))
	addr, err := mail.ParseAddress(e)
	if err != nil || addr.Address != e || len(e) > 254 {
		return "", false
	}
	return e, true
}

// register handles POST /auth/register.
func (a *api) register(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeJSON[credentials](w, r)
	if !ok {
		return
	}
	email, ok := normaliseEmail(in.Email)
	if !ok {
		writeProblem(w, http.StatusBadRequest, "email must be a valid address")
		return
	}
	hash, err := auth.HashPassword(r.Context(), in.Password)
	if errors.Is(err, auth.ErrPasswordPolicy) {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	u, err := a.store.CreateUser(r.Context(), email, hash)
	if errors.Is(err, store.ErrConflict) {
		// Reveals that the email is registered: a documented trade-off
		// (D10). Registration is throttled per IP to limit probing.
		writeProblem(w, http.StatusConflict, "email already registered")
		return
	}
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, userResponse{ID: id.Encode(u.ID), Email: u.Email})
}

// login handles POST /auth/login and returns a new session token.
func (a *api) login(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeJSON[credentials](w, r)
	if !ok {
		return
	}
	email, _ := normaliseEmail(in.Email)

	u, hash, err := a.store.UserByEmail(r.Context(), email)
	known := err == nil
	if errors.Is(err, store.ErrNotFound) {
		// Spend the same time as for a real user (see auth.DummyHash).
		hash = auth.DummyHash()
	} else if err != nil {
		internalError(w, r, err)
		return
	}
	match, err := auth.VerifyPassword(r.Context(), in.Password, hash)
	if errors.Is(err, auth.ErrMalformedHash) {
		// Fail closed, and make it visible: a stored hash should never be
		// malformed.
		info(r.Context()).Err = err
	} else if err != nil {
		internalError(w, r, err)
		return
	}
	if !known || !match {
		writeProblem(w, http.StatusUnauthorized, errBadCredentials)
		return
	}

	token, tokenHash, err := auth.NewSessionToken()
	if err != nil {
		internalError(w, r, err)
		return
	}
	expires := time.Now().Add(auth.SessionTTL)
	if err := a.store.CreateSession(r.Context(), u.ID, tokenHash, expires); err != nil {
		internalError(w, r, err)
		return
	}
	info(r.Context()).User = id.Encode(u.ID)
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "token_type": "Bearer", "expires_at": expires.UTC()})
}

// logout handles POST /auth/logout: it ends the current session.
func (a *api) logout(w http.ResponseWriter, r *http.Request) {
	_, token, _ := strings.Cut(r.Header.Get("Authorization"), " ")
	if err := a.store.DeleteSession(r.Context(), auth.HashToken(token)); err != nil {
		internalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// me handles GET /auth/me.
func (a *api) me(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r.Context())
	writeJSON(w, http.StatusOK, userResponse{ID: id.Encode(u.ID), Email: u.Email})
}
