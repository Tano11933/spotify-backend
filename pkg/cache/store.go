package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Store struct {
	rdb *redis.Client
}

func NewStore(rdb *redis.Client) *Store {
	return &Store{rdb: rdb}
}

func (s *Store) GetJSON(ctx context.Context, key string, dest any) (bool, error) {
	raw, err := s.rdb.Get(ctx, key).Bytes()

	if errors.Is(err, redis.Nil) {
		return false, nil // cache miss, bukan kegagalan
	}
	if err != nil {
		return false, fmt.Errorf("cache get %s: %w", key, err)
	}

	if err := json.Unmarshal(raw, dest); err != nil {

		_ = s.rdb.Del(ctx, key).Err()
		return false, fmt.Errorf("cache unmarshal %s: %w", key, err)
	}

	return true, nil
}

func (s *Store) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache marshal %s: %w", key, err)
	}

	if err := s.rdb.Set(ctx, key, raw, ttl).Err(); err != nil {
		return fmt.Errorf("cache set %s: %w", key, err)
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	if err := s.rdb.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("cache delete: %w", err)
	}
	return nil
}
