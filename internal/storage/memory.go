package storage

import "sync"

type DeliveryStore interface {
	Reserve(id string) bool
	Commit(id string)
	Release(id string)
	Seen(id string) bool
}

type state uint8

const (
	statePending state = iota
	stateCommitted
)

type MemoryDeliveryStore struct {
	mu    sync.RWMutex
	state map[string]state
}

func NewMemoryDeliveryStore() *MemoryDeliveryStore {
	return &MemoryDeliveryStore{state: map[string]state{}}
}

func (s *MemoryDeliveryStore) Reserve(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.state[id]; exists {
		return false
	}
	s.state[id] = statePending
	return true
}

func (s *MemoryDeliveryStore) Commit(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.state[id]; exists {
		s.state[id] = stateCommitted
	}
}

func (s *MemoryDeliveryStore) Release(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state[id] == statePending {
		delete(s.state, id)
	}
}

func (s *MemoryDeliveryStore) Seen(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.state[id]
	return ok
}
