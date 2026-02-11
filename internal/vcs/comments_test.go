package vcs

import "testing"

func TestPostOrSupersede(t *testing.T) {
	s := NewMemoryCommentStore()
	c1 := s.PostOrSupersede(1, "first")
	c2 := s.PostOrSupersede(1, "second")
	items := s.List(1)
	if len(items) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(items))
	}
	if !items[0].Superseded || items[0].SupersededBy != c2.ID {
		t.Fatalf("expected first superseded by second: %+v", items[0])
	}
	if items[1].ID != c2.ID || items[1].Superseded {
		t.Fatalf("unexpected second comment: %+v", items[1])
	}
	_ = c1
}
