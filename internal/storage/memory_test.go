package storage

import "testing"

func TestMemoryDeliveryStoreSeenLifecycle(t *testing.T) {
	s := NewMemoryDeliveryStore()
	id := "delivery-123"

	if s.Seen(id) {
		t.Fatal("expected unseen id before mark")
	}

	s.MarkSeen(id)

	if !s.Seen(id) {
		t.Fatal("expected seen id after mark")
	}
}
