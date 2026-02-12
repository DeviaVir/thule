package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunStartsServer(t *testing.T) {
	t.Setenv("THULE_API_ADDR", "127.0.0.1:0")
	t.Setenv("THULE_WEBHOOK_SECRET", "secret")

	orig := listenAndServe
	t.Cleanup(func() { listenAndServe = orig })

	called := false
	listenAndServe = func(addr string, handler http.Handler) error {
		called = true
		if addr != "127.0.0.1:0" {
			t.Fatalf("unexpected addr: %s", addr)
		}
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
		return nil
	}

	if err := run(); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !called {
		t.Fatal("listenAndServe not called")
	}
}

func TestMainCallsRun(t *testing.T) {
	t.Setenv("THULE_API_ADDR", "127.0.0.1:0")

	orig := listenAndServe
	t.Cleanup(func() { listenAndServe = orig })
	listenAndServe = func(string, http.Handler) error {
		return nil
	}

	main()
}

func TestRunQueueInitError(t *testing.T) {
	t.Setenv("THULE_QUEUE", "memory")
	t.Setenv("THULE_QUEUE_BUFFER", "bad")
	if err := run(); err == nil {
		t.Fatal("expected queue init error")
	}
}

func TestRunDedupeInitError(t *testing.T) {
	t.Setenv("THULE_QUEUE", "memory")
	t.Setenv("THULE_QUEUE_BUFFER", "1")
	t.Setenv("THULE_DEDUPE", "memory")
	t.Setenv("THULE_DEDUPE_TTL", "bad")
	if err := run(); err == nil {
		t.Fatal("expected dedupe init error")
	}
}

func TestRunListenError(t *testing.T) {
	t.Setenv("THULE_API_ADDR", "127.0.0.1:0")
	t.Setenv("THULE_QUEUE", "memory")
	t.Setenv("THULE_QUEUE_BUFFER", "1")
	t.Setenv("THULE_DEDUPE", "disabled")

	orig := listenAndServe
	t.Cleanup(func() { listenAndServe = orig })
	listenAndServe = func(string, http.Handler) error { return http.ErrServerClosed }

	if err := run(); err == nil {
		t.Fatal("expected listen error")
	}
}
