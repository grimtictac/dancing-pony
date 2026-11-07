package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"the-dancing-pony-v2/internal/id"
	"the-dancing-pony-v2/internal/review"
	"the-dancing-pony-v2/internal/sentiment"
	"the-dancing-pony-v2/internal/store/sqlcgen"
)

// pgForeignKeyViolation is Postgres's SQLSTATE for a foreign key violation:
// here, a review of a dish that doesn't exist (in this restaurant).
const pgForeignKeyViolation = "23503"

// PutReview creates or replaces userID's review of a dish, and reports
// whether it was created. sent is nil when the review has no text.
func (s *Store) PutReview(ctx context.Context, restaurantID, dishID, userID uuid.UUID,
	in review.Input, sent *sentiment.Sentiment,
) (review.Review, bool, error) {
	p := sqlcgen.UpsertReviewParams{
		RestaurantID: restaurantID,
		DishID:       dishID,
		UserID:       userID,
		Rating:       int16(in.Rating), //nolint:gosec // validated to be 1-5
		Body:         in.Body,
	}
	if sent != nil {
		p.SentimentScore = pgtype.Float4{Float32: float32(sent.Score), Valid: true}
		p.SentimentLabel = pgtype.Text{String: sent.Label, Valid: true}
		p.SentimentModel = pgtype.Text{String: sent.Model, Valid: true}
	}

	var r sqlcgen.Review
	err := s.inTenant(ctx, restaurantID, pgx.ReadWrite, func(q *sqlcgen.Queries) (err error) {
		r, err = q.UpsertReview(ctx, p)
		return err
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation {
		return review.Review{}, false, ErrNotFound
	}
	if err != nil {
		return review.Review{}, false, fmt.Errorf("put review: %w", err)
	}
	// An insert sets both timestamps to the transaction's now(); an update
	// in a later transaction moves updated_at on.
	return toReview(r), r.CreatedAt.Equal(r.UpdatedAt), nil
}

// DeleteReview removes userID's review of a dish, or returns ErrNotFound.
func (s *Store) DeleteReview(ctx context.Context, restaurantID, dishID, userID uuid.UUID) error {
	var n int64
	err := s.inTenant(ctx, restaurantID, pgx.ReadWrite, func(q *sqlcgen.Queries) (err error) {
		n, err = q.DeleteReview(ctx, sqlcgen.DeleteReviewParams{RestaurantID: restaurantID, DishID: dishID, UserID: userID})
		return err
	})
	if err != nil {
		return fmt.Errorf("delete review: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListReviews returns a dish's most recent reviews, newest first, or
// ErrNotFound if the restaurant has no such dish.
func (s *Store) ListReviews(ctx context.Context, restaurantID, dishID uuid.UUID, limit int) ([]review.Review, error) {
	var rows []sqlcgen.Review
	err := s.inTenant(ctx, restaurantID, pgx.ReadOnly, func(q *sqlcgen.Queries) error {
		exists, err := q.DishExists(ctx, sqlcgen.DishExistsParams{RestaurantID: restaurantID, ID: dishID})
		if err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		rows, err = q.ListReviews(ctx, sqlcgen.ListReviewsParams{
			RestaurantID: restaurantID, DishID: dishID, Limit: int32(limit), //nolint:gosec // capped by the caller
		})
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("list reviews: %w", err)
	}
	reviews := make([]review.Review, len(rows))
	for i, r := range rows {
		reviews[i] = toReview(r)
	}
	return reviews, nil
}

func toReview(r sqlcgen.Review) review.Review {
	out := review.Review{
		ID:        id.Encode(r.ID),
		DishID:    id.Encode(r.DishID),
		Rating:    int(r.Rating),
		Body:      r.Body,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
	if r.SentimentScore.Valid {
		out.Sentiment = &review.Sentiment{
			Score: float64(r.SentimentScore.Float32),
			Label: r.SentimentLabel.String,
			Model: r.SentimentModel.String,
		}
	}
	return out
}
