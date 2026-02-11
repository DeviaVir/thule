package run

import "testing"

func TestMemoryStoreLifecycleAndPagination(t *testing.T) {
	s := NewMemoryStore()
	s.SetLatestSHA(1, "sha2")
	if !s.IsStale(1, "sha1") {
		t.Fatal("expected stale sha")
	}
	r1 := s.Start(1, "sha2", "p1")
	r2 := s.Start(1, "sha2", "p2")
	s.Complete(r1.ID, StateSuccess, "")
	s.Complete(r2.ID, StateFailed, "boom")

	page1 := s.List(1, 1, 1)
	if len(page1) != 1 {
		t.Fatalf("expected one record in first page: %+v", page1)
	}

	s.AddArtifact(r1.ID, "a1", "d1")
	s.AddArtifact(r1.ID, "a2", "d2")
	arts := s.ListArtifacts(r1.ID, 2, 1)
	if len(arts) != 1 || arts[0].Name != "a2" {
		t.Fatalf("unexpected artifacts page: %+v", arts)
	}
}

func TestMemoryStoreDefaultsAndBounds(t *testing.T) {
	s := NewMemoryStore()
	if got := s.List(123, 1, 10); len(got) != 0 {
		t.Fatalf("expected empty list, got %+v", got)
	}
	r := s.Start(2, "sha", "p")
	s.Complete(999, StateFailed, "ignored")
	arts := s.ListArtifacts(r.ID, 5, 1)
	if len(arts) != 0 {
		t.Fatalf("expected empty artifact page, got %+v", arts)
	}
}
