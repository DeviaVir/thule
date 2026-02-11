package orchestrator

import (
	"context"
	"fmt"

	"github.com/example/thule/internal/queue"
	"github.com/example/thule/internal/storage"
)

type MergeRequestEvent struct {
	DeliveryID string `json:"delivery_id"`
	EventType  string `json:"event_type"`
	Repository string `json:"repository"`
	MergeReqID int64  `json:"merge_request_id"`
	HeadSHA    string `json:"head_sha"`
}

type Service struct {
	jobs  queue.Queue
	store storage.DeliveryStore
}

func New(jobs queue.Queue, store storage.DeliveryStore) *Service {
	return &Service{jobs: jobs, store: store}
}

func (s *Service) HandleMergeRequestEvent(ctx context.Context, event MergeRequestEvent) error {
	if event.DeliveryID == "" {
		return fmt.Errorf("delivery_id is required")
	}
	if event.EventType == "" || event.Repository == "" || event.HeadSHA == "" || event.MergeReqID <= 0 {
		return fmt.Errorf("missing required event fields")
	}

	if s.store.Seen(event.DeliveryID) {
		return nil
	}

	s.store.MarkSeen(event.DeliveryID)
	return s.jobs.Enqueue(ctx, queue.Job{
		DeliveryID: event.DeliveryID,
		EventType:  event.EventType,
		Repository: event.Repository,
		MergeReqID: event.MergeReqID,
		HeadSHA:    event.HeadSHA,
	})
}
