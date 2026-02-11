package queue

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

func FromEnv() (Queue, error) {
	mode := strings.ToLower(getEnv("THULE_QUEUE", "memory"))
	if mode == "redis" {
		addr := getEnv("THULE_REDIS_ADDR", "127.0.0.1:6379")
		password := os.Getenv("THULE_REDIS_PASSWORD")
		db, err := getEnvInt("THULE_REDIS_DB", 0)
		if err != nil {
			return nil, err
		}
		key := getEnv("THULE_REDIS_QUEUE", "thule:jobs")
		client := redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db})
		return NewRedisQueue(client, key), nil
	}

	buffer, err := getEnvInt("THULE_QUEUE_BUFFER", 100)
	if err != nil {
		return nil, err
	}
	return NewMemoryQueue(buffer), nil
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
