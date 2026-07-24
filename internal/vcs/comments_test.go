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

func TestMemoryCommentStorePostIsStandalone(t *testing.T) {
	s := NewMemoryCommentStore()
	first := s.Post(7, "/review")
	s.PostOrSupersede(7, "plan")
	items := s.List(7)
	if len(items) != 2 {
		t.Fatalf("expected two comments, got %d", len(items))
	}
	for _, c := range items {
		if c.ID == first.ID && c.Superseded {
			t.Fatal("standalone comment must not be superseded")
		}
	}
}

func TestMemoryCommentStoreHasComment(t *testing.T) {
	s := NewMemoryCommentStore()
	marker := ReviewFollowUpMarker("abc123")
	if marker != "<!--thule-review:abc123-->" {
		t.Fatalf("unexpected review marker: %q", marker)
	}
	if s.HasComment(7, marker) {
		t.Fatal("unexpected marker before comment is posted")
	}

	s.Post(7, "/review\n"+marker)
	if !s.HasComment(7, marker) {
		t.Fatal("expected marker in standalone comment")
	}
	if s.HasComment(7, ReviewFollowUpMarker("other")) {
		t.Fatal("unexpected marker for another commit")
	}
	if s.HasComment(7, "") {
		t.Fatal("empty marker must not match")
	}
}
