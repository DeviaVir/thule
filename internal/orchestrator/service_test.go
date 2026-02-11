package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/example/thule/internal/queue"
	"github.com/example/thule/internal/storage"
)

func baseEvent() MergeRequestEvent {
	return MergeRequestEvent{DeliveryID: "d1", EventType: "merge_request.updated", Repository: "org/repo", MergeReqID: 99, HeadSHA: "abc"}
}

func TestHandleMergeRequestEventQueuesJob(t *testing.T) {
	jobs := queue.NewMemoryQueue(1)
	store := storage.NewMemoryDeliveryStore()
	svc := New(jobs, store)
	event := baseEvent()

	if err := svc.HandleMergeRequestEvent(context.Background(), event); err != nil {
		t.Fatalf("handle event failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	job, err := jobs.Dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue failed: %v", err)
	}
	if job.DeliveryID != event.DeliveryID || job.MergeReqID != event.MergeReqID || job.HeadSHA != event.HeadSHA {
		t.Fatalf("unexpected job: %+v", job)
	}
}

func TestHandleMergeRequestEventRejectsInvalidEvents(t *testing.T) {
	jobs := queue.NewMemoryQueue(1)
	store := storage.NewMemoryDeliveryStore()
	svc := New(jobs, store)

	tests := []MergeRequestEvent{{}, {DeliveryID: "d", Repository: "org/repo", MergeReqID: 1, HeadSHA: "a"}}
	for _, tc := range tests {
		if err := svc.HandleMergeRequestEvent(context.Background(), tc); err == nil {
			t.Fatalf("expected error for event %+v", tc)
		}
	}
}

func TestHandleMergeRequestEventDeduplicatesByDeliveryID(t *testing.T) {
	jobs := queue.NewMemoryQueue(2)
	store := storage.NewMemoryDeliveryStore()
	svc := New(jobs, store)
	event := baseEvent()

	if err := svc.HandleMergeRequestEvent(context.Background(), event); err != nil {
		t.Fatalf("first event failed: %v", err)
	}
	if err := svc.HandleMergeRequestEvent(context.Background(), event); err != nil {
		t.Fatalf("duplicate event should be ignored, got err: %v", err)
	}
}

func TestHandleMergeRequestEventReleasesReservationOnEnqueueFailure(t *testing.T) {
	jobs := queue.NewMemoryQueue(1)
	store := storage.NewMemoryDeliveryStore()
	svc := New(jobs, store)

	if err := jobs.Enqueue(context.Background(), queue.Job{DeliveryID: "prefill"}); err != nil {
		t.Fatalf("prefill failed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	event := baseEvent()
	if err := svc.HandleMergeRequestEvent(ctx, event); err == nil {
		t.Fatal("expected enqueue failure")
	}
	if !store.Reserve(event.DeliveryID) {
		t.Fatal("delivery id should be reservable again after enqueue failure")
	}
}
