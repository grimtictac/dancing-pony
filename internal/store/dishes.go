package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"the-dancing-pony-v2/internal/dish"
	"the-dancing-pony-v2/internal/id"
	"the-dancing-pony-v2/internal/store/sqlcgen"
)

// GetDish returns one of a restaurant's dishes, or ErrNotFound.
func (s *Store) GetDish(ctx context.Context, restaurantID, dishID uuid.UUID) (dish.Dish, error) {
	var r sqlcgen.Dish
	err := s.inTenant(ctx, restaurantID, pgx.ReadOnly, func(q *sqlcgen.Queries) (err error) {
		r, err = q.GetDish(ctx, sqlcgen.GetDishParams{RestaurantID: restaurantID, ID: dishID})
		return err
	})
	return result(r, err, "get dish")
}

// CreateDish adds a dish to a restaurant. Any ID in d is ignored.
func (s *Store) CreateDish(ctx context.Context, restaurantID uuid.UUID, d dish.Dish) (dish.Dish, error) {
	var r sqlcgen.Dish
	err := s.inTenant(ctx, restaurantID, pgx.ReadWrite, func(q *sqlcgen.Queries) (err error) {
		r, err = q.CreateDish(ctx, sqlcgen.CreateDishParams{
			RestaurantID:  restaurantID,
			Name:          d.Name,
			Description:   d.Description,
			PriceAmount:   d.Price.Amount,
			PriceCurrency: d.Price.Currency,
			ImageUrl:      d.Image,
		})
		return err
	})
	return result(r, err, "create dish")
}

// UpdateDish replaces one of a restaurant's dishes, or returns ErrNotFound.
// Any ID in d is ignored.
func (s *Store) UpdateDish(ctx context.Context, restaurantID, dishID uuid.UUID, d dish.Dish) (dish.Dish, error) {
	var r sqlcgen.Dish
	err := s.inTenant(ctx, restaurantID, pgx.ReadWrite, func(q *sqlcgen.Queries) (err error) {
		r, err = q.UpdateDish(ctx, sqlcgen.UpdateDishParams{
			RestaurantID:  restaurantID,
			ID:            dishID,
			Name:          d.Name,
			Description:   d.Description,
			PriceAmount:   d.Price.Amount,
			PriceCurrency: d.Price.Currency,
			ImageUrl:      d.Image,
		})
		return err
	})
	return result(r, err, "update dish")
}

// DeleteDish removes one of a restaurant's dishes, or returns ErrNotFound.
func (s *Store) DeleteDish(ctx context.Context, restaurantID, dishID uuid.UUID) error {
	var n int64
	err := s.inTenant(ctx, restaurantID, pgx.ReadWrite, func(q *sqlcgen.Queries) (err error) {
		n, err = q.DeleteDish(ctx, sqlcgen.DeleteDishParams{RestaurantID: restaurantID, ID: dishID})
		return err
	})
	if err != nil {
		return fmt.Errorf("delete dish: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func result(r sqlcgen.Dish, err error, op string) (dish.Dish, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return dish.Dish{}, ErrNotFound
	}
	if err != nil {
		return dish.Dish{}, fmt.Errorf("%s: %w", op, err)
	}
	return toDish(r), nil
}

func toDish(r sqlcgen.Dish) dish.Dish {
	return dish.Dish{
		ID:          id.Encode(r.ID),
		Name:        r.Name,
		Description: r.Description,
		Price:       dish.Money{Amount: r.PriceAmount, Currency: r.PriceCurrency},
		Image:       r.ImageUrl,
	}
}
