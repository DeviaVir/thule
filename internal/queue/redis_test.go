package queue

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestNewRedisQueueDefaultKey(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	q := NewRedisQueue(client, "")
	if q.key != "thule:jobs" {
		t.Fatalf("expected default key, got %s", q.key)
	}
}

func TestRedisQueueDequeueRejectsInvalidPayload(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	q := NewRedisQueue(client, "thule:jobs")

	if err := client.RPush(context.Background(), q.key, "not-json").Err(); err != nil {
		t.Fatalf("rpush failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := q.Dequeue(ctx); err == nil || !strings.Contains(err.Error(), "unmarshal") {
		t.Fatalf("expected unmarshal error, got %v", err)
	}
}
