package main

import (
	"context"
	"testing"
	"time"

	"github.com/example/thule/internal/orchestrator"
	"github.com/example/thule/internal/queue"
	"github.com/example/thule/internal/repo"
)

func TestRunWorkerProcessesJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobs := queue.NewMemoryQueue(1)
	job := queue.Job{
		DeliveryID: "d1",
		EventType:  "merge_request.updated",
		Repository: "org/repo",
		MergeReqID: 42,
		HeadSHA:    "abc123",
	}
	if err := jobs.Enqueue(context.Background(), job); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	called := false
	var got orchestrator.MergeRequestEvent
	err := runWorker(ctx, jobs, nil, func(_ context.Context, evt orchestrator.MergeRequestEvent) error {
		called = true
		got = evt
		cancel()
		return nil
	})

	if !called {
		t.Fatal("expected plan function to be called")
	}
	if got.MergeReqID != job.MergeReqID || got.HeadSHA != job.HeadSHA {
		t.Fatalf("unexpected event: %+v", got)
	}
	if err == nil {
		t.Fatal("expected worker to exit with context cancellation")
	}
}

func TestRunWorkerSyncErrorSkipsPlan(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	jobs := queue.NewMemoryQueue(1)
	job := queue.Job{DeliveryID: "d2", EventType: "merge_request.updated", Repository: "org/repo", MergeReqID: 7, HeadSHA: "def456"}
	if err := jobs.Enqueue(context.Background(), job); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	syncer := repo.NewSyncer("https://example.com/repo.git", "", "", nil)
	called := false
	err := runWorker(ctx, jobs, syncer, func(context.Context, orchestrator.MergeRequestEvent) error {
		called = true
		return nil
	})

	if called {
		t.Fatal("plan should not run when sync fails")
	}
	if err == nil {
		t.Fatal("expected worker to exit with error")
	}
}

func TestGetEnvFallback(t *testing.T) {
	if val := getEnv("THULE_WORKER_TEST_ENV", "fallback"); val != "fallback" {
		t.Fatalf("expected fallback, got %q", val)
	}
	t.Setenv("THULE_WORKER_TEST_ENV", "set")
	if val := getEnv("THULE_WORKER_TEST_ENV", "fallback"); val != "set" {
		t.Fatalf("expected env override, got %q", val)
	}
}

func TestBuildWorkerFromEnv(t *testing.T) {
	t.Setenv("THULE_QUEUE", "memory")
	t.Setenv("THULE_REPO_URL", "https://example.com/repo.git")
	t.Setenv("THULE_REPO_REF", "main")

	deps, err := buildWorker(t.TempDir())
	if err != nil {
		t.Fatalf("build worker failed: %v", err)
	}
	if deps.jobs == nil || deps.syncer == nil || deps.plan == nil {
		t.Fatal("expected worker dependencies to be set")
	}
	if !deps.syncer.Enabled() {
		t.Fatal("expected syncer to be enabled")
	}
}

func TestMainUsesRunWorkerFunc(t *testing.T) {
	t.Setenv("THULE_QUEUE", "memory")

	orig := runWorkerFunc
	t.Cleanup(func() { runWorkerFunc = orig })

	called := false
	runWorkerFunc = func(ctx context.Context, jobs queue.Queue, syncer *repo.Syncer, plan planFunc) error {
		called = true
		if jobs == nil || syncer == nil || plan == nil {
			t.Fatal("expected worker deps to be set")
		}
		return nil
	}

	main()

	if !called {
		t.Fatal("expected main to call runWorkerFunc")
	}
}
