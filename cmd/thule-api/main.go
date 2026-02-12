package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/example/thule/internal/lock"
	"github.com/example/thule/internal/orchestrator"
	"github.com/example/thule/internal/queue"
	"github.com/example/thule/internal/storage"
	"github.com/example/thule/internal/webhook"
)

var listenAndServe = http.ListenAndServe

func main() {
	if err := run(); err != nil {
		log.Fatalf("thule-api failed: %v", err)
	}
}

func run() error {
	addr := getEnv("THULE_API_ADDR", ":8080")
	secret := os.Getenv("THULE_WEBHOOK_SECRET")

	jobs, err := queue.FromEnv()
	if err != nil {
		return fmt.Errorf("queue init failed: %w", err)
	}
	store := storage.NewMemoryDeliveryStore()
	dedupeCfg, err := storage.DedupeFromEnv()
	if err != nil {
		return fmt.Errorf("dedupe init failed: %w", err)
	}
	var dedupeStore storage.DedupeStore
	var dedupeTTL time.Duration
	if dedupeCfg != nil && dedupeCfg.Enabled {
		dedupeStore = dedupeCfg.Store
		dedupeTTL = dedupeCfg.TTL
	}
	orch := orchestrator.New(jobs, store, lock.NewMemoryLocker(), dedupeStore, dedupeTTL)
	handler := webhook.NewHandler(secret, orch)

	mux := http.NewServeMux()
	mux.Handle("/webhook", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	log.Printf("thule-api listening on %s", addr)
	if err := listenAndServe(addr, mux); err != nil {
		return fmt.Errorf("api stopped: %w", err)
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
