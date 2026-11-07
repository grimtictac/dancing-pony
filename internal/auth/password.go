// Package auth holds the building blocks of authentication: password
// hashing, session tokens and login throttling. Cryptography comes from
// vetted libraries (golang.org/x/crypto/argon2, crypto/rand, crypto/subtle);
// only the flows are ours.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Password length limits, in characters. No composition rules (NIST
// SP 800-63B): length is what matters.
const (
	MinPasswordLength = 12
	MaxPasswordLength = 128
)

// argon2id parameters: OWASP's minimum recommendation (19 MiB, 2 passes,
// 1 thread). Memory per hash is the constraint on a 512 MB server, hence
// also the cap on concurrent hashing below.
const (
	argonMemoryKiB = 19 * 1024
	argonTime      = 2
	argonThreads   = 1
	saltLen        = 16
	keyLen         = 32
)

var (
	// ErrPasswordPolicy is returned for a password outside the length limits.
	ErrPasswordPolicy = fmt.Errorf("password must be %d to %d characters", MinPasswordLength, MaxPasswordLength)
	// ErrMalformedHash means a stored hash couldn't be parsed. Verification
	// fails closed: it is never treated as a match.
	ErrMalformedHash = errors.New("malformed password hash")
)

// hashSlots caps concurrent hashing, so a burst of logins can't allocate
// 19 MiB each without bound and run the server out of memory. Callers
// queue (respecting their context) instead.
var hashSlots = make(chan struct{}, 4)

// Go note: a send on a full buffered channel blocks, so a channel of
// capacity 4 works as a semaphore: send to acquire, receive to release.
// select waits for whichever case is ready first.
func acquire(ctx context.Context) error {
	select {
	case hashSlots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func release() { <-hashSlots }

// CheckPasswordPolicy returns ErrPasswordPolicy if pw is too short or long.
func CheckPasswordPolicy(pw string) error {
	if n := utf8.RuneCountInString(pw); n < MinPasswordLength || n > MaxPasswordLength {
		return ErrPasswordPolicy
	}
	return nil
}

// HashPassword returns an argon2id hash of pw in the self-describing PHC
// format: $argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>.
func HashPassword(ctx context.Context, pw string) (string, error) {
	if err := CheckPasswordPolicy(pw); err != nil {
		return "", err
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	if err := acquire(ctx); err != nil {
		return "", err
	}
	defer release()
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemoryKiB, argonThreads, keyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version,
		argonMemoryKiB, argonTime, argonThreads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// VerifyPassword reports whether pw matches the PHC-format hash encoded.
// A hash that doesn't parse returns ErrMalformedHash and false: it fails
// closed, never open.
func VerifyPassword(ctx context.Context, pw, encoded string) (bool, error) {
	p, err := parsePHC(encoded)
	if err != nil {
		return false, err
	}
	// Over-long passwords can't have been stored; reject before hashing so
	// they cost nothing.
	if utf8.RuneCountInString(pw) > MaxPasswordLength {
		return false, nil
	}
	if err := acquire(ctx); err != nil {
		return false, err
	}
	defer release()
	key := argon2.IDKey([]byte(pw), p.salt, p.time, p.memory, p.threads, uint32(len(p.key))) //nolint:gosec // len checked in parsePHC
	return subtle.ConstantTimeCompare(key, p.key) == 1, nil
}

var b64 = base64.RawStdEncoding

type phc struct {
	memory, time uint32
	threads      uint8
	salt, key    []byte
}

// parsePHC parses an argon2id PHC string strictly. Every field must be
// present and within bounds, so a corrupted or attacker-supplied hash can
// neither match by accident nor demand enormous memory.
func parsePHC(s string) (phc, error) {
	parts := strings.Split(s, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return phc{}, ErrMalformedHash
	}
	var m, t, p uint64
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil ||
		fmt.Sprintf("m=%d,t=%d,p=%d", m, t, p) != parts[3] {
		return phc{}, ErrMalformedHash
	}
	if m < 8*1024 || m > 256*1024 || t < 1 || t > 10 || p < 1 || p > 4 {
		return phc{}, ErrMalformedHash
	}
	salt, err := b64.DecodeString(parts[4])
	if err != nil || len(salt) < 16 {
		return phc{}, ErrMalformedHash
	}
	key, err := b64.DecodeString(parts[5])
	if err != nil || len(key) != keyLen {
		return phc{}, ErrMalformedHash
	}
	return phc{memory: uint32(m), time: uint32(t), threads: uint8(p), salt: salt, key: key}, nil
}

// DummyHash returns a valid hash of a random password, computed once. Login
// verifies against it when the email is unknown, so an unknown email takes
// as long to reject as a wrong password and response times don't reveal
// which accounts exist.
var DummyHash = sync.OnceValue(func() string {
	pw := make([]byte, 24)
	_, _ = rand.Read(pw)
	h, err := HashPassword(context.Background(), b64.EncodeToString(pw))
	if err != nil {
		panic(err)
	}
	return h
})
