package store

import (
	"context"
	"fmt"
)

// MarkUpdateProcessed records a Telegram update; first is false if it was
// already recorded, i.e. this is a redelivery.
func (s *Store) MarkUpdateProcessed(ctx context.Context, updateID int64) (first bool, err error) {
	n, err := s.q.MarkUpdateProcessed(ctx, updateID)
	if err != nil {
		return false, fmt.Errorf("mark update processed: %w", err)
	}
	return n == 1, nil
}

// UnmarkUpdate forgets an update whose handling failed.
func (s *Store) UnmarkUpdate(ctx context.Context, updateID int64) error {
	if err := s.q.UnmarkUpdate(ctx, updateID); err != nil {
		return fmt.Errorf("unmark update: %w", err)
	}
	return nil
}
