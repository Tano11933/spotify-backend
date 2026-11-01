package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type TokenRepository struct {
	rdb *redis.Client
}

func NewTokenRepository(rdb *redis.Client) *TokenRepository {
	return &TokenRepository{rdb: rdb}
}

func resetTokenKey(token string) string {
	return fmt.Sprintf("reset_token:%s", token)
}

func (r *TokenRepository) StoreResetToken(
	ctx context.Context,
	token string,
	userID uuid.UUID,
	ttl time.Duration,
) error {
	return r.rdb.Set(ctx, resetTokenKey(token), userID.String(), ttl).Err()
}

func (r *TokenRepository) ConsumeResetToken(ctx context.Context, token string) (uuid.UUID, error) {
	raw, err := r.rdb.GetDel(ctx, resetTokenKey(token)).Result()
	if errors.Is(err, redis.Nil) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}

	userID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, ErrNotFound
	}
	return userID, nil
}
