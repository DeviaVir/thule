package run

import (
	"sort"
	"sync"
	"time"
)

type State string

const (
	StateRunning  State = "running"
	StateSuccess  State = "success"
	StateFailed   State = "failed"
	StateCanceled State = "canceled"
)

type Record struct {
	ID         int64
	MergeReqID int64
	HeadSHA    string
	Project    string
	State      State
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Error      string
}

type Artifact struct {
	RunID int64
	Name  string
	Data  string
}

type Store interface {
	Start(mergeReqID int64, sha, project string) Record
	Complete(runID int64, state State, errMsg string)
	List(mergeReqID int64, page, pageSize int) []Record
	AddArtifact(runID int64, name, data string)
	ListArtifacts(runID int64, page, pageSize int) []Artifact
	SetLatestSHA(mergeReqID int64, sha string)
	IsStale(mergeReqID int64, sha string) bool
}

type MemoryStore struct {
	mu        sync.Mutex
	nextRunID int64
	runs      map[int64]Record
	byMR      map[int64][]int64
	artifacts map[int64][]Artifact
	latestSHA map[int64]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{nextRunID: 1, runs: map[int64]Record{}, byMR: map[int64][]int64{}, artifacts: map[int64][]Artifact{}, latestSHA: map[int64]string{}}
}

func (s *MemoryStore) Start(mergeReqID int64, sha, project string) Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	r := Record{ID: s.nextRunID, MergeReqID: mergeReqID, HeadSHA: sha, Project: project, State: StateRunning, CreatedAt: now, UpdatedAt: now}
	s.nextRunID++
	s.runs[r.ID] = r
	s.byMR[mergeReqID] = append(s.byMR[mergeReqID], r.ID)
	return r
}

func (s *MemoryStore) Complete(runID int64, state State, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runID]
	if !ok {
		return
	}
	r.State = state
	r.Error = errMsg
	r.UpdatedAt = time.Now().UTC()
	s.runs[runID] = r
}

func (s *MemoryStore) List(mergeReqID int64, page, pageSize int) []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := append([]int64{}, s.byMR[mergeReqID]...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] > ids[j] })
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	start := (page - 1) * pageSize
	if start >= len(ids) {
		return nil
	}
	end := start + pageSize
	if end > len(ids) {
		end = len(ids)
	}
	out := make([]Record, 0, end-start)
	for _, id := range ids[start:end] {
		out = append(out, s.runs[id])
	}
	return out
}

func (s *MemoryStore) AddArtifact(runID int64, name, data string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.artifacts[runID] = append(s.artifacts[runID], Artifact{RunID: runID, Name: name, Data: data})
}

func (s *MemoryStore) ListArtifacts(runID int64, page, pageSize int) []Artifact {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.artifacts[runID]
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return nil
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	out := make([]Artifact, end-start)
	copy(out, items[start:end])
	return out
}

func (s *MemoryStore) SetLatestSHA(mergeReqID int64, sha string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latestSHA[mergeReqID] = sha
}

func (s *MemoryStore) IsStale(mergeReqID int64, sha string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latestSHA[mergeReqID] != "" && s.latestSHA[mergeReqID] != sha
}
