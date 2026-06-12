package vcs

import "sync"

type Comment struct {
	ID           int64
	MergeReqID   int64
	Body         string
	Superseded   bool
	SupersededBy int64
	// Standalone comments (from Post) are exempt from the supersede
	// lifecycle, matching the GitLab store which only supersedes notes
	// carrying the plan marker.
	Standalone bool
}

type CommentStore interface {
	PostOrSupersede(mergeReqID int64, body string) Comment
	// Post adds a standalone comment that does not participate in the
	// supersede lifecycle (used for follow-up comments such as bot
	// triggers).
	Post(mergeReqID int64, body string) Comment
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
		if !items[i].Superseded && !items[i].Standalone {
			items[i].Superseded = true
			items[i].SupersededBy = newComment.ID
		}
	}
	s.comments[mergeReqID] = append(items, newComment)
	return newComment
}

func (s *MemoryCommentStore) Post(mergeReqID int64, body string) Comment {
	s.mu.Lock()
	defer s.mu.Unlock()
	newComment := Comment{ID: s.nextID, MergeReqID: mergeReqID, Body: body, Standalone: true}
	s.nextID++
	s.comments[mergeReqID] = append(s.comments[mergeReqID], newComment)
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
