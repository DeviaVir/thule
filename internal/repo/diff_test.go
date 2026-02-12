package repo

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestChangedFiles(t *testing.T) {
	repoDir := filepath.Join(t.TempDir(), "repo")
	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("base"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := wt.Commit("base", &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()}}); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if err := wt.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("feature"), Create: true}); err != nil {
		t.Fatalf("checkout feature: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "new.yaml"), []byte("kind: ConfigMap\nmetadata:\n  name: cm\n"), 0o644); err != nil {
		t.Fatalf("write new file: %v", err)
	}
	if _, err := wt.Add("new.yaml"); err != nil {
		t.Fatalf("add new file: %v", err)
	}
	commit, err := wt.Commit("feature", &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()}})
	if err != nil {
		t.Fatalf("commit feature: %v", err)
	}

	files, err := ChangedFiles(repoDir, "master", commit.String())
	if err != nil {
		t.Fatalf("changed files: %v", err)
	}
	if len(files) != 1 || files[0] != "new.yaml" {
		t.Fatalf("unexpected files: %+v", files)
	}
}

func TestChangedFilesInvalidBaseRef(t *testing.T) {
	repoDir := filepath.Join(t.TempDir(), "repo")
	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("base"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("add: %v", err)
	}
	commit, err := wt.Commit("base", &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()}})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if _, err := ChangedFiles(repoDir, "missing", commit.String()); err == nil {
		t.Fatal("expected error for missing base ref")
	}
}

func TestChangedFilesWithHashBaseRef(t *testing.T) {
	repoDir := filepath.Join(t.TempDir(), "repo")
	repo, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatalf("init repo: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("base"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("add: %v", err)
	}
	baseCommit, err := wt.Commit("base", &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()}})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "file.yaml"), []byte("kind: ConfigMap\nmetadata:\n  name: cm\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := wt.Add("file.yaml"); err != nil {
		t.Fatalf("add: %v", err)
	}
	headCommit, err := wt.Commit("head", &git.CommitOptions{Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()}})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	files, err := ChangedFiles(repoDir, baseCommit.String(), headCommit.String())
	if err != nil {
		t.Fatalf("changed files: %v", err)
	}
	if len(files) != 1 || files[0] != "file.yaml" {
		t.Fatalf("unexpected files: %+v", files)
	}
}

func TestChangedFilesMissingHeadSHA(t *testing.T) {
	repoDir := filepath.Join(t.TempDir(), "repo")
	if _, err := ChangedFiles(repoDir, "master", ""); err == nil {
		t.Fatal("expected error for empty head sha")
	}
}
