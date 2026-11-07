package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPasswordPolicyBoundaries(t *testing.T) {
	tests := []struct {
		name string
		pw   string
		ok   bool
	}{
		{"empty", "", false},
		{"11 characters", strings.Repeat("a", 11), false},
		{"12 characters", strings.Repeat("a", 12), true},
		{"128 characters", strings.Repeat("a", 128), true},
		{"129 characters", strings.Repeat("a", 129), false},
		// CVE-2026-123158 accepted any 256-1024 character password.
		{"300 characters", strings.Repeat("a", 300), false},
		{"12 multibyte characters count as 12", strings.Repeat("é", 12), true},
		{"NUL bytes are just characters", "abc\x00def\x00ghij", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := CheckPasswordPolicy(tt.pw); (err == nil) != tt.ok {
				t.Errorf("CheckPasswordPolicy = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestHashAndVerify(t *testing.T) {
	ctx := context.Background()
	h, err := HashPassword(ctx, "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("hash %q is not PHC argon2id with the expected parameters", h)
	}
	if ok, err := VerifyPassword(ctx, "correct horse battery", h); !ok || err != nil {
		t.Errorf("right password: ok=%v err=%v", ok, err)
	}
	if ok, _ := VerifyPassword(ctx, "correct horse battery!", h); ok {
		t.Error("wrong password verified")
	}
	if ok, _ := VerifyPassword(ctx, strings.Repeat("a", 300), h); ok {
		t.Error("300-character password verified")
	}
	if h2, _ := HashPassword(ctx, "correct horse battery"); h2 == h {
		t.Error("two hashes of one password are equal: salt not random")
	}
}

// Malformed hashes must fail closed: never a match, always an error.
func TestVerifyFailsClosedOnMalformedHash(t *testing.T) {
	good, _ := HashPassword(context.Background(), "correct horse battery")
	parts := strings.Split(good, "$")
	for name, h := range map[string]string{
		"empty":            "",
		"plain text":       "correct horse battery",
		"bcrypt":           "$2a$10$abcdefghijklmnopqrstuuJ8Q5XRmWzP0o3hK1Wf8bE5mF8m5Ywe",
		"argon2i":          strings.Replace(good, "argon2id", "argon2i", 1),
		"other version":    strings.Replace(good, "v=19", "v=16", 1),
		"huge memory":      strings.Replace(good, "m=19456", "m=4194304", 1),
		"extra params":     strings.Replace(good, "p=1", "p=1,x=2", 1),
		"short key":        strings.Join(append(parts[:5:5], parts[5][:10]), "$"),
		"missing salt":     strings.Join(append(parts[:4:4], "", parts[5]), "$"),
		"trailing segment": good + "$",
		"non-base64 salt":  strings.Join(append(parts[:4:4], "!!!!", parts[5]), "$"),
	} {
		t.Run(name, func(t *testing.T) {
			ok, err := VerifyPassword(context.Background(), "correct horse battery", h)
			if ok || !errors.Is(err, ErrMalformedHash) {
				t.Errorf("ok=%v err=%v, want false and ErrMalformedHash", ok, err)
			}
		})
	}
}

func TestDummyHashIsValid(t *testing.T) {
	if ok, err := VerifyPassword(context.Background(), "anything at all", DummyHash()); ok || err != nil {
		t.Errorf("ok=%v err=%v, want a well-formed non-match", ok, err)
	}
}

func FuzzVerifyNeverPanics(f *testing.F) {
	f.Add("$argon2id$v=19$m=19456,t=2,p=1$c2FsdHNhbHRzYWx0c2FsdA$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGFzaGhhc2g")
	f.Fuzz(func(t *testing.T, h string) {
		if ok, err := VerifyPassword(context.Background(), "x", h); ok && err == nil {
			// Only a real hash of "x" could match, and "x" is below the
			// minimum length so none can exist.
			t.Fatalf("matched %q", h)
		}
	})
}
