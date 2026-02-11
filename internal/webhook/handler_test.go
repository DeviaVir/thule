package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/example/thule/internal/lock"
	"github.com/example/thule/internal/orchestrator"
	"github.com/example/thule/internal/queue"
	"github.com/example/thule/internal/storage"
	"github.com/example/thule/internal/vcs"
)

func TestWebhookQueuesJobAndDeduplicatesDelivery(t *testing.T) {
	jobs := queue.NewMemoryQueue(2)
	store := storage.NewMemoryDeliveryStore()
	orch := orchestrator.New(jobs, store, lock.NewMemoryLocker(), vcs.NewMemoryApprover())
	h := NewHandler("", orch)

	payload := []byte(`{
		"delivery_id":"d-1",
		"event_type":"merge_request.updated",
		"repository":"org/repo",
		"merge_request_id":42,
		"head_sha":"abc123"
	}`)

	req1 := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
	rr1 := httptest.NewRecorder()
	h.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr1.Code)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	job, err := jobs.Dequeue(ctx)
	if err != nil {
		t.Fatalf("expected queued job: %v", err)
	}
	if job.MergeReqID != 42 || job.HeadSHA != "abc123" {
		t.Fatalf("unexpected job payload: %+v", job)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for duplicate delivery, got %d", rr2.Code)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	if _, err := jobs.Dequeue(ctx2); err == nil {
		t.Fatal("expected no second queued job for duplicate delivery")
	}
}

func TestWebhookSupportsGitLabMergeRequestAndCommandEvents(t *testing.T) {
	jobs := queue.NewMemoryQueue(3)
	store := storage.NewMemoryDeliveryStore()
	orch := orchestrator.New(jobs, store, lock.NewMemoryLocker(), vcs.NewMemoryApprover())
	h := NewHandler("", orch)

	mrPayload := []byte(`{
		"object_kind":"merge_request",
		"event_id":"evt-1",
		"project":{"path_with_namespace":"group/repo"},
		"changed_files":["apps/p1/deploy.yaml"],
		"object_attributes":{"iid":7,"last_commit":{"id":"sha777"}}
	}`)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(mrPayload)))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	job, err := jobs.Dequeue(ctx)
	if err != nil || job.MergeReqID != 7 || job.EventType != "merge_request.updated" {
		t.Fatalf("unexpected MR job: %+v err=%v", job, err)
	}

	cmdPayload := []byte(`{
		"object_kind":"note",
		"event_id":"evt-2",
		"project":{"path_with_namespace":"group/repo"},
		"merge_request":{"iid":7,"last_commit":"sha777"},
		"object_attributes":{"note":"/thule plan"}
	}`)
	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(cmdPayload)))
	if rr2.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for /thule command, got %d", rr2.Code)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	job2, err := jobs.Dequeue(ctx2)
	if err != nil || job2.EventType != "comment.plan" {
		t.Fatalf("unexpected command job: %+v err=%v", job2, err)
	}
}

func TestWebhookRejectsInvalidMethod(t *testing.T) {
	h := NewHandler("", orchestrator.New(queue.NewMemoryQueue(1), storage.NewMemoryDeliveryStore(), lock.NewMemoryLocker(), vcs.NewMemoryApprover()))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)

	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestWebhookRejectsInvalidPayload(t *testing.T) {
	h := NewHandler("", orchestrator.New(queue.NewMemoryQueue(1), storage.NewMemoryDeliveryStore(), lock.NewMemoryLocker(), vcs.NewMemoryApprover()))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString("not-json"))

	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestWebhookSignatureValidation(t *testing.T) {
	jobs := queue.NewMemoryQueue(1)
	h := NewHandler("supersecret", orchestrator.New(jobs, storage.NewMemoryDeliveryStore(), lock.NewMemoryLocker(), vcs.NewMemoryApprover()))

	payload := []byte(`{
		"delivery_id":"d-2",
		"event_type":"merge_request.updated",
		"repository":"org/repo",
		"merge_request_id":43,
		"head_sha":"def456"
	}`)

	mac := hmac.New(sha256.New, []byte("supersecret"))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	t.Run("valid-signature", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
		req.Header.Set("X-Thule-Signature", sig)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusAccepted {
			t.Fatalf("expected 202, got %d", rr.Code)
		}
	})

	t.Run("invalid-signature", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload))
		req.Header.Set("X-Thule-Signature", "sha256=deadbeef")
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})
}

func TestWebhookParsesGitLabCloseEventType(t *testing.T) {
	jobs := queue.NewMemoryQueue(1)
	store := storage.NewMemoryDeliveryStore()
	orch := orchestrator.New(jobs, store, lock.NewMemoryLocker(), vcs.NewMemoryApprover())
	h := NewHandler("", orch)

	payload := []byte(`{
		"object_kind":"merge_request",
		"event_id":"evt-close",
		"project":{"path_with_namespace":"group/repo"},
		"object_attributes":{"iid":7,"action":"close","last_commit":{"id":"sha777"}}
	}`)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(payload)))
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := jobs.Dequeue(ctx); err == nil {
		t.Fatal("expected no queued job for close event")
	}
}
