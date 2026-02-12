package repo

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestSyncerCloneAndCheckout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	src := filepath.Join(t.TempDir(), "src")
	repo, err := git.PlainInit(src, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("add: %v", err)
	}
	commit, err := wt.Commit("initial", &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()}})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "dest")
	syncer := NewSyncer(src, "", dest, nil)
	if err := syncer.Sync(ctx, ""); err != nil {
		t.Fatalf("sync clone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "README.md")); err != nil {
		t.Fatalf("expected file in clone: %v", err)
	}

	if err := syncer.Sync(ctx, commit.String()); err != nil {
		t.Fatalf("sync checkout: %v", err)
	}
}

func TestSyncerEmptyDirError(t *testing.T) {
	syncer := NewSyncer("https://example.com/repo.git", "", "", nil)
	if err := syncer.Sync(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty repo dir")
	}
}

func TestSyncerEnabled(t *testing.T) {
	if NewSyncer("", "", "dir", nil).Enabled() {
		t.Fatal("expected disabled when url empty")
	}
	if !NewSyncer("https://example.com/repo.git", "", "dir", nil).Enabled() {
		t.Fatal("expected enabled when url set")
	}
}

func TestNormalizeRef(t *testing.T) {
	if got := normalizeRef("main").String(); got != "refs/heads/main" {
		t.Fatalf("unexpected ref: %s", got)
	}
	if got := normalizeRef("refs/merge-requests/1/head").String(); got != "refs/merge-requests/1/head" {
		t.Fatalf("unexpected ref passthrough: %s", got)
	}
}

func TestSyncerCloneWithRef(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	src := filepath.Join(t.TempDir(), "src")
	repo, err := git.PlainInit(src, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := wt.Commit("initial", &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()}}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := wt.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("feature"), Create: true}); err != nil {
		t.Fatalf("checkout feature: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "dest")
	syncer := NewSyncer(src, "feature", dest, nil)
	if err := syncer.Sync(ctx, ""); err != nil {
		t.Fatalf("sync clone with ref: %v", err)
	}

	cloned, err := git.PlainOpen(dest)
	if err != nil {
		t.Fatalf("open clone: %v", err)
	}
	head, err := cloned.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	if head.Name().Short() != "feature" {
		t.Fatalf("expected feature branch, got %s", head.Name().Short())
	}
}

func TestSyncerMaintain(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	src := filepath.Join(t.TempDir(), "src")
	repo, err := git.PlainInit(src, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := wt.Commit("initial", &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()}}); err != nil {
		t.Fatalf("commit: %v", err)
	}

	dest := filepath.Join(t.TempDir(), "dest")
	syncer := NewSyncer(src, "", dest, nil)
	if err := syncer.Sync(ctx, ""); err != nil {
		t.Fatalf("sync clone: %v", err)
	}
	if err := syncer.Maintain(ctx); err != nil {
		t.Fatalf("maintain failed: %v", err)
	}
}
