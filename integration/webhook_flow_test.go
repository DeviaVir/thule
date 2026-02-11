package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/thule/internal/lock"
	"github.com/example/thule/internal/orchestrator"
	"github.com/example/thule/internal/queue"
	"github.com/example/thule/internal/storage"
	"github.com/example/thule/internal/webhook"
)

func TestWebhookToQueueIntegration(t *testing.T) {
	jobs := queue.NewMemoryQueue(2)
	store := storage.NewMemoryDeliveryStore()
	orch := orchestrator.New(jobs, store, lock.NewMemoryLocker())
	h := webhook.NewHandler("", orch)

	server := httptest.NewServer(h)
	defer server.Close()

	payload := map[string]any{
		"delivery_id":      "integration-1",
		"event_type":       "merge_request.updated",
		"repository":       "org/repo",
		"merge_request_id": 55,
		"head_sha":         "aaabbb",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	resp, err := http.Post(server.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post webhook: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	job, err := jobs.Dequeue(ctx)
	if err != nil {
		t.Fatalf("expected queued job: %v", err)
	}

	if job.DeliveryID != "integration-1" || job.MergeReqID != 55 {
		t.Fatalf("unexpected job: %+v", job)
	}
}
