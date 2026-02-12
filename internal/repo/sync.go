package repo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

type Syncer struct {
	url  string
	ref  string
	dir  string
	auth transport.AuthMethod
}

func NewSyncer(url, ref, dir string, auth transport.AuthMethod) *Syncer {
	return &Syncer{url: url, ref: ref, dir: dir, auth: auth}
}

func (s *Syncer) Enabled() bool {
	return s.url != ""
}

func (s *Syncer) Sync(ctx context.Context, sha string) error {
	if s.url == "" {
		return nil
	}
	if s.dir == "" {
		return fmt.Errorf("repo dir is empty")
	}

	repo, err := git.PlainOpen(s.dir)
	if err == git.ErrRepositoryNotExists {
		if err := os.MkdirAll(filepath.Dir(s.dir), 0o755); err != nil {
			return fmt.Errorf("create repo parent: %w", err)
		}

		cloneOpts := &git.CloneOptions{URL: s.url, Auth: s.auth}
		if s.ref != "" {
			cloneOpts.ReferenceName = normalizeRef(s.ref)
			cloneOpts.SingleBranch = true
		}
		repo, err = git.PlainCloneContext(ctx, s.dir, false, cloneOpts)
		if err != nil {
			return fmt.Errorf("clone repo: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("open repo: %w", err)
	}

	fetchOpts := &git.FetchOptions{
		Auth: s.auth,
		RefSpecs: []config.RefSpec{
			"+refs/heads/*:refs/remotes/origin/*",
			"+refs/merge-requests/*:refs/merge-requests/*",
		},
	}
	if err := repo.FetchContext(ctx, fetchOpts); err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("fetch repo: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("worktree: %w", err)
	}

	if sha != "" {
		if err := wt.Checkout(&git.CheckoutOptions{Hash: plumbing.NewHash(sha), Force: true}); err != nil {
			return fmt.Errorf("checkout sha %s: %w", sha, err)
		}
		return nil
	}

	if s.ref != "" {
		if err := wt.Checkout(&git.CheckoutOptions{Branch: normalizeRef(s.ref), Force: true}); err != nil {
			return fmt.Errorf("checkout ref %s: %w", s.ref, err)
		}
	}

	return nil
}

func (s *Syncer) Maintain(_ context.Context) error {
	if !s.Enabled() {
		return nil
	}
	repo, err := git.PlainOpen(s.dir)
	if err == git.ErrRepositoryNotExists {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open repo: %w", err)
	}

	// Refresh remote-tracking refs and compact objects to bound long-lived clone growth.
	if err := repo.Fetch(&git.FetchOptions{
		Auth:       s.auth,
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			"+refs/heads/*:refs/remotes/origin/*",
			"+refs/merge-requests/*:refs/merge-requests/*",
		},
	}); err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("prune fetch: %w", err)
	}

	// Compact packfiles and prune old loose objects to keep disk usage bounded.
	if err := repo.RepackObjects(&git.RepackConfig{}); err != nil && err != git.ErrPackedObjectsNotSupported {
		// Repack/prune are best-effort; keep processing jobs even if GC cannot run.
		return nil
	}
	if err := repo.Prune(git.PruneOptions{
		OnlyObjectsOlderThan: time.Now().Add(-1 * time.Hour),
		Handler:              repo.DeleteObject,
	}); err != nil && err != git.ErrLooseObjectsNotSupported {
		return nil
	}
	return nil
}

func normalizeRef(ref string) plumbing.ReferenceName {
	if strings.HasPrefix(ref, "refs/") {
		return plumbing.ReferenceName(ref)
	}
	return plumbing.NewBranchReferenceName(ref)
}
