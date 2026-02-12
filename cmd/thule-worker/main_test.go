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
	}, nil)

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
	}, nil)

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
	t.Setenv("THULE_GITLAB_TOKEN", "")

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
	t.Setenv("THULE_GITLAB_TOKEN", "")

	orig := runWorkerFunc
	t.Cleanup(func() { runWorkerFunc = orig })

	called := false
	runWorkerFunc = func(ctx context.Context, jobs queue.Queue, syncer *repo.Syncer, plan planFunc, mrChanges mrChangedFilesFunc) error {
		called = true
		if jobs == nil || syncer == nil || plan == nil {
			t.Fatal("expected worker deps to be set")
		}
		if mrChanges != nil {
			t.Fatal("expected no gitlab changed-files fallback in this test")
		}
		return nil
	}

	main()

	if !called {
		t.Fatal("expected main to call runWorkerFunc")
	}
}

func TestBuildWorkerGitLabEnabled(t *testing.T) {
	t.Setenv("THULE_QUEUE", "memory")
	t.Setenv("THULE_REPO_URL", "ssh://git@gl.blockstream.io/infrastructure/devops/kubernetes")
	t.Setenv("THULE_REPO_REF", "master")
	t.Setenv("THULE_GITLAB_TOKEN", "test-token")
	t.Setenv("THULE_GITLAB_BASE_URL", "https://gl.blockstream.io/api/v4")
	t.Setenv("THULE_GITLAB_PROJECT_PATH", "infrastructure/devops/kubernetes")

	deps, err := buildWorker(t.TempDir())
	if err != nil {
		t.Fatalf("build worker failed: %v", err)
	}
	if deps.jobs == nil || deps.syncer == nil || deps.plan == nil {
		t.Fatal("expected worker dependencies to be set")
	}
	if deps.mrChangedFile == nil {
		t.Fatal("expected gitlab changed-files fallback to be enabled")
	}
}

func TestBuildWorkerGitLabConfigError(t *testing.T) {
	t.Setenv("THULE_QUEUE", "memory")
	t.Setenv("THULE_REPO_URL", "")
	t.Setenv("THULE_GITLAB_TOKEN", "test-token")
	t.Setenv("THULE_GITLAB_PROJECT_PATH", "")
	t.Setenv("THULE_GITLAB_BASE_URL", "https://gl.blockstream.io/api/v4")

	if _, err := buildWorker(t.TempDir()); err == nil {
		t.Fatal("expected gitlab config error")
	}
}

func TestRunWorkerFallsBackToGitLabChangedFiles(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobs := queue.NewMemoryQueue(1)
	job := queue.Job{
		DeliveryID: "d3",
		EventType:  "comment.plan",
		Repository: "org/repo",
		MergeReqID: 88,
		HeadSHA:    "abc123",
		BaseRef:    "master",
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
	}, func(mrID int64) ([]string, error) {
		if mrID != 88 {
			t.Fatalf("unexpected merge request id: %d", mrID)
		}
		return []string{"clusters/cadmus/thule/deployment-worker.yaml"}, nil
	})
	if !called {
		t.Fatal("expected plan function to be called")
	}
	if len(got.ChangedFiles) != 1 || got.ChangedFiles[0] != "clusters/cadmus/thule/deployment-worker.yaml" {
		t.Fatalf("unexpected changed files: %+v", got.ChangedFiles)
	}
	if err == nil {
		t.Fatal("expected worker to exit with context cancellation")
	}
}
