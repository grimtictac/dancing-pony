// Package id converts UUIDs to and from the 22-character base62 form used
// in URLs, e.g. 0hUEdHXeXp4dQWVYVQzfWa instead of a 36-character UUID.
//
// The encoding is fixed-length and its alphabet is in ASCII order, so
// encoded IDs sort in the same order as the UUIDs; for UUIDv7 that is
// creation order. Every UUID has exactly one encoding, and Decode rejects
// anything else.
package id

import (
	"encoding/binary"
	"errors"
	"math/bits"

	"github.com/google/uuid"
)

// Length is the length of every encoded ID: 62^22 > 2^128 > 62^21.
const Length = 22

const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// ErrInvalid is returned by Decode for anything that is not an encoded UUID.
var ErrInvalid = errors.New("invalid id")

var digits = func() (d [256]int8) {
	for i := range d {
		d[i] = -1
	}
	for i := range len(alphabet) {
		d[alphabet[i]] = int8(i) //nolint:gosec // i < 62
	}
	return d
}()

// Encode returns the base62 form of u.
func Encode(u uuid.UUID) string {
	hi := binary.BigEndian.Uint64(u[:8])
	lo := binary.BigEndian.Uint64(u[8:])
	var buf [Length]byte
	for i := Length - 1; i >= 0; i-- {
		var r uint64
		hi, r = bits.Div64(0, hi, 62)
		lo, r = bits.Div64(r, lo, 62)
		buf[i] = alphabet[r]
	}
	return string(buf[:])
}

// Decode parses the base62 form produced by Encode.
func Decode(s string) (uuid.UUID, error) {
	if len(s) != Length {
		return uuid.Nil, ErrInvalid
	}
	var hi, lo uint64
	for i := range len(s) {
		d := digits[s[i]]
		if d < 0 {
			return uuid.Nil, ErrInvalid
		}
		// (hi, lo) = (hi, lo)*62 + d, failing if it no longer fits in 128 bits.
		h1, h0 := bits.Mul64(hi, 62)
		l1, l0 := bits.Mul64(lo, 62)
		var carry uint64
		lo, carry = bits.Add64(l0, uint64(d), 0)
		hi, carry = bits.Add64(h0, l1, carry)
		if h1 != 0 || carry != 0 {
			return uuid.Nil, ErrInvalid
		}
	}
	var u uuid.UUID
	binary.BigEndian.PutUint64(u[:8], hi)
	binary.BigEndian.PutUint64(u[8:], lo)
	return u, nil
}
