package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/example/thule/internal/lock"
	"github.com/example/thule/internal/queue"
	"github.com/example/thule/internal/storage"
)

func baseEvent() MergeRequestEvent {
	return MergeRequestEvent{DeliveryID: "d1", EventType: "merge_request.updated", Repository: "org/repo", MergeReqID: 99, HeadSHA: "abc", ChangedFiles: []string{"apps/payments/deploy.yaml"}}
}

func TestHandleMergeRequestEventQueuesJob(t *testing.T) {
	jobs := queue.NewMemoryQueue(1)
	store := storage.NewMemoryDeliveryStore()
	svc := New(jobs, store, lock.NewMemoryLocker(), storage.NewMemoryDedupeStore(), time.Minute)
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
	svc := New(jobs, store, lock.NewMemoryLocker(), storage.NewMemoryDedupeStore(), time.Minute)

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
	svc := New(jobs, store, lock.NewMemoryLocker(), storage.NewMemoryDedupeStore(), time.Minute)
	event := baseEvent()

	if err := svc.HandleMergeRequestEvent(context.Background(), event); err != nil {
		t.Fatalf("first event failed: %v", err)
	}
	if err := svc.HandleMergeRequestEvent(context.Background(), event); err != nil {
		t.Fatalf("duplicate event should be ignored, got err: %v", err)
	}
}

func TestHandleMergeRequestEventDeduplicatesByKey(t *testing.T) {
	jobs := queue.NewMemoryQueue(2)
	store := storage.NewMemoryDeliveryStore()
	svc := New(jobs, store, lock.NewMemoryLocker(), storage.NewMemoryDedupeStore(), time.Minute)

	event := baseEvent()
	event.DeliveryID = "d1"
	if err := svc.HandleMergeRequestEvent(context.Background(), event); err != nil {
		t.Fatalf("first event failed: %v", err)
	}

	dupe := baseEvent()
	dupe.DeliveryID = "d2"
	if err := svc.HandleMergeRequestEvent(context.Background(), dupe); err != nil {
		t.Fatalf("duplicate event should be ignored, got err: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := jobs.Dequeue(ctx); err != nil {
		t.Fatalf("expected first job: %v", err)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()
	if _, err := jobs.Dequeue(ctx2); err == nil {
		t.Fatal("expected no second job for deduped event")
	}
}

func TestHandleMergeRequestEventReleasesReservationOnEnqueueFailure(t *testing.T) {
	jobs := queue.NewMemoryQueue(1)
	store := storage.NewMemoryDeliveryStore()
	svc := New(jobs, store, lock.NewMemoryLocker(), storage.NewMemoryDedupeStore(), time.Minute)

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

func TestHandleMergeRequestEventProjectLockConflict(t *testing.T) {
	jobs := queue.NewMemoryQueue(2)
	store := storage.NewMemoryDeliveryStore()
	locker := lock.NewMemoryLocker()
	svc := New(jobs, store, locker, storage.NewMemoryDedupeStore(), time.Minute)

	e1 := baseEvent()
	e1.DeliveryID = "evt1"
	e1.MergeReqID = 1
	if err := svc.HandleMergeRequestEvent(context.Background(), e1); err != nil {
		t.Fatalf("expected first lock success: %v", err)
	}

	e2 := baseEvent()
	e2.DeliveryID = "evt2"
	e2.MergeReqID = 2
	if err := svc.HandleMergeRequestEvent(context.Background(), e2); err == nil {
		t.Fatal("expected lock conflict error")
	}
}

func TestHandleMergeRequestEventCloseReleasesLocks(t *testing.T) {
	jobs := queue.NewMemoryQueue(2)
	store := storage.NewMemoryDeliveryStore()
	locker := lock.NewMemoryLocker()
	svc := New(jobs, store, locker, storage.NewMemoryDedupeStore(), time.Minute)

	e1 := baseEvent()
	e1.DeliveryID = "evt1"
	e1.MergeReqID = 10
	if err := svc.HandleMergeRequestEvent(context.Background(), e1); err != nil {
		t.Fatalf("expected lock acquired: %v", err)
	}

	closeEvt := MergeRequestEvent{DeliveryID: "evt-close", EventType: "merge_request.closed", Repository: "org/repo", MergeReqID: 10, HeadSHA: "abc"}
	if err := svc.HandleMergeRequestEvent(context.Background(), closeEvt); err != nil {
		t.Fatalf("expected close to release locks: %v", err)
	}

	e2 := baseEvent()
	e2.DeliveryID = "evt2"
	e2.MergeReqID = 20
	if err := svc.HandleMergeRequestEvent(context.Background(), e2); err != nil {
		t.Fatalf("expected lock available after close: %v", err)
	}
}
