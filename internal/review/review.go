// Package review contains the review model and its validation rules.
package review

import (
	"errors"
	"time"
	"unicode/utf8"
)

// MaxBodyLength is the longest review text, in characters. The database
// enforces the same limit.
const MaxBodyLength = 2000

// Go note: the `json:"..."` strings after each field are struct tags. The
// encoding/json package reads them to choose the JSON key; omitempty leaves
// the key out when the value is empty (here: no text, or no sentiment).

// Review is one user's review of one dish.
type Review struct {
	ID        string     `json:"id"`
	DishID    string     `json:"dish_id"`
	Rating    int        `json:"rating"`
	Body      string     `json:"body,omitempty"`
	Sentiment *Sentiment `json:"sentiment,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Go note: a pointer field (*Sentiment) doubles as "optional": nil means
// there is none, here because a review without text isn't scored.

// Sentiment is the scored tone of a review's text.
type Sentiment struct {
	Score float64 `json:"score"`
	Label string  `json:"label"`
	Model string  `json:"model"`
}

// Input is what a client sends to create or replace their review.
type Input struct {
	Rating int    `json:"rating"`
	Body   string `json:"body"`
}

// Validate returns the first problem with in, or nil if it is valid.
func Validate(in Input) error {
	if in.Rating < 1 || in.Rating > 5 {
		return errors.New("rating must be a whole number from 1 to 5")
	}
	if utf8.RuneCountInString(in.Body) > MaxBodyLength {
		return errors.New("body must be at most 2000 characters")
	}
	return nil
}
