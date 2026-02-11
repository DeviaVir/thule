package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/example/thule/internal/queue"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	jobs := queue.NewMemoryQueue(100)
	log.Printf("thule-worker started (phase 0 skeleton)")

	for {
		job, err := jobs.Dequeue(ctx)
		if err != nil {
			log.Printf("worker exiting: %v", err)
			return
		}
		log.Printf("received job delivery=%s event=%s repo=%s mr=%d sha=%s", job.DeliveryID, job.EventType, job.Repository, job.MergeReqID, job.HeadSHA)
	}
}
