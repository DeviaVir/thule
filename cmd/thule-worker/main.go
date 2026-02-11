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
	jobs, err := queue.FromEnv()
	if err != nil {
		log.Fatalf("queue init failed: %v", err)
	}
	repoURL := os.Getenv("THULE_REPO_URL")
	repoRef := getEnv("THULE_REPO_REF", "master")
	auth, err := repo.AuthFromEnv()
	if err != nil {
		log.Fatalf("repo auth failed: %v", err)
	}
	syncer := repo.NewSyncer(repoURL, repoRef, repoRoot, auth)
	comments := vcs.NewMemoryCommentStore()
	statuses := vcs.NewMemoryStatusPublisher()
	runs := run.NewMemoryStore()
	cluster := &orchestrator.MemoryClusterReader{ByClusterNS: map[string][]render.Resource{}}

	planner := orchestrator.NewPlanner(repoRoot, cluster, comments, statuses, runs, policy.NewBuiltinEvaluator())

	log.Printf("thule-worker started repo=%s", repoRoot)

	for {
		job, err := jobs.Dequeue(ctx)
		if err != nil {
			log.Printf("worker exiting: %v", err)
			return
		}
		if syncer.Enabled() {
			if err := syncer.Sync(ctx, job.HeadSHA); err != nil {
				log.Printf("repo sync failed delivery=%s mr=%d sha=%s err=%v", job.DeliveryID, job.MergeReqID, job.HeadSHA, err)
				continue
			}
		}
		evt := orchestrator.MergeRequestEvent{
			DeliveryID:   job.DeliveryID,
			EventType:    job.EventType,
			Repository:   job.Repository,
			MergeReqID:   job.MergeReqID,
			HeadSHA:      job.HeadSHA,
			ChangedFiles: job.ChangedFiles,
		}
		if err := planner.PlanForEvent(ctx, evt); err != nil {
			log.Printf("plan failed delivery=%s mr=%d sha=%s err=%v", job.DeliveryID, job.MergeReqID, job.HeadSHA, err)
			continue
		}
		log.Printf("plan completed delivery=%s mr=%d sha=%s", job.DeliveryID, job.MergeReqID, job.HeadSHA)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
