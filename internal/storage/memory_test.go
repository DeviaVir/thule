package storage

import "testing"

func TestMemoryDeliveryStoreLifecycle(t *testing.T) {
	s := NewMemoryDeliveryStore()
	id := "delivery-123"

	if !s.Reserve(id) {
		t.Fatal("expected reserve to succeed")
	}
	if s.Reserve(id) {
		t.Fatal("expected duplicate reserve to fail")
	}
	if !s.Seen(id) {
		t.Fatal("expected seen after reserve")
	}

	s.Release(id)
	if s.Seen(id) {
		t.Fatal("expected release to clear pending reservation")
	}

	if !s.Reserve(id) {
		t.Fatal("expected reserve to succeed again after release")
	}
	s.Commit(id)
	s.Release(id)
	if !s.Seen(id) {
		t.Fatal("expected committed id to remain seen")
	}
}
