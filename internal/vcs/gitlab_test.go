package vcs

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestGitLabCommentStorePostOrSupersede(t *testing.T) {
	type note struct {
		ID     int64  `json:"id"`
		Body   string `json:"body"`
		System bool   `json:"system"`
	}
	notes := []note{{ID: 1, Body: prependMarker("old"), System: false}}
	nextID := int64(2)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/projects/group/repo/merge_requests/42/notes") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(notes)
		case http.MethodPost:
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode post: %v", err)
			}
			n := note{ID: nextID, Body: payload["body"]}
			nextID++
			notes = append(notes, n)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(n)
		case http.MethodPut:
			idPart := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			noteID, err := strconv.ParseInt(idPart, 10, 64)
			if err != nil {
				t.Fatalf("parse note id: %v", err)
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode put: %v", err)
			}
			for i := range notes {
				if notes[i].ID == noteID {
					notes[i].Body = payload["body"]
				}
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer srv.Close()

	store, err := NewGitLabCommentStore(GitLabOptions{
		BaseURL:     srv.URL,
		Token:       "token",
		ProjectPath: "group/repo",
		Client:      srv.Client(),
	})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	c := store.PostOrSupersede(42, "new body")
	if c.ID == 0 {
		t.Fatal("expected created comment id")
	}
	if !isSupersededNote(notes[0].Body) {
		t.Fatalf("expected old note superseded, got: %s", notes[0].Body)
	}
	if !isThulePlanNote(notes[1].Body) || !strings.Contains(notes[1].Body, "new body") {
		t.Fatalf("expected new note marker+body, got: %s", notes[1].Body)
	}
}

func TestGitLabStatusPublisherSetStatus(t *testing.T) {
	var gotPath string
	var gotPayload map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		data, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(data, &gotPayload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	pub, err := NewGitLabStatusPublisher(GitLabOptions{
		BaseURL:     srv.URL,
		Token:       "token",
		ProjectPath: "group/repo",
		Client:      srv.Client(),
	})
	if err != nil {
		t.Fatalf("new status publisher: %v", err)
	}

	pub.SetStatus(StatusCheck{
		SHA:         "abc123",
		Context:     "thule/plan",
		State:       CheckSuccess,
		Description: "ok",
	})

	if !strings.Contains(gotPath, "/projects/group/repo/statuses/abc123") {
		t.Fatalf("unexpected status path: %s", gotPath)
	}
	if gotPayload["state"] != "success" || gotPayload["name"] != "thule/plan" {
		t.Fatalf("unexpected payload: %+v", gotPayload)
	}
}

func TestGitLabOptionsFromEnv(t *testing.T) {
	t.Setenv("THULE_GITLAB_TOKEN", "tok")
	t.Setenv("THULE_GITLAB_BASE_URL", "https://gl.example.com/api/v4")
	t.Setenv("THULE_GITLAB_PROJECT_PATH", "")

	opts, ok, err := GitLabOptionsFromEnv("ssh://git@gl.example.com/group/repo.git")
	if err != nil {
		t.Fatalf("options error: %v", err)
	}
	if !ok {
		t.Fatal("expected gitlab options to be enabled")
	}
	if opts.ProjectPath != "group/repo" {
		t.Fatalf("unexpected project path: %s", opts.ProjectPath)
	}
	if opts.BaseURL != "https://gl.example.com/api/v4" {
		t.Fatalf("unexpected base url: %s", opts.BaseURL)
	}
}

func TestGitLabOptionsFromEnvDisabledWithoutToken(t *testing.T) {
	t.Setenv("THULE_GITLAB_TOKEN", "")
	if _, ok, err := GitLabOptionsFromEnv("ssh://git@gl.example.com/group/repo.git"); err != nil || ok {
		t.Fatalf("expected disabled with no error, got ok=%v err=%v", ok, err)
	}
}

func TestGitLabOptionsFromEnvParsesDefaults(t *testing.T) {
	t.Setenv("THULE_GITLAB_TOKEN", "tok")
	t.Setenv("THULE_GITLAB_BASE_URL", "")
	t.Setenv("THULE_GITLAB_PROJECT_PATH", "")
	opts, ok, err := GitLabOptionsFromEnv("git@gl.blockstream.io:infrastructure/devops/kubernetes.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected enabled options")
	}
	if opts.ProjectPath != "infrastructure/devops/kubernetes" {
		t.Fatalf("unexpected project path: %s", opts.ProjectPath)
	}
	if opts.BaseURL != "https://gl.blockstream.io/api/v4" {
		t.Fatalf("unexpected base url: %s", opts.BaseURL)
	}
}

func TestNewGitLabClientValidation(t *testing.T) {
	_, err := newGitLabClient(GitLabOptions{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	_, err = newGitLabClient(GitLabOptions{BaseURL: "http://x", Token: "t", ProjectPath: "g/r"})
	if err != nil {
		t.Fatalf("expected valid client, got %v", err)
	}
}

func TestGitLabCommentStoreListFiltersMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": 1, "body": prependMarker("one"), "system": false},
			{"id": 2, "body": "plain note", "system": false},
			{"id": 3, "body": buildSupersededBody(4), "system": false},
		})
	}))
	defer srv.Close()

	store, err := NewGitLabCommentStore(GitLabOptions{
		BaseURL:     srv.URL,
		Token:       "token",
		ProjectPath: "group/repo",
		Client:      srv.Client(),
	})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	items := store.List(12)
	if len(items) != 2 {
		t.Fatalf("expected 2 thule notes, got %d", len(items))
	}
	if items[0].Superseded {
		t.Fatalf("expected first note not superseded: %+v", items[0])
	}
	if !items[1].Superseded {
		t.Fatalf("expected second note superseded: %+v", items[1])
	}
}

func TestGitLabRequestHandlesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusUnauthorized)
	}))
	defer srv.Close()
	c, err := newGitLabClient(GitLabOptions{
		BaseURL:     srv.URL,
		Token:       "t",
		ProjectPath: "group/repo",
		Client:      srv.Client(),
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if err := c.request(http.MethodGet, srv.URL, nil, nil); err == nil {
		t.Fatal("expected request error")
	}
}

func TestHelpers(t *testing.T) {
	if got := parseHost("git@gl.blockstream.io:group/repo.git"); got != "gl.blockstream.io" {
		t.Fatalf("unexpected host: %s", got)
	}
	if got := parseHost("https://gl.blockstream.io/group/repo.git"); got != "gl.blockstream.io" {
		t.Fatalf("unexpected host from https: %s", got)
	}
	if got := parseHost(""); got != "" {
		t.Fatalf("expected empty host, got: %s", got)
	}
	if got := parseProjectPath("https://gl.blockstream.io/group/repo.git"); got != "group/repo" {
		t.Fatalf("unexpected project path: %s", got)
	}
	if got := parseProjectPath("git@gl.blockstream.io:group/repo.git"); got != "group/repo" {
		t.Fatalf("unexpected project path from scp style: %s", got)
	}
	if got := parseProjectPath(""); got != "" {
		t.Fatalf("expected empty project path, got: %s", got)
	}
	if got := mapState(CheckFailed); got != "failed" {
		t.Fatalf("unexpected state mapping: %s", got)
	}
	if got := mapState(CheckPending); got != "pending" {
		t.Fatalf("unexpected pending state mapping: %s", got)
	}
	if got := truncate("abcdef", 3); got != "abc" {
		t.Fatalf("unexpected truncation: %s", got)
	}
	if !isThulePlanNote(prependMarker("x")) {
		t.Fatal("expected thule marker detection")
	}
}

func TestGitLabStatusPublisherNoSHA(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	pub, err := NewGitLabStatusPublisher(GitLabOptions{
		BaseURL:     srv.URL,
		Token:       "token",
		ProjectPath: "group/repo",
		Client:      srv.Client(),
	})
	if err != nil {
		t.Fatalf("new status publisher: %v", err)
	}
	pub.SetStatus(StatusCheck{SHA: "", Context: "thule/plan", State: CheckPending})
	if called {
		t.Fatal("expected no request when sha is empty")
	}
	if got := pub.ListStatuses(1, "abc"); got != nil {
		t.Fatalf("expected nil list for gitlab publisher, got %+v", got)
	}
}

func TestGitLabRequestDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()
	c, err := newGitLabClient(GitLabOptions{
		BaseURL:     srv.URL,
		Token:       "t",
		ProjectPath: "group/repo",
		Client:      srv.Client(),
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	var out map[string]any
	if err := c.request(http.MethodGet, srv.URL, nil, &out); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestGitLabMergeRequestReaderChangedFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/projects/group/repo/merge_requests/42/changes") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"changes": []map[string]any{
				{"new_path": "clusters/cadmus/thule/deployment-worker.yaml"},
				{"new_path": "clusters/cadmus/thule/deployment-worker.yaml"}, // duplicate
				{"new_path": "", "old_path": "clusters/theseus/thule/rbac.yaml"},
			},
		})
	}))
	defer srv.Close()

	reader, err := NewGitLabMergeRequestReader(GitLabOptions{
		BaseURL:     srv.URL,
		Token:       "token",
		ProjectPath: "group/repo",
		Client:      srv.Client(),
	})
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}
	files, err := reader.ChangedFiles(42)
	if err != nil {
		t.Fatalf("changed files: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 unique paths, got %+v", files)
	}
	if files[0] != "clusters/cadmus/thule/deployment-worker.yaml" || files[1] != "clusters/theseus/thule/rbac.yaml" {
		t.Fatalf("unexpected paths: %+v", files)
	}
}
