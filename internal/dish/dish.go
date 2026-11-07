// Package dish contains the dish model and its validation rules.
package dish

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Maximum lengths, in characters. The database enforces the same limits.
const (
	MaxNameLength        = 200
	MaxDescriptionLength = 2000
	MaxImageLength       = 2048
)

// Money is an amount in the currency's minor unit (e.g. cents) plus its
// ISO 4217 code. Integers avoid floating-point rounding errors.
type Money struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// Dish is an item on a restaurant's menu. ID is the 22-character base62
// form of its UUID.
type Dish struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       Money  `json:"price"`
	Image       string `json:"image"`
	Rating      Rating `json:"rating"` // read-only: derived from reviews
	Photo       *Photo `json:"photo"`  // read-only: set by uploading; null when none
}

// Photo is a dish's uploaded photo. Image is a URL the client supplies;
// a photo is uploaded, processed and served by the API itself.
//
// Width, Height and the placeholders let a client lay out and preview
// the photo before it loads; how (or whether) to show them is up to the
// client.
type Photo struct {
	URL    string `json:"url"` // relative to the API's base URL
	Width  int    `json:"width"`
	Height int    `json:"height"`
	// ThumbHash is a ~25-byte blurred preview, base64 (evanw.github.io/thumbhash).
	ThumbHash string `json:"thumbhash"`
	// Placeholder is a tiny PNG data URL any client can show as-is.
	Placeholder string `json:"placeholder"`
}

// Rating summarises a dish's reviews. Average is null when there are none.
type Rating struct {
	Average *float64 `json:"average"`
	Count   int64    `json:"count"`
}

// Validate returns the first problem found with d, or nil if it is valid
// for a restaurant whose prices are in currency.
func Validate(d Dish, currency string) error {
	if err := text("name", d.Name, MaxNameLength); err != nil {
		return err
	}
	if err := text("description", d.Description, MaxDescriptionLength); err != nil {
		return err
	}
	if d.Price.Amount <= 0 {
		return errors.New("price.amount must be a positive whole number of minor units")
	}
	if d.Price.Currency == "" {
		return errors.New("price.currency is required")
	}
	if d.Price.Currency != currency {
		return fmt.Errorf("price.currency must be %s", currency)
	}
	return text("image", d.Image, MaxImageLength)
}

func text(field, value string, maxLength int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if utf8.RuneCountInString(value) > maxLength {
		return fmt.Errorf("%s must be at most %d characters", field, maxLength)
	}
	return nil
}
