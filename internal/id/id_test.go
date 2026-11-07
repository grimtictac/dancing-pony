package id

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
)

func TestKnownValues(t *testing.T) {
	tests := []struct {
		uuid string
		want string
	}{
		{"00000000-0000-0000-0000-000000000000", "0000000000000000000000"},
		{"00000000-0000-0000-0000-000000000001", "0000000000000000000001"},
		{"00000000-0000-0000-0000-00000000003d", "000000000000000000000z"}, // 61
		{"00000000-0000-0000-0000-00000000003e", "0000000000000000000010"}, // 62
		{"ffffffff-ffff-ffff-ffff-ffffffffffff", "7n42DGM5Tflk9n8mt7Fhc7"}, // 2^128-1
	}
	for _, tt := range tests {
		u := uuid.MustParse(tt.uuid)
		if got := Encode(u); got != tt.want {
			t.Errorf("Encode(%s) = %q, want %q", tt.uuid, got, tt.want)
		}
		if got, err := Decode(tt.want); err != nil || got != u {
			t.Errorf("Decode(%q) = %s, %v; want %s", tt.want, got, err, tt.uuid)
		}
	}
}

func TestDecodeRejectsInvalid(t *testing.T) {
	for _, s := range []string{
		"",
		"1",
		"000000000000000000000",   // 21 characters
		"00000000000000000000000", // 23 characters
		"000000000000000000000-",  // not in the alphabet
		"00000000000000000000 0",
		"7n42DGM5Tflk9n8mt7Fhc8",     // 2^128: one more than the largest UUID
		"zzzzzzzzzzzzzzzzzzzzzz",     // far larger than 128 bits
		"0000000000000000000\u00e90", // non-ASCII, 22 bytes
	} {
		if u, err := Decode(s); !errors.Is(err, ErrInvalid) {
			t.Errorf("Decode(%q) = %s, %v; want ErrInvalid", s, u, err)
		}
	}
}

func TestEncodingPreservesOrder(t *testing.T) {
	ids := make([]uuid.UUID, 1000)
	for i := range ids {
		ids[i] = uuid.New()
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	for i := 1; i < len(ids); i++ {
		if Encode(ids[i-1]) >= Encode(ids[i]) {
			t.Fatalf("Encode(%s) >= Encode(%s), want encodings in UUID order", ids[i-1], ids[i])
		}
	}
}

func FuzzRoundTrip(f *testing.F) {
	f.Add(make([]byte, 16))
	f.Add(bytes.Repeat([]byte{0xff}, 16))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) != 16 {
			return
		}
		u := uuid.UUID(b)
		s := Encode(u)
		if len(s) != Length {
			t.Fatalf("Encode(%s) = %q has length %d", u, s, len(s))
		}
		if got, err := Decode(s); err != nil || got != u {
			t.Fatalf("Decode(Encode(%s)) = %s, %v", u, got, err)
		}
	})
}

// Every string Decode accepts must be the one encoding of its UUID, so an ID
// can't be written two ways.
func FuzzDecodeIsCanonical(f *testing.F) {
	f.Add("0000000000000000000000")
	f.Add("7n42DGM5Tflk9n8mt7Fhc7")
	f.Add("7n42DGM5Tflk9n8mt7Fhc8")
	f.Fuzz(func(t *testing.T, s string) {
		u, err := Decode(s)
		if err != nil {
			return
		}
		if got := Encode(u); got != s {
			t.Fatalf("Decode(%q) = %s, which encodes as %q", s, u, got)
		}
	})
}
