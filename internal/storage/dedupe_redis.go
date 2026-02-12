package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisDedupeStore struct {
	client *redis.Client
	prefix string
}

func NewRedisDedupeStore(client *redis.Client, prefix string) *RedisDedupeStore {
	if prefix == "" {
		prefix = "thule:dedupe:"
	}
	return &RedisDedupeStore{client: client, prefix: prefix}
}

func (s *RedisDedupeStore) Reserve(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		return true, nil
	}
	ok, err := s.client.SetNX(ctx, s.prefix+key, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("redis reserve: %w", err)
	}
	return ok, nil
}

func (s *RedisDedupeStore) Release(ctx context.Context, key string) error {
	if err := s.client.Del(ctx, s.prefix+key).Err(); err != nil {
		return fmt.Errorf("redis release: %w", err)
	}
	return nil
}
