package storage

import (
	"context"
	"time"
)

type DedupeStore interface {
	Reserve(ctx context.Context, key string, ttl time.Duration) (bool, error)
	Release(ctx context.Context, key string) error
}
