package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"the-dancing-pony-v2/internal/dish"
	"the-dancing-pony-v2/internal/id"
	"the-dancing-pony-v2/internal/photo"
)

// dishSelect reads dishes with their rating summary and photo. The rating
// comes from running totals on the dish (migration 11), kept up to date by
// a trigger on reviews.
const dishSelect = `SELECT d.restaurant_id, d.id, d.name, d.description, d.price_amount, d.price_currency,
       d.image_url, d.created_at, d.updated_at, d.rating_count::bigint,
       CASE WHEN d.rating_count > 0
            THEN round(d.rating_sum::numeric / d.rating_count, 2)::float8 END AS rating_avg,
       p.key, p.width, p.height, p.thumbhash, p.data_url
FROM dishes d
LEFT JOIN dish_photos p ON p.restaurant_id = d.restaurant_id AND p.dish_id = d.id`

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
	// The photo columns are all NULL when the dish has no photo.
	PhotoKey       *string
	PhotoWidth     *int32
	PhotoHeight    *int32
	PhotoThumbHash *string
	PhotoDataURL   *string
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
	d := dish.Dish{
		ID:          id.Encode(r.ID),
		Name:        r.Name,
		Description: r.Description,
		Price:       dish.Money{Amount: r.PriceAmount, Currency: r.PriceCurrency},
		Image:       r.ImageUrl,
		Rating:      dish.Rating{Average: r.RatingAvg, Count: r.RatingCount},
	}
	if r.PhotoKey != nil {
		d.Photo = &dish.Photo{
			URL:         photo.URL(*r.PhotoKey),
			Width:       int(*r.PhotoWidth),
			Height:      int(*r.PhotoHeight),
			ThumbHash:   *r.PhotoThumbHash,
			Placeholder: *r.PhotoDataURL,
		}
	}
	return d
}
