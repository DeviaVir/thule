package repo

import (
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func ChangedFiles(repoDir, baseRef, headSHA string) ([]string, error) {
	if repoDir == "" {
		return nil, fmt.Errorf("repo dir is empty")
	}
	if headSHA == "" {
		return nil, fmt.Errorf("head sha is empty")
	}
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	baseHash, err := resolveRef(repo, baseRef)
	if err != nil {
		return nil, err
	}

	headCommit, err := repo.CommitObject(plumbing.NewHash(headSHA))
	if err != nil {
		return nil, fmt.Errorf("head commit: %w", err)
	}
	baseCommit, err := repo.CommitObject(baseHash)
	if err != nil {
		return nil, fmt.Errorf("base commit: %w", err)
	}
	mergeBase := baseCommit
	if bases, err := headCommit.MergeBase(baseCommit); err == nil && len(bases) > 0 {
		mergeBase = bases[0]
	}

	patch, err := mergeBase.Patch(headCommit)
	if err != nil {
		return nil, fmt.Errorf("diff commits: %w", err)
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(patch.FilePatches()))
	for _, fp := range patch.FilePatches() {
		from, to := fp.Files()
		path := ""
		switch {
		case to != nil:
			path = to.Path()
		case from != nil:
			path = from.Path()
		}
		if path == "" {
			continue
		}
		path = strings.TrimPrefix(path, "./")
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out, nil
}

func resolveRef(repo *git.Repository, ref string) (plumbing.Hash, error) {
	if ref == "" {
		return plumbing.Hash{}, fmt.Errorf("base ref is empty")
	}
	if plumbing.IsHash(ref) {
		return plumbing.NewHash(ref), nil
	}

	candidates := []plumbing.ReferenceName{}
	if strings.HasPrefix(ref, "refs/") {
		candidates = append(candidates, plumbing.ReferenceName(ref))
	} else {
		// Prefer origin/<ref>; local branches can be stale in long-lived worker clones.
		candidates = append(candidates, plumbing.NewRemoteReferenceName("origin", ref))
		candidates = append(candidates, plumbing.NewBranchReferenceName(ref))
	}
	for _, name := range candidates {
		r, err := repo.Reference(name, true)
		if err == nil {
			return r.Hash(), nil
		}
	}
	return plumbing.Hash{}, fmt.Errorf("base ref %q not found", ref)
}
