package storage

import "sync"

type DeliveryStore interface {
	Seen(id string) bool
	MarkSeen(id string)
}

type MemoryDeliveryStore struct {
	mu   sync.RWMutex
	seen map[string]struct{}
}

func NewMemoryDeliveryStore() *MemoryDeliveryStore {
	return &MemoryDeliveryStore{seen: map[string]struct{}{}}
}

func (s *MemoryDeliveryStore) Seen(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.seen[id]
	return ok
}

func (s *MemoryDeliveryStore) MarkSeen(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen[id] = struct{}{}
}
