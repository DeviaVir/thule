package vcs

import (
	"strings"
	"sync"
)

// reviewFollowUpMarkerPrefix opens the hidden HTML comment embedded in a
// follow-up review trigger. It is closed with "-->" by ReviewFollowUpMarker.
const reviewFollowUpMarkerPrefix = "<!--thule-review:"

// ReviewFollowUpMarker returns the hidden marker embedded in a follow-up review
// trigger comment, keyed by the commit it was posted for. Thule checks for it
// (via CommentStore.HasComment) before posting, so repeated plan runs for the
// same commit do not re-post the trigger — which would make the downstream
// reviewer (pr-agent) run, and bill, a duplicate review. It is an HTML comment
// (invisible in rendered markdown) and contains no whitespace, so a reviewer
// that parses the comment body as a command treats it as a single ignored
// token after "/review". A new commit produces a new marker.
func ReviewFollowUpMarker(sha string) string {
	return reviewFollowUpMarkerPrefix + sha + "-->"
}

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
	// HasComment reports whether any existing note on the MR contains marker
	// as a substring. Unlike List (which is scoped to plan notes), it scans
	// every note, so it can detect markerless standalone comments. Used to
	// make follow-up triggers idempotent per commit.
	HasComment(mergeReqID int64, marker string) bool
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

func (s *MemoryCommentStore) HasComment(mergeReqID int64, marker string) bool {
	if marker == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.comments[mergeReqID] {
		if strings.Contains(c.Body, marker) {
			return true
		}
	}
	return false
}
