package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/example/thule/internal/lock"
	"github.com/example/thule/internal/project"
	"github.com/example/thule/internal/queue"
	"github.com/example/thule/internal/storage"
)

type MergeRequestEvent struct {
	DeliveryID   string   `json:"delivery_id"`
	EventType    string   `json:"event_type"`
	Repository   string   `json:"repository"`
	MergeReqID   int64    `json:"merge_request_id"`
	HeadSHA      string   `json:"head_sha"`
	ChangedFiles []string `json:"changed_files,omitempty"`
}

type Service struct {
	jobs   queue.Queue
	store  storage.DeliveryStore
	locker lock.Locker
}

func New(jobs queue.Queue, store storage.DeliveryStore, locker lock.Locker) *Service {
	return &Service{jobs: jobs, store: store, locker: locker}
}

func (s *Service) HandleMergeRequestEvent(ctx context.Context, event MergeRequestEvent) error {
	if event.DeliveryID == "" {
		return fmt.Errorf("delivery_id is required")
	}
	if event.EventType == "" || event.Repository == "" || event.HeadSHA == "" || event.MergeReqID <= 0 {
		return fmt.Errorf("missing required event fields")
	}

	if !s.store.Reserve(event.DeliveryID) {
		return nil
	}

	if isCloseEvent(event.EventType) {
		if s.locker != nil {
			s.locker.ReleaseByMR(event.Repository, event.MergeReqID)
		}
		s.store.Commit(event.DeliveryID)
		return nil
	}

	if s.locker != nil {
		for _, p := range project.DiscoverFromChangedFiles(event.ChangedFiles) {
			ok, owner := s.locker.Acquire(event.Repository, p.Root, event.MergeReqID)
			if !ok {
				s.store.Release(event.DeliveryID)
				return fmt.Errorf("project %q is locked by MR !%d", p.Root, owner)
			}
		}
	}

	if err := s.jobs.Enqueue(ctx, queue.Job{
		DeliveryID:   event.DeliveryID,
		EventType:    event.EventType,
		Repository:   event.Repository,
		MergeReqID:   event.MergeReqID,
		HeadSHA:      event.HeadSHA,
		ChangedFiles: event.ChangedFiles,
	}); err != nil {
		s.store.Release(event.DeliveryID)
		return err
	}

	s.store.Commit(event.DeliveryID)
	return nil
}

func isCloseEvent(eventType string) bool {
	e := strings.ToLower(eventType)
	return strings.Contains(e, "closed") || strings.Contains(e, "merged")
}
