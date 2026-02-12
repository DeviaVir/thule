package storage

import (
	"testing"
	"time"
)

func TestDedupeFromEnvDisabled(t *testing.T) {
	t.Setenv("THULE_DEDUPE", "disabled")
	cfg, err := DedupeFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil || cfg.Enabled {
		t.Fatalf("expected disabled config, got %+v", cfg)
	}
}

func TestDedupeFromEnvMemoryDefault(t *testing.T) {
	t.Setenv("THULE_DEDUPE", "")
	t.Setenv("THULE_QUEUE", "memory")
	t.Setenv("THULE_DEDUPE_TTL", "2m")
	cfg, err := DedupeFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Store == nil || !cfg.Enabled || cfg.StoreLabel != "memory" {
		t.Fatalf("expected memory store enabled, got %+v", cfg)
	}
	if cfg.TTL != 2*time.Minute {
		t.Fatalf("expected ttl 2m, got %v", cfg.TTL)
	}
}

func TestDedupeFromEnvInvalidMode(t *testing.T) {
	t.Setenv("THULE_DEDUPE", "bad")
	if _, err := DedupeFromEnv(); err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestDedupeFromEnvInvalidTTL(t *testing.T) {
	t.Setenv("THULE_DEDUPE", "memory")
	t.Setenv("THULE_DEDUPE_TTL", "nope")
	if _, err := DedupeFromEnv(); err == nil {
		t.Fatal("expected error for invalid ttl")
	}
}

func TestDedupeFromEnvInvalidRedisDB(t *testing.T) {
	t.Setenv("THULE_DEDUPE", "redis")
	t.Setenv("THULE_REDIS_ADDR", "127.0.0.1:6379")
	t.Setenv("THULE_REDIS_DB", "nope")
	if _, err := DedupeFromEnv(); err == nil {
		t.Fatal("expected error for invalid redis db")
	}
}

func TestGetEnvHelpers(t *testing.T) {
	t.Setenv("THULE_DEDUPE_TTL", "")
	if got := getEnv("THULE_DEDUPE_TTL", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}
	t.Setenv("THULE_REDIS_DB", "7")
	got, err := getEnvInt("THULE_REDIS_DB", 0)
	if err != nil || got != 7 {
		t.Fatalf("expected 7, got %d err=%v", got, err)
	}
	t.Setenv("THULE_REDIS_DB", "bad")
	if _, err := getEnvInt("THULE_REDIS_DB", 0); err == nil {
		t.Fatal("expected error for invalid int")
	}
}
