package storage

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type DedupeConfig struct {
	Store      DedupeStore
	TTL        time.Duration
	Enabled    bool
	StoreLabel string
}

func DedupeFromEnv() (*DedupeConfig, error) {
	mode := strings.ToLower(getEnv("THULE_DEDUPE", "auto"))
	queueMode := strings.ToLower(getEnv("THULE_QUEUE", "memory"))
	if mode == "auto" {
		if queueMode == "redis" {
			mode = "redis"
		} else {
			mode = "memory"
		}
	}
	if mode == "disabled" || mode == "off" || mode == "false" {
		return &DedupeConfig{Enabled: false}, nil
	}

	ttl, err := time.ParseDuration(getEnv("THULE_DEDUPE_TTL", "5m"))
	if err != nil {
		return nil, fmt.Errorf("invalid THULE_DEDUPE_TTL: %w", err)
	}

	switch mode {
	case "redis":
		addr := getEnv("THULE_REDIS_ADDR", "127.0.0.1:6379")
		password := os.Getenv("THULE_REDIS_PASSWORD")
		db, err := getEnvInt("THULE_REDIS_DB", 0)
		if err != nil {
			return nil, err
		}
		prefix := getEnv("THULE_REDIS_DEDUPE_PREFIX", "thule:dedupe:")
		client := redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db})
		return &DedupeConfig{Store: NewRedisDedupeStore(client, prefix), TTL: ttl, Enabled: true, StoreLabel: "redis"}, nil
	case "memory":
		return &DedupeConfig{Store: NewMemoryDedupeStore(), TTL: ttl, Enabled: true, StoreLabel: "memory"}, nil
	default:
		return nil, fmt.Errorf("invalid THULE_DEDUPE: %s", mode)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) (int, error) {
	val := os.Getenv(key)
	if val == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", key, err)
	}
	return n, nil
}
