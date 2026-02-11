package vcs

import "sync"

type CheckState string

const (
	CheckPending CheckState = "pending"
	CheckSuccess CheckState = "success"
	CheckFailed  CheckState = "failed"
)

type StatusCheck struct {
	MergeReqID  int64
	SHA         string
	Context     string
	State       CheckState
	Description string
}

type StatusPublisher interface {
	SetStatus(status StatusCheck)
	ListStatuses(mergeReqID int64, sha string) []StatusCheck
}

type MemoryStatusPublisher struct {
	mu    sync.Mutex
	items []StatusCheck
}

func NewMemoryStatusPublisher() *MemoryStatusPublisher { return &MemoryStatusPublisher{} }

func (m *MemoryStatusPublisher) SetStatus(status StatusCheck) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = append(m.items, status)
}

func (m *MemoryStatusPublisher) ListStatuses(mergeReqID int64, sha string) []StatusCheck {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []StatusCheck{}
	for _, s := range m.items {
		if s.MergeReqID == mergeReqID && s.SHA == sha {
			out = append(out, s)
		}
	}
	return out
}
