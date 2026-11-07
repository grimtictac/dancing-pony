package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"time"

	"github.com/google/uuid"
)

// SessionTTL is how long a session token stays valid after login.
const SessionTTL = 30 * 24 * time.Hour

// User is an authenticated person. Users are global; what they may do at a
// restaurant depends on their membership there.
type User struct {
	ID            uuid.UUID
	Email         string
	PlatformAdmin bool
}

// NewSessionToken returns a new random session token (256 bits, base64url)
// for the client, and the hash to store. The token itself is never stored.
func NewSessionToken() (token string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

// HashToken returns the SHA-256 of a session token. A fast hash is fine
// here, unlike for passwords: the token is 256 random bits, so there is
// nothing to guess.
func HashToken(token string) []byte {
	// Go note: sha256.Sum256 returns a fixed-size array ([32]byte); sum[:]
	// slices it, giving a []byte view of the same memory.
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
