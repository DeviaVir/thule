package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/example/thule/internal/queue"
	"github.com/example/thule/internal/storage"
)

func TestHandleMergeRequestEventQueuesJob(t *testing.T) {
	jobs := queue.NewMemoryQueue(1)
	store := storage.NewMemoryDeliveryStore()
	svc := New(jobs, store)

	event := MergeRequestEvent{
		DeliveryID: "d1",
		EventType:  "merge_request.updated",
		Repository: "org/repo",
		MergeReqID: 99,
		HeadSHA:    "abc",
	}

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

	tests := []MergeRequestEvent{
		{},
		{DeliveryID: "d", EventType: "", Repository: "org/repo", MergeReqID: 1, HeadSHA: "a"},
		{DeliveryID: "d", EventType: "merge_request.updated", Repository: "", MergeReqID: 1, HeadSHA: "a"},
		{DeliveryID: "d", EventType: "merge_request.updated", Repository: "org/repo", MergeReqID: 0, HeadSHA: "a"},
		{DeliveryID: "d", EventType: "merge_request.updated", Repository: "org/repo", MergeReqID: 1, HeadSHA: ""},
	}

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

	event := MergeRequestEvent{
		DeliveryID: "dup",
		EventType:  "merge_request.updated",
		Repository: "org/repo",
		MergeReqID: 1,
		HeadSHA:    "sha1",
	}

	if err := svc.HandleMergeRequestEvent(context.Background(), event); err != nil {
		t.Fatalf("first event failed: %v", err)
	}
	if err := svc.HandleMergeRequestEvent(context.Background(), event); err != nil {
		t.Fatalf("duplicate event should be ignored, got err: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := jobs.Dequeue(ctx); err != nil {
		t.Fatalf("expected first job in queue: %v", err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	if _, err := jobs.Dequeue(ctx2); err == nil {
		t.Fatal("expected no second job for duplicate delivery id")
	}
}
