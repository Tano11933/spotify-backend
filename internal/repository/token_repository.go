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

func refreshTokenKey(userID uuid.UUID) string {
	return fmt.Sprintf("refresh_token:%s", userID)
}

func resetTokenKey(token string) string {
	return fmt.Sprintf("reset_token:%s", token)
}

func (r *TokenRepository) StoreRefreshToken(
	ctx context.Context,
	userID uuid.UUID,
	token string,
	ttl time.Duration,
) error {
	return r.rdb.Set(ctx, refreshTokenKey(userID), token, ttl).Err()
}

func (r *TokenRepository) GetRefreshToken(ctx context.Context, userID uuid.UUID) (string, error) {
	token, err := r.rdb.Get(ctx, refreshTokenKey(userID)).Result()

	if errors.Is(err, redis.Nil) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return token, nil
}

func (r *TokenRepository) DeleteRefreshToken(ctx context.Context, userID uuid.UUID) error {
	return r.rdb.Del(ctx, refreshTokenKey(userID)).Err()
}

var rotateRefreshToken = redis.NewScript(`
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
return 1
`)

func (r *TokenRepository) RotateRefreshToken(ctx context.Context, userID uuid.UUID, previous, next string, ttl time.Duration) (bool, error) {
	if ttl.Milliseconds() <= 0 {
		return false, fmt.Errorf("refresh TTL must be positive")
	}
	result, err := rotateRefreshToken.Run(ctx, r.rdb, []string{refreshTokenKey(userID)}, previous, next, ttl.Milliseconds()).Int()
	return result == 1, err
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
