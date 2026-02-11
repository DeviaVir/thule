package vcs

import "sync"

type ApprovalDecision string

const (
	DecisionApproved       ApprovalDecision = "approved"
	DecisionRequestChanges ApprovalDecision = "request_changes"
)

type ApprovalRecord struct {
	MergeReqID int64
	SHA        string
	Decision   ApprovalDecision
	Reason     string
}

type Approver interface {
	SetApproval(record ApprovalRecord)
	ListApprovals(mergeReqID int64) []ApprovalRecord
}

type MemoryApprover struct {
	mu      sync.Mutex
	records []ApprovalRecord
}

func NewMemoryApprover() *MemoryApprover { return &MemoryApprover{} }

func (m *MemoryApprover) SetApproval(record ApprovalRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, record)
}

func (m *MemoryApprover) ListApprovals(mergeReqID int64) []ApprovalRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []ApprovalRecord{}
	for _, r := range m.records {
		if r.MergeReqID == mergeReqID {
			out = append(out, r)
		}
	}
	return out
}
