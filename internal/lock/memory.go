package lock

import "sync"

type Locker interface {
	Acquire(repo, projectKey string, mergeReqID int64) (bool, int64)
	ReleaseByMR(repo string, mergeReqID int64)
	List(repo string) map[string]int64
}

type MemoryLocker struct {
	mu    sync.Mutex
	locks map[string]map[string]int64
}

func NewMemoryLocker() *MemoryLocker {
	return &MemoryLocker{locks: map[string]map[string]int64{}}
}

func (m *MemoryLocker) Acquire(repo, projectKey string, mergeReqID int64) (bool, int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.locks[repo]; !ok {
		m.locks[repo] = map[string]int64{}
	}
	owner, exists := m.locks[repo][projectKey]
	if !exists || owner == mergeReqID {
		m.locks[repo][projectKey] = mergeReqID
		return true, mergeReqID
	}
	return false, owner
}

func (m *MemoryLocker) ReleaseByMR(repo string, mergeReqID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, owner := range m.locks[repo] {
		if owner == mergeReqID {
			delete(m.locks[repo], key)
		}
	}
}

func (m *MemoryLocker) List(repo string) map[string]int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]int64{}
	for k, v := range m.locks[repo] {
		out[k] = v
	}
	return out
}
