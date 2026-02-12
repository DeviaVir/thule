package vcs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	thulePlanMarker       = "<!-- thule:plan -->"
	thuleSupersededMarker = "<!-- thule:superseded -->"
	defaultGitLabTimeout  = 15 * time.Second
)

type GitLabOptions struct {
	BaseURL     string
	Token       string
	ProjectPath string
	Client      *http.Client
}

func GitLabOptionsFromEnv(repoURL string) (GitLabOptions, bool, error) {
	token := strings.TrimSpace(os.Getenv("THULE_GITLAB_TOKEN"))
	if token == "" {
		return GitLabOptions{}, false, nil
	}

	projectPath := strings.TrimSpace(os.Getenv("THULE_GITLAB_PROJECT_PATH"))
	if projectPath == "" {
		projectPath = parseProjectPath(repoURL)
	}
	if projectPath == "" {
		return GitLabOptions{}, false, fmt.Errorf("THULE_GITLAB_PROJECT_PATH is required when project path cannot be derived")
	}

	baseURL := strings.TrimSpace(os.Getenv("THULE_GITLAB_BASE_URL"))
	if baseURL == "" {
		host := parseHost(repoURL)
		if host == "" {
			host = "gl.blockstream.io"
		}
		baseURL = fmt.Sprintf("https://%s/api/v4", host)
	}

	return GitLabOptions{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		Token:       token,
		ProjectPath: projectPath,
		Client:      &http.Client{Timeout: defaultGitLabTimeout},
	}, true, nil
}

func NewGitLabCommentStore(opts GitLabOptions) (*GitLabCommentStore, error) {
	client, err := newGitLabClient(opts)
	if err != nil {
		return nil, err
	}
	return &GitLabCommentStore{client: client}, nil
}

func NewGitLabStatusPublisher(opts GitLabOptions) (*GitLabStatusPublisher, error) {
	client, err := newGitLabClient(opts)
	if err != nil {
		return nil, err
	}
	return &GitLabStatusPublisher{client: client}, nil
}

func NewGitLabMergeRequestReader(opts GitLabOptions) (*GitLabMergeRequestReader, error) {
	client, err := newGitLabClient(opts)
	if err != nil {
		return nil, err
	}
	return &GitLabMergeRequestReader{client: client}, nil
}

type GitLabCommentStore struct {
	client *gitLabClient
}

func (s *GitLabCommentStore) PostOrSupersede(mergeReqID int64, body string) Comment {
	if mergeReqID <= 0 {
		return Comment{}
	}
	existing, err := s.client.listNotes(mergeReqID)
	if err != nil {
		log.Printf("gitlab comment list failed mr=%d err=%v", mergeReqID, err)
	}

	created, err := s.client.createNote(mergeReqID, prependMarker(body))
	if err != nil {
		log.Printf("gitlab comment create failed mr=%d err=%v", mergeReqID, err)
		return Comment{}
	}

	for _, note := range existing {
		if note.ID == created.ID || note.System {
			continue
		}
		if !isThulePlanNote(note.Body) || isSupersededNote(note.Body) {
			continue
		}
		supersededBody := buildSupersededBody(created.ID)
		if err := s.client.updateNote(mergeReqID, note.ID, supersededBody); err != nil {
			log.Printf("gitlab comment supersede failed mr=%d note=%d err=%v", mergeReqID, note.ID, err)
		}
	}

	return Comment{ID: created.ID, MergeReqID: mergeReqID, Body: body}
}

func (s *GitLabCommentStore) List(mergeReqID int64) []Comment {
	notes, err := s.client.listNotes(mergeReqID)
	if err != nil {
		log.Printf("gitlab comment list failed mr=%d err=%v", mergeReqID, err)
		return nil
	}
	out := make([]Comment, 0, len(notes))
	for _, n := range notes {
		if !isThulePlanNote(n.Body) {
			continue
		}
		out = append(out, Comment{
			ID:         n.ID,
			MergeReqID: mergeReqID,
			Body:       stripPlanMarker(n.Body),
			Superseded: isSupersededNote(n.Body),
		})
	}
	return out
}

type GitLabStatusPublisher struct {
	client *gitLabClient
}

func (p *GitLabStatusPublisher) SetStatus(status StatusCheck) {
	if strings.TrimSpace(status.SHA) == "" {
		return
	}
	if err := p.client.setCommitStatus(status); err != nil {
		log.Printf("gitlab status publish failed sha=%s context=%s err=%v", status.SHA, status.Context, err)
	}
}

func (p *GitLabStatusPublisher) ListStatuses(_ int64, _ string) []StatusCheck {
	return nil
}

type GitLabMergeRequestReader struct {
	client *gitLabClient
}

func (r *GitLabMergeRequestReader) ChangedFiles(mergeReqID int64) ([]string, error) {
	if mergeReqID <= 0 {
		return nil, fmt.Errorf("merge request id is required")
	}
	path := fmt.Sprintf("%s/projects/%s/merge_requests/%d/changes", r.client.baseURL, url.PathEscape(r.client.projectPath), mergeReqID)
	var resp struct {
		Changes []struct {
			NewPath string `json:"new_path"`
			OldPath string `json:"old_path"`
		} `json:"changes"`
	}
	if err := r.client.request(http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(resp.Changes))
	for _, ch := range resp.Changes {
		p := strings.TrimSpace(ch.NewPath)
		if p == "" {
			p = strings.TrimSpace(ch.OldPath)
		}
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out, nil
}

type gitLabClient struct {
	baseURL     string
	token       string
	projectPath string
	httpClient  *http.Client
}

func newGitLabClient(opts GitLabOptions) (*gitLabClient, error) {
	if strings.TrimSpace(opts.BaseURL) == "" {
		return nil, fmt.Errorf("gitlab base url is required")
	}
	if strings.TrimSpace(opts.Token) == "" {
		return nil, fmt.Errorf("gitlab token is required")
	}
	if strings.TrimSpace(opts.ProjectPath) == "" {
		return nil, fmt.Errorf("gitlab project path is required")
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: defaultGitLabTimeout}
	}
	return &gitLabClient{
		baseURL:     strings.TrimRight(opts.BaseURL, "/"),
		token:       opts.Token,
		projectPath: strings.TrimSpace(opts.ProjectPath),
		httpClient:  client,
	}, nil
}

type gitLabNote struct {
	ID     int64  `json:"id"`
	Body   string `json:"body"`
	System bool   `json:"system"`
}

func (c *gitLabClient) listNotes(mergeReqID int64) ([]gitLabNote, error) {
	path := fmt.Sprintf("%s?per_page=100&order_by=created_at&sort=asc", c.notesURL(mergeReqID))
	var notes []gitLabNote
	if err := c.request(http.MethodGet, path, nil, &notes); err != nil {
		return nil, err
	}
	return notes, nil
}

func (c *gitLabClient) createNote(mergeReqID int64, body string) (gitLabNote, error) {
	payload := map[string]string{"body": body}
	var out gitLabNote
	if err := c.request(http.MethodPost, c.notesURL(mergeReqID), payload, &out); err != nil {
		return gitLabNote{}, err
	}
	return out, nil
}

func (c *gitLabClient) updateNote(mergeReqID, noteID int64, body string) error {
	payload := map[string]string{"body": body}
	return c.request(http.MethodPut, fmt.Sprintf("%s/%d", c.notesURL(mergeReqID), noteID), payload, nil)
}

func (c *gitLabClient) setCommitStatus(status StatusCheck) error {
	payload := map[string]string{
		"state":       mapState(status.State),
		"name":        status.Context,
		"description": truncate(status.Description, 255),
	}
	url := fmt.Sprintf("%s/projects/%s/statuses/%s", c.baseURL, url.PathEscape(c.projectPath), url.PathEscape(status.SHA))
	return c.request(http.MethodPost, url, payload, nil)
}

func (c *gitLabClient) notesURL(mergeReqID int64) string {
	return fmt.Sprintf("%s/projects/%s/merge_requests/%d/notes", c.baseURL, url.PathEscape(c.projectPath), mergeReqID)
}

func (c *gitLabClient) request(method, target string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal gitlab request: %w", err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		return fmt.Errorf("build gitlab request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gitlab request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("gitlab api %s %s: status=%d body=%s", method, target, resp.StatusCode, truncate(string(respBody), 300))
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode gitlab response: %w", err)
		}
	}
	return nil
}

func prependMarker(body string) string {
	return thulePlanMarker + "\n\n" + body
}

func stripPlanMarker(body string) string {
	return strings.TrimSpace(strings.TrimPrefix(body, thulePlanMarker))
}

func isThulePlanNote(body string) bool {
	return strings.Contains(body, thulePlanMarker)
}

func isSupersededNote(body string) bool {
	return strings.Contains(body, thuleSupersededMarker)
}

func buildSupersededBody(newNoteID int64) string {
	return fmt.Sprintf("%s\n%s\n<details><summary>Superseded Thule plan</summary>\n\nReplaced by newer Thule run (note id: %d).\n</details>", thulePlanMarker, thuleSupersededMarker, newNoteID)
}

func mapState(state CheckState) string {
	switch state {
	case CheckSuccess:
		return "success"
	case CheckFailed:
		return "failed"
	default:
		return "pending"
	}
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}

func parseHost(repoURL string) string {
	if repoURL == "" {
		return ""
	}
	if strings.HasPrefix(repoURL, "git@") {
		at := strings.Index(repoURL, "@")
		colon := strings.Index(repoURL, ":")
		if at >= 0 && colon > at {
			return repoURL[at+1 : colon]
		}
	}
	if u, err := url.Parse(repoURL); err == nil {
		return u.Hostname()
	}
	return ""
}

func parseProjectPath(repoURL string) string {
	if repoURL == "" {
		return ""
	}
	if strings.HasPrefix(repoURL, "git@") {
		if idx := strings.Index(repoURL, ":"); idx >= 0 && idx+1 < len(repoURL) {
			return strings.TrimSuffix(repoURL[idx+1:], ".git")
		}
	}
	if u, err := url.Parse(repoURL); err == nil {
		return strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), ".git")
	}
	return ""
}
