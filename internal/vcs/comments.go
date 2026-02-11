package vcs

import "sync"

type Comment struct {
	ID           int64
	MergeReqID   int64
	Body         string
	Superseded   bool
	SupersededBy int64
}

type CommentStore interface {
	PostOrSupersede(mergeReqID int64, body string) Comment
	List(mergeReqID int64) []Comment
}

type MemoryCommentStore struct {
	mu       sync.Mutex
	nextID   int64
	comments map[int64][]Comment
}

func NewMemoryCommentStore() *MemoryCommentStore {
	return &MemoryCommentStore{nextID: 1, comments: map[int64][]Comment{}}
}

func (s *MemoryCommentStore) PostOrSupersede(mergeReqID int64, body string) Comment {
	s.mu.Lock()
	defer s.mu.Unlock()
	newComment := Comment{ID: s.nextID, MergeReqID: mergeReqID, Body: body}
	s.nextID++
	items := s.comments[mergeReqID]
	for i := range items {
		if !items[i].Superseded {
			items[i].Superseded = true
			items[i].SupersededBy = newComment.ID
		}
	}
	s.comments[mergeReqID] = append(items, newComment)
	return newComment
}

func (s *MemoryCommentStore) List(mergeReqID int64) []Comment {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.comments[mergeReqID]
	out := make([]Comment, len(items))
	copy(out, items)
	return out
}
