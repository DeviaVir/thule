package vcs

import "testing"

func TestMemoryStatusPublisher(t *testing.T) {
	p := NewMemoryStatusPublisher()
	p.SetStatus(StatusCheck{MergeReqID: 1, SHA: "abc", Context: "thule/plan", State: CheckPending})
	p.SetStatus(StatusCheck{MergeReqID: 1, SHA: "abc", Context: "thule/plan", State: CheckSuccess})
	items := p.ListStatuses(1, "abc")
	if len(items) != 2 {
		t.Fatalf("expected 2 statuses, got %+v", items)
	}
}
