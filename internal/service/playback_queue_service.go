package service

import (
	"context"

	"github.com/google/uuid"
	"spotify-backend/internal/model"
	"spotify-backend/pkg/pagination"
)

func (s *PlayerService) GetQueueEntries(ctx context.Context, userID uuid.UUID, params pagination.Params) (pagination.Page[model.QueueEntry], error) {
	entries, total, err := s.repo.FindQueueEntries(ctx, userID, params.Limit, params.Offset)
	if err != nil {
		return pagination.Page[model.QueueEntry]{}, err
	}
	return pagination.NewPage(entries, total, params), nil
}

func (s *PlayerService) ReserveQueueEntry(ctx context.Context, userID uuid.UUID, entryID uint64) (*model.QueueReservation, error) {
	return s.repo.ReserveQueueEntry(ctx, userID, entryID)
}

func (s *PlayerService) ConfirmQueueEntry(ctx context.Context, userID uuid.UUID, reservationID uuid.UUID) error {
	return s.repo.ConfirmQueueEntry(ctx, userID, reservationID)
}

func (s *PlayerService) ReleaseQueueEntry(ctx context.Context, userID uuid.UUID, reservationID uuid.UUID) error {
	return s.repo.ReleaseQueueEntry(ctx, userID, reservationID)
}

func (s *PlayerService) RemoveQueueEntry(ctx context.Context, userID uuid.UUID, entryID uint64) error {
	return s.repo.RemoveQueueEntry(ctx, userID, entryID)
}
