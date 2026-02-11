package queue

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestFromEnvDefaultsToMemory(t *testing.T) {
	q, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := q.(*MemoryQueue); !ok {
		t.Fatalf("expected MemoryQueue, got %T", q)
	}
}

func TestFromEnvMemoryBufferOverride(t *testing.T) {
	t.Setenv("THULE_QUEUE", "memory")
	t.Setenv("THULE_QUEUE_BUFFER", "5")
	q, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mq, ok := q.(*MemoryQueue)
	if !ok {
		t.Fatalf("expected MemoryQueue, got %T", q)
	}
	if cap(mq.ch) != 5 {
		t.Fatalf("expected buffer 5, got %d", cap(mq.ch))
	}
}

func TestFromEnvRedisQueue(t *testing.T) {
	mr := miniredis.RunT(t)
	t.Setenv("THULE_QUEUE", "redis")
	t.Setenv("THULE_REDIS_ADDR", mr.Addr())
	t.Setenv("THULE_REDIS_DB", "0")
	t.Setenv("THULE_REDIS_QUEUE", "thule:test")

	q, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rq, ok := q.(*RedisQueue)
	if !ok {
		t.Fatalf("expected RedisQueue, got %T", q)
	}

	job := Job{DeliveryID: "d1", EventType: "merge_request.updated", Repository: "org/repo", MergeReqID: 1, HeadSHA: "abc"}
	if err := rq.Enqueue(context.Background(), job); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := rq.Dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue failed: %v", err)
	}
	if out.DeliveryID != job.DeliveryID || out.HeadSHA != job.HeadSHA {
		t.Fatalf("unexpected job: %+v", out)
	}
}

func TestFromEnvRedisInvalidDB(t *testing.T) {
	t.Setenv("THULE_QUEUE", "redis")
	t.Setenv("THULE_REDIS_DB", "nope")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected error for invalid redis db")
	}
}
