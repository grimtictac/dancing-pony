package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"the-dancing-pony-v2/internal/dish"
	"the-dancing-pony-v2/internal/id"
)

// dishSelect reads dishes with their rating summary. The LATERAL subquery
// runs once per dish row; Round 6 measures whether caching the aggregate
// on the dish is worth it.
const dishSelect = `SELECT d.restaurant_id, d.id, d.name, d.description, d.price_amount, d.price_currency,
       d.image_url, d.created_at, d.updated_at, r.rating_count, r.rating_avg
FROM dishes d
CROSS JOIN LATERAL (
    SELECT count(*) AS rating_count, round(avg(rv.rating), 2)::float8 AS rating_avg
    FROM reviews rv
    WHERE rv.restaurant_id = d.restaurant_id AND rv.dish_id = d.id
) r`

// dishRow is one row of dishSelect, in column order.
type dishRow struct {
	RestaurantID  uuid.UUID
	ID            uuid.UUID
	Name          string
	Description   string
	PriceAmount   int64
	PriceCurrency string
	ImageUrl      string //nolint:revive // matches sqlc's naming
	CreatedAt     time.Time
	UpdatedAt     time.Time
	RatingCount   int64
	RatingAvg     *float64 // NULL when there are no reviews
}

func getDishRow(ctx context.Context, tx pgx.Tx, restaurantID, dishID uuid.UUID) (dishRow, error) {
	rows, err := tx.Query(ctx, dishSelect+"\nWHERE d.restaurant_id = $1 AND d.id = $2", restaurantID, dishID)
	if err != nil {
		return dishRow{}, err
	}
	// Returns pgx.ErrNoRows when there is no such dish.
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[dishRow])
}

// Go note: a method is declared with a receiver before its name, (r dishRow);
// it is then called as r.toDish().
func (r dishRow) toDish() dish.Dish {
	return dish.Dish{
		ID:          id.Encode(r.ID),
		Name:        r.Name,
		Description: r.Description,
		Price:       dish.Money{Amount: r.PriceAmount, Currency: r.PriceCurrency},
		Image:       r.ImageUrl,
		Rating:      dish.Rating{Average: r.RatingAvg, Count: r.RatingCount},
	}
}
