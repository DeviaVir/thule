package storage

import (
	"context"
	"sync"
	"time"
)

type MemoryDedupeStore struct {
	mu     sync.Mutex
	expiry map[string]time.Time
}

func NewMemoryDedupeStore() *MemoryDedupeStore {
	return &MemoryDedupeStore{expiry: map[string]time.Time{}}
}

func (s *MemoryDedupeStore) Reserve(_ context.Context, key string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		return true, nil
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, exp := range s.expiry {
		if !exp.IsZero() && exp.Before(now) {
			delete(s.expiry, k)
		}
	}
	if exp, exists := s.expiry[key]; exists && exp.After(now) {
		return false, nil
	}
	s.expiry[key] = now.Add(ttl)
	return true, nil
}

func (s *MemoryDedupeStore) Release(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.expiry, key)
	return nil
}
