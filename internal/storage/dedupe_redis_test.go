package storage

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisDedupeStoreReserveRelease(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	store := NewRedisDedupeStore(client, "thule:dedupe:")

	ctx := context.Background()
	ok, err := store.Reserve(ctx, "k1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("expected reserve ok, got ok=%v err=%v", ok, err)
	}
	ok, err = store.Reserve(ctx, "k1", time.Minute)
	if err != nil || ok {
		t.Fatalf("expected second reserve to fail, got ok=%v err=%v", ok, err)
	}
	if err := store.Release(ctx, "k1"); err != nil {
		t.Fatalf("release failed: %v", err)
	}
	ok, err = store.Reserve(ctx, "k1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("expected reserve after release ok, got ok=%v err=%v", ok, err)
	}
}

func TestDedupeFromEnvRedisAuto(t *testing.T) {
	srv := miniredis.RunT(t)
	t.Setenv("THULE_QUEUE", "redis")
	t.Setenv("THULE_REDIS_ADDR", srv.Addr())
	t.Setenv("THULE_DEDUPE_TTL", "30s")

	cfg, err := DedupeFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Store == nil || !cfg.Enabled || cfg.StoreLabel != "redis" {
		t.Fatalf("expected redis store enabled, got %+v", cfg)
	}
	if cfg.TTL != 30*time.Second {
		t.Fatalf("expected ttl 30s, got %v", cfg.TTL)
	}
}
