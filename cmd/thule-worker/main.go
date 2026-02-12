package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/thule/internal/orchestrator"
	"github.com/example/thule/internal/policy"
	"github.com/example/thule/internal/queue"
	"github.com/example/thule/internal/render"
	"github.com/example/thule/internal/repo"
	"github.com/example/thule/internal/run"
	"github.com/example/thule/internal/vcs"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repoRoot := getEnv("THULE_REPO_ROOT", ".")
	deps, err := buildWorker(repoRoot)
	if err != nil {
		log.Fatalf("worker init failed: %v", err)
	}

	log.Printf("thule-worker started repo=%s", repoRoot)

	_ = runWorkerFunc(ctx, deps.jobs, deps.syncer, deps.plan)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

type planFunc func(context.Context, orchestrator.MergeRequestEvent) error

var runWorkerFunc = runWorker

const defaultBaseRef = "master"

type workerDeps struct {
	jobs   queue.Queue
	syncer *repo.Syncer
	plan   planFunc
}

func buildWorker(repoRoot string) (workerDeps, error) {
	jobs, err := queue.FromEnv()
	if err != nil {
		return workerDeps{}, err
	}
	repoURL := os.Getenv("THULE_REPO_URL")
	repoRef := getEnv("THULE_REPO_REF", "master")
	auth, err := repo.AuthFromEnv()
	if err != nil {
		return workerDeps{}, err
	}
	syncer := repo.NewSyncer(repoURL, repoRef, repoRoot, auth)
	comments := vcs.NewMemoryCommentStore()
	statuses := vcs.NewMemoryStatusPublisher()
	runs := run.NewMemoryStore()
	cluster := &orchestrator.MemoryClusterReader{ByClusterNS: map[string][]render.Resource{}}

	planner := orchestrator.NewPlanner(repoRoot, cluster, comments, statuses, runs, policy.NewBuiltinEvaluator())
	return workerDeps{jobs: jobs, syncer: syncer, plan: planner.PlanForEvent}, nil
}

func runWorker(ctx context.Context, jobs queue.Queue, syncer *repo.Syncer, plan planFunc) error {
	for {
		job, err := jobs.Dequeue(ctx)
		if err != nil {
			log.Printf("worker exiting: %v", err)
			return err
		}
		if syncer != nil && syncer.Enabled() {
			if err := syncer.Sync(ctx, job.HeadSHA); err != nil {
				log.Printf("repo sync failed delivery=%s mr=%d sha=%s err=%v", job.DeliveryID, job.MergeReqID, job.HeadSHA, err)
				continue
			}
		}
		changedFiles := job.ChangedFiles
		if len(changedFiles) == 0 {
			baseRef := job.BaseRef
			if baseRef == "" {
				baseRef = getEnv("THULE_REPO_BASE_REF", defaultBaseRef)
			}
			files, err := repo.ChangedFiles(getEnv("THULE_REPO_ROOT", "."), baseRef, job.HeadSHA)
			if err != nil {
				log.Printf("diff files failed delivery=%s mr=%d base=%s sha=%s err=%v", job.DeliveryID, job.MergeReqID, baseRef, job.HeadSHA, err)
			} else {
				changedFiles = files
			}
		}
		evt := orchestrator.MergeRequestEvent{
			DeliveryID:   job.DeliveryID,
			EventType:    job.EventType,
			Repository:   job.Repository,
			MergeReqID:   job.MergeReqID,
			HeadSHA:      job.HeadSHA,
			BaseRef:      job.BaseRef,
			ChangedFiles: changedFiles,
		}
		if err := plan(ctx, evt); err != nil {
			log.Printf("plan failed delivery=%s mr=%d sha=%s err=%v", job.DeliveryID, job.MergeReqID, job.HeadSHA, err)
			continue
		}
		log.Printf("plan completed delivery=%s mr=%d sha=%s", job.DeliveryID, job.MergeReqID, job.HeadSHA)
	}
}
