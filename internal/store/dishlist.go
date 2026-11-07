package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"the-dancing-pony-v2/internal/dish"
	"the-dancing-pony-v2/internal/id"
)

// Page sizes for ListDishes.
const (
	DefaultLimit = 20
	MaxLimit     = 100
	maxSearchLen = 200
)

// ErrInvalidListOptions wraps every problem with ListOptions, so callers can
// report it as a client error.
var ErrInvalidListOptions = errors.New("invalid list options")

// ListOptions narrows, orders and pages a restaurant's dish list. The zero
// value lists the first page, oldest dish first.
type ListOptions struct {
	Search   string // full-text match on name and description, or a close match on the name
	MinPrice int64  // minor units; 0 means no minimum
	MaxPrice int64  // minor units; 0 means no maximum
	Sort     string // "" (oldest first), "name", "-name", "price" or "-price"
	Limit    int    // 0 means DefaultLimit
	Cursor   string // NextCursor from the previous page
}

// sortKey describes one allowed sort order. Only these columns can ever
// reach the SQL, so user input is never spliced into ORDER BY.
type sortKey struct {
	column string // empty: sort by id only
	cast   string // type of column, for the cursor comparison
	desc   bool
	value  func(dishRow) string // the row's sort value, for the cursor
}

// Go note: a map literal of struct values, keyed by the API's sort names.
// Anonymous functions are values too, so each entry carries its own getter.
var sortKeys = map[string]sortKey{
	"":       {},
	"name":   {column: "name", cast: "text", value: func(d dishRow) string { return d.Name }},
	"-name":  {column: "name", cast: "text", desc: true, value: func(d dishRow) string { return d.Name }},
	"price":  {column: "price_amount", cast: "bigint", value: priceValue},
	"-price": {column: "price_amount", cast: "bigint", desc: true, value: priceValue},
}

func priceValue(d dishRow) string { return strconv.FormatInt(d.PriceAmount, 10) }

// cursor marks where the previous page ended. It is opaque to clients:
// base64url-encoded JSON.
type cursor struct {
	Sort  string `json:"s"`
	Value string `json:"v,omitempty"`
	ID    string `json:"id"`
}

// ListDishes returns one page of a restaurant's dishes, and the cursor for
// the next page ("" on the last page).
func (s *Store) ListDishes(ctx context.Context, restaurantID uuid.UUID, opts ListOptions) ([]dish.Dish, string, error) {
	query, args, key, limit, err := buildDishList(restaurantID, opts)
	if err != nil {
		return nil, "", err
	}

	var rows []dishRow
	err = s.inTenantTx(ctx, restaurantID, pgx.ReadOnly, func(tx pgx.Tx) error {
		r, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		// Go note: CollectRows is generic. The type argument in [...] says
		// what each row becomes; RowToStructByPos fills the struct's fields
		// from the columns in order.
		rows, err = pgx.CollectRows(r, pgx.RowToStructByPos[dishRow])
		return err
	})
	if err != nil {
		return nil, "", fmt.Errorf("list dishes: %w", err)
	}

	var next string
	if len(rows) > limit { // we fetched one extra row to learn there is a next page
		rows = rows[:limit]
		last := rows[len(rows)-1]
		c := cursor{Sort: opts.Sort, ID: id.Encode(last.ID)}
		if key.value != nil {
			c.Value = key.value(last)
		}
		next = encodeCursor(c)
	}

	dishes := make([]dish.Dish, len(rows))
	for i, r := range rows {
		dishes[i] = r.toDish()
	}
	return dishes, next, nil
}

// buildDishList assembles the list query from fixed SQL fragments; user
// input only ever travels as bind parameters ($2, $3, ...).
func buildDishList(restaurantID uuid.UUID, opts ListOptions) (string, []any, sortKey, int, error) {
	key, ok := sortKeys[opts.Sort]
	if !ok {
		return "", nil, key, 0, fmt.Errorf("%w: sort must be one of name, -name, price, -price", ErrInvalidListOptions)
	}
	limit := opts.Limit
	switch {
	case limit == 0:
		limit = DefaultLimit
	case limit < 1 || limit > MaxLimit:
		return "", nil, key, 0, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidListOptions, MaxLimit)
	}
	if len(opts.Search) > maxSearchLen {
		return "", nil, key, 0, fmt.Errorf("%w: q must be at most %d characters", ErrInvalidListOptions, maxSearchLen)
	}
	if opts.MinPrice < 0 || opts.MaxPrice < 0 {
		return "", nil, key, 0, fmt.Errorf("%w: prices must not be negative", ErrInvalidListOptions)
	}

	// Go note: strings.Builder builds a string efficiently; args grows with
	// each bind parameter, and the arg closure returns its placeholder.
	var b strings.Builder
	args := []any{restaurantID}
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}

	b.WriteString(dishSelect + "\nWHERE d.restaurant_id = $1")
	if opts.Search != "" {
		p := arg(opts.Search)
		// Go note: %[1]s reuses the first argument, so p appears twice.
		// %% is a literal %, giving pg_trgm's "<%" (word similarity) operator.
		fmt.Fprintf(&b, "\n  AND (dish_search_vector(name, description) @@ websearch_to_tsquery('english', %[1]s)\n       OR %[1]s <%% name)", p)
	}
	if opts.MinPrice > 0 {
		b.WriteString("\n  AND price_amount >= " + arg(opts.MinPrice))
	}
	if opts.MaxPrice > 0 {
		b.WriteString("\n  AND price_amount <= " + arg(opts.MaxPrice))
	}

	cmp, dir := ">", ""
	if key.desc {
		cmp, dir = "<", " DESC"
	}
	if opts.Cursor != "" {
		c, err := decodeCursor(opts.Cursor)
		if err != nil || c.Sort != opts.Sort {
			return "", nil, key, 0, fmt.Errorf("%w: cursor is invalid or from a different sort", ErrInvalidListOptions)
		}
		after, err := id.Decode(c.ID)
		if err != nil {
			return "", nil, key, 0, fmt.Errorf("%w: cursor is invalid", ErrInvalidListOptions)
		}
		if key.column == "" {
			fmt.Fprintf(&b, "\n  AND id %s %s", cmp, arg(after))
		} else {
			// Row comparison: rows after (value, id) in this sort order.
			fmt.Fprintf(&b, "\n  AND (%s, id) %s (%s::text::%s, %s)", key.column, cmp, arg(c.Value), key.cast, arg(after))
		}
	}

	b.WriteString("\nORDER BY ")
	if key.column != "" {
		b.WriteString(key.column + dir + ", ")
	}
	b.WriteString("id" + dir)
	b.WriteString("\nLIMIT " + arg(limit+1))

	return b.String(), args, key, limit, nil
}

func encodeCursor(c cursor) string {
	// Go note: errors that can't happen are still returned by the API; the
	// blank identifier _ discards one explicitly. Marshal can't fail for
	// a struct of strings.
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (cursor, error) {
	var c cursor
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	return c, err
}
