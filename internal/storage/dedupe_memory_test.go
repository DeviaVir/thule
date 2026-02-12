package storage

import (
	"context"
	"testing"
	"time"
)

func TestMemoryDedupeStoreReserveRelease(t *testing.T) {
	store := NewMemoryDedupeStore()
	ctx := context.Background()

	ok, err := store.Reserve(ctx, "k1", 50*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("expected first reserve ok, got ok=%v err=%v", ok, err)
	}
	ok, err = store.Reserve(ctx, "k1", 50*time.Millisecond)
	if err != nil || ok {
		t.Fatalf("expected second reserve to be rejected, got ok=%v err=%v", ok, err)
	}
	if err := store.Release(ctx, "k1"); err != nil {
		t.Fatalf("release failed: %v", err)
	}
	ok, err = store.Reserve(ctx, "k1", 50*time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("expected reserve after release ok, got ok=%v err=%v", ok, err)
	}
}
