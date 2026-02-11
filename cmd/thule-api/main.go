package main

import (
	"log"
	"net/http"
	"os"

	"github.com/example/thule/internal/lock"
	"github.com/example/thule/internal/orchestrator"
	"github.com/example/thule/internal/queue"
	"github.com/example/thule/internal/storage"
	"github.com/example/thule/internal/webhook"
)

func main() {
	addr := getEnv("THULE_API_ADDR", ":8080")
	secret := os.Getenv("THULE_WEBHOOK_SECRET")

	jobs := queue.NewMemoryQueue(100)
	store := storage.NewMemoryDeliveryStore()
	orch := orchestrator.New(jobs, store, lock.NewMemoryLocker())
	handler := webhook.NewHandler(secret, orch)

	mux := http.NewServeMux()
	mux.Handle("/webhook", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	log.Printf("thule-api listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("api stopped: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
