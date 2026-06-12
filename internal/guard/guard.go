// Package guard implements repository-level merge request guards.
//
// Guards are MR-wide checks configured by the target repository in a
// .thule.yaml file at its root. Unlike per-project policy findings, guards
// look at the shape of the whole change set (which paths are touched
// together), so repositories can encode rules like "do not roll the same
// app in more than one failure domain in a single MR" without Thule
// knowing anything about the repository's layout.
package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigFilename is resolved relative to the repository root.
const ConfigFilename = ".thule.yaml"

// TypeSameAppAcrossGroups guards a tree laid out as
// <prefix>/<group>/<app>/... and fails when one MR modifies the same app
// under two or more groups. "Group" is whatever failure domain the path
// segment encodes: a region, a site, a shard.
const TypeSameAppAcrossGroups = "same-app-across-groups"

type Config struct {
	Guards []Spec `yaml:"guards"`
	// FollowUp optionally posts an extra comment after each plan comment,
	// e.g. to trigger a downstream bot once the plan exists. Placeholders
	// {sha} and {summary} are substituted.
	FollowUp FollowUp `yaml:"followUp"`
}

type FollowUp struct {
	Comment string `yaml:"comment"`
}

type Spec struct {
	// Name identifies the guard in comments and commit statuses.
	Name string `yaml:"name"`
	// Description is shown to MR authors when the guard fires.
	Description string `yaml:"description"`
	// Type selects the guard implementation.
	Type string `yaml:"type"`
	// Prefix is the path prefix the guard watches, e.g. "clusters/prod".
	Prefix string `yaml:"prefix"`
	// Exempt lists app names the guard ignores (e.g. "flux-system").
	Exempt []string `yaml:"exempt"`
}

type Violation struct {
	Guard  Spec
	App    string
	Groups []string
}

func (v Violation) Message() string {
	return fmt.Sprintf("guard %q: app %q is modified in multiple groups (%s) under %s; split the change so each group rolls independently",
		v.Guard.Name, v.App, strings.Join(v.Groups, ", "), v.Guard.Prefix)
}

// LoadConfig reads <repoRoot>/.thule.yaml. A missing file is not an error;
// it returns an empty config. A present but invalid file is an error so
// misconfigured guards fail loudly instead of silently not guarding.
func LoadConfig(repoRoot string) (Config, error) {
	content, err := os.ReadFile(filepath.Join(repoRoot, ConfigFilename))
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("read %s: %w", ConfigFilename, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", ConfigFilename, err)
	}
	for i, g := range cfg.Guards {
		if strings.TrimSpace(g.Name) == "" {
			return Config{}, fmt.Errorf("%s: guards[%d]: name is required", ConfigFilename, i)
		}
		if g.Type != TypeSameAppAcrossGroups {
			return Config{}, fmt.Errorf("%s: guard %q: unknown type %q", ConfigFilename, g.Name, g.Type)
		}
		if strings.TrimSpace(g.Prefix) == "" {
			return Config{}, fmt.Errorf("%s: guard %q: prefix is required", ConfigFilename, g.Name)
		}
	}
	return cfg, nil
}

// Evaluate runs every configured guard against the MR's changed files.
// The second return reports whether any guarded prefix was touched at all,
// so callers can publish a green status once a previously violating MR is
// split, while staying silent on unrelated MRs.
func Evaluate(cfg Config, changedFiles []string) ([]Violation, bool) {
	violations := []Violation{}
	touched := false
	for _, g := range cfg.Guards {
		vs, t := evaluateSameAppAcrossGroups(g, changedFiles)
		violations = append(violations, vs...)
		touched = touched || t
	}
	return violations, touched
}

func evaluateSameAppAcrossGroups(g Spec, changedFiles []string) ([]Violation, bool) {
	exempt := map[string]struct{}{}
	for _, e := range g.Exempt {
		exempt[e] = struct{}{}
	}
	prefix := strings.TrimSuffix(filepath.ToSlash(g.Prefix), "/") + "/"

	touched := false
	groupsByApp := map[string]map[string]struct{}{}
	for _, f := range changedFiles {
		clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(f)))
		rest, ok := strings.CutPrefix(clean, prefix)
		if !ok {
			continue
		}
		touched = true
		parts := strings.Split(rest, "/")
		// need at least group/app/file
		if len(parts) < 3 {
			continue
		}
		group, app := parts[0], parts[1]
		if _, ok := exempt[app]; ok {
			continue
		}
		if groupsByApp[app] == nil {
			groupsByApp[app] = map[string]struct{}{}
		}
		groupsByApp[app][group] = struct{}{}
	}

	violations := []Violation{}
	apps := make([]string, 0, len(groupsByApp))
	for app := range groupsByApp {
		apps = append(apps, app)
	}
	sort.Strings(apps)
	for _, app := range apps {
		if len(groupsByApp[app]) < 2 {
			continue
		}
		groups := make([]string, 0, len(groupsByApp[app]))
		for grp := range groupsByApp[app] {
			groups = append(groups, grp)
		}
		sort.Strings(groups)
		violations = append(violations, Violation{Guard: g, App: app, Groups: groups})
	}
	return violations, touched
}
