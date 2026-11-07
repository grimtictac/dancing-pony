package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"the-dancing-pony-v2/internal/dish"
	"the-dancing-pony-v2/internal/store/sqlcgen"
)

// SeedRestaurant is a restaurant, and its starting menu, for pony seed.
type SeedRestaurant struct {
	Slug, Name, Currency string
	Dishes               []dish.Dish
}

// SeedResult reports what Seed did for one restaurant.
type SeedResult struct {
	Slug        string
	DishesAdded int
}

// DemoRestaurants are the development and demo tenants: The Orc Shack from
// the brief, and a second restaurant to show the two are kept apart.
var DemoRestaurants = []SeedRestaurant{
	{
		Slug: "orc-shack", Name: "The Orc Shack", Currency: "ZAR",
		Dishes: []dish.Dish{
			{
				Name:        "Seed Cake",
				Description: "A warm hobbit-style honey cake with butter and seeds.",
				Price:       dish.Money{Amount: 8500, Currency: "ZAR"},
				Image:       "https://example.com/seed-cake.jpg",
			},
			{
				Name:        "Mushroom Pie",
				Description: "A savoury pie filled with woodland mushrooms and herbs.",
				Price:       dish.Money{Amount: 12000, Currency: "ZAR"},
				Image:       "https://example.com/mushroom-pie.jpg",
			},
		},
	},
	{
		Slug: "green-dragon", Name: "The Green Dragon", Currency: "ZAR",
		Dishes: []dish.Dish{
			{
				Name:        "Second Breakfast",
				Description: "Eggs, bacon, sausages and fried bread, for the hobbit who has already eaten.",
				Price:       dish.Money{Amount: 9500, Currency: "ZAR"},
				Image:       "https://example.com/second-breakfast.jpg",
			},
		},
	},
}

// Seed adds restaurants, and their dishes if a restaurant has none yet, so
// it is safe to run repeatedly. dsn must connect as the schema owner:
// pony_app may not create restaurants.
func Seed(ctx context.Context, dsn string, restaurants []SeedRestaurant) ([]SeedResult, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	results := make([]SeedResult, 0, len(restaurants))
	for _, sr := range restaurants {
		// One transaction per restaurant, scoped by Row-Level Security like
		// the API's, which FORCE applies to the owner too.
		var added int
		err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			q := sqlcgen.New(tx)
			r, err := q.UpsertRestaurant(ctx, sqlcgen.UpsertRestaurantParams{
				Slug: sr.Slug, Name: sr.Name, Currency: sr.Currency,
			})
			if err != nil {
				return fmt.Errorf("upsert restaurant: %w", err)
			}
			if err := q.SetTenant(ctx, r.ID); err != nil {
				return fmt.Errorf("set tenant: %w", err)
			}
			n, err := q.CountDishes(ctx, r.ID)
			if err != nil || n > 0 {
				return err
			}
			for _, d := range sr.Dishes {
				if _, err := q.CreateDish(ctx, sqlcgen.CreateDishParams{
					RestaurantID:  r.ID,
					Name:          d.Name,
					Description:   d.Description,
					PriceAmount:   d.Price.Amount,
					PriceCurrency: d.Price.Currency,
					ImageUrl:      d.Image,
				}); err != nil {
					return fmt.Errorf("create dish %q: %w", d.Name, err)
				}
				added++
			}
			return nil
		})
		if err != nil {
			return results, errors.Join(fmt.Errorf("seed %s", sr.Slug), err)
		}
		results = append(results, SeedResult{Slug: sr.Slug, DishesAdded: added})
	}
	return results, nil
}
