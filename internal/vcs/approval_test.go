package vcs

import "testing"

func TestMemoryApprover(t *testing.T) {
	a := NewMemoryApprover()
	a.SetApproval(ApprovalRecord{MergeReqID: 1, SHA: "a", Decision: DecisionApproved})
	a.SetApproval(ApprovalRecord{MergeReqID: 1, SHA: "b", Decision: DecisionRequestChanges})
	a.SetApproval(ApprovalRecord{MergeReqID: 2, SHA: "c", Decision: DecisionApproved})
	items := a.ListApprovals(1)
	if len(items) != 2 {
		t.Fatalf("expected 2 approvals, got %+v", items)
	}
}
