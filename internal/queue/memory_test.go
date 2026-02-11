package queue

import (
	"context"
	"testing"
	"time"
)

func TestMemoryQueueEnqueueDequeue(t *testing.T) {
	q := NewMemoryQueue(1)
	ctx := context.Background()
	want := Job{DeliveryID: "d1", EventType: "merge_request.updated", Repository: "org/repo", MergeReqID: 1, HeadSHA: "abc"}

	if err := q.Enqueue(ctx, want); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	got, err := q.Dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue failed: %v", err)
	}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestMemoryQueueRespectsCanceledContextOnEnqueue(t *testing.T) {
	q := NewMemoryQueue(1)
	if err := q.Enqueue(context.Background(), Job{DeliveryID: "1"}); err != nil {
		t.Fatalf("prefill enqueue failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := q.Enqueue(ctx, Job{DeliveryID: "2"}); err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestMemoryQueueRespectsCanceledContextOnDequeue(t *testing.T) {
	q := NewMemoryQueue(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	if _, err := q.Dequeue(ctx); err == nil {
		t.Fatal("expected context timeout/cancellation error")
	}
}
