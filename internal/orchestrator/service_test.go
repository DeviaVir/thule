package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/example/thule/internal/lock"
	"github.com/example/thule/internal/queue"
	"github.com/example/thule/internal/storage"
	"github.com/example/thule/internal/vcs"
)

func baseEvent() MergeRequestEvent {
	return MergeRequestEvent{DeliveryID: "d1", EventType: "merge_request.updated", Repository: "org/repo", MergeReqID: 99, HeadSHA: "abc", ChangedFiles: []string{"apps/payments/deploy.yaml"}}
}

func TestHandleMergeRequestEventQueuesJob(t *testing.T) {
	jobs := queue.NewMemoryQueue(1)
	store := storage.NewMemoryDeliveryStore()
	svc := New(jobs, store, lock.NewMemoryLocker(), vcs.NewMemoryApprover())
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
	svc := New(jobs, store, lock.NewMemoryLocker(), vcs.NewMemoryApprover())

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
	svc := New(jobs, store, lock.NewMemoryLocker(), vcs.NewMemoryApprover())
	event := baseEvent()

	if err := svc.HandleMergeRequestEvent(context.Background(), event); err != nil {
		t.Fatalf("first event failed: %v", err)
	}
	if err := svc.HandleMergeRequestEvent(context.Background(), event); err != nil {
		t.Fatalf("duplicate event should be ignored, got err: %v", err)
	}
}

func TestHandleMergeRequestEventReleasesReservationOnEnqueueFailureAndRequestsChanges(t *testing.T) {
	jobs := queue.NewMemoryQueue(1)
	store := storage.NewMemoryDeliveryStore()
	approver := vcs.NewMemoryApprover()
	svc := New(jobs, store, lock.NewMemoryLocker(), approver)

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
	if approvals := approver.ListApprovals(event.MergeReqID); len(approvals) == 0 || approvals[0].Decision != vcs.DecisionRequestChanges {
		t.Fatalf("expected request changes approval after enqueue failure: %+v", approvals)
	}
}

func TestHandleMergeRequestEventProjectLockConflictRequestsChanges(t *testing.T) {
	jobs := queue.NewMemoryQueue(2)
	store := storage.NewMemoryDeliveryStore()
	locker := lock.NewMemoryLocker()
	approver := vcs.NewMemoryApprover()
	svc := New(jobs, store, locker, approver)

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
	if approvals := approver.ListApprovals(2); len(approvals) == 0 || approvals[0].Decision != vcs.DecisionRequestChanges {
		t.Fatalf("expected request changes on lock conflict: %+v", approvals)
	}
}

func TestHandleMergeRequestEventCloseReleasesLocks(t *testing.T) {
	jobs := queue.NewMemoryQueue(2)
	store := storage.NewMemoryDeliveryStore()
	locker := lock.NewMemoryLocker()
	svc := New(jobs, store, locker, vcs.NewMemoryApprover())

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
