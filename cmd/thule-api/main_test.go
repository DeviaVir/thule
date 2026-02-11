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
