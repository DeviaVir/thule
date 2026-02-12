package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/example/thule/internal/policy"
	"github.com/example/thule/internal/render"
	"github.com/example/thule/internal/run"
	"github.com/example/thule/internal/vcs"
)

func TestPlannerPlanForEvent(t *testing.T) {
	repo := t.TempDir()
	projectDir := filepath.Join(repo, "apps", "payments")
	if err := os.MkdirAll(filepath.Join(projectDir, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "version: v1\nproject: payments\nclusterRef: prod\nnamespace: payments\npolicy:\n  profile: strict\nrender:\n  mode: yaml\n  path: manifests\n"
	if err := os.WriteFile(filepath.Join(projectDir, "thule.conf"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := "apiVersion: v1\nkind: Secret\nmetadata:\n  name: cm\n  namespace: payments\n"
	if err := os.WriteFile(filepath.Join(projectDir, "manifests", "cm.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	cluster := &MemoryClusterReader{ByClusterNS: map[string][]render.Resource{"prod/payments": {}}}
	comments := vcs.NewMemoryCommentStore()
	statuses := vcs.NewMemoryStatusPublisher()
	runs := run.NewMemoryStore()
	planner := NewPlanner(repo, cluster, comments, statuses, runs, policy.NewBuiltinEvaluator())

	evt := MergeRequestEvent{MergeReqID: 10, HeadSHA: "abc", ChangedFiles: []string{"apps/payments/manifests/cm.yaml"}}
	if err := planner.PlanForEvent(context.Background(), evt); err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	items := comments.List(10)
	if len(items) != 1 {
		t.Fatalf("expected one comment, got %d", len(items))
	}
	if !strings.Contains(items[0].Body, "Policy Findings") {
		t.Fatalf("expected policy findings in comment: %s", items[0].Body)
	}
	if len(statuses.ListStatuses(10, "abc")) < 2 {
		t.Fatal("expected pending and success statuses")
	}
	if got := runs.List(10, 1, 10); len(got) != 1 || got[0].State != run.StateSuccess {
		t.Fatalf("expected successful run record, got %+v", got)
	}
}

func TestPlannerSkipsMissingConfig(t *testing.T) {
	repo := t.TempDir()
	comments := vcs.NewMemoryCommentStore()
	planner := NewPlanner(repo, &MemoryClusterReader{}, comments, nil, nil, nil)
	evt := MergeRequestEvent{MergeReqID: 11, HeadSHA: "abc", ChangedFiles: []string{"apps/ghost/file.yaml"}}
	if err := planner.PlanForEvent(context.Background(), evt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	items := comments.List(11)
	if got := len(items); got != 1 {
		t.Fatalf("expected one comment, got %d", got)
	}
	if !strings.Contains(items[0].Body, "no diffs generated") {
		t.Fatalf("expected no-change comment, got: %s", items[0].Body)
	}
}

type errCluster struct{}

func (e *errCluster) ListResources(_ context.Context, _, _ string) ([]render.Resource, error) {
	return nil, os.ErrPermission
}

func TestPlannerSetsFailedStatusOnClusterError(t *testing.T) {
	repo := t.TempDir()
	projectDir := filepath.Join(repo, "apps", "payments")
	if err := os.MkdirAll(filepath.Join(projectDir, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "version: v1\nproject: payments\nclusterRef: prod\nnamespace: payments\nrender:\n  mode: yaml\n  path: manifests\n"
	if err := os.WriteFile(filepath.Join(projectDir, "thule.conf"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n"
	if err := os.WriteFile(filepath.Join(projectDir, "manifests", "cm.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	comments := vcs.NewMemoryCommentStore()
	statuses := vcs.NewMemoryStatusPublisher()
	runs := run.NewMemoryStore()
	planner := NewPlanner(repo, &errCluster{}, comments, statuses, runs, policy.NewBuiltinEvaluator())
	evt := MergeRequestEvent{MergeReqID: 22, HeadSHA: "abc", ChangedFiles: []string{"apps/payments/manifests/cm.yaml"}}
	if err := planner.PlanForEvent(context.Background(), evt); err == nil {
		t.Fatal("expected error")
	}
	ss := statuses.ListStatuses(22, "abc")
	if len(ss) < 2 || ss[len(ss)-1].State != vcs.CheckFailed {
		t.Fatalf("expected failed status, got %+v", ss)
	}
	rr := runs.List(22, 1, 10)
	if len(rr) != 1 || rr[0].State != run.StateFailed {
		t.Fatalf("expected failed run, got %+v", rr)
	}
}

func TestPlannerStaleRunStopsEarly(t *testing.T) {
	repo := t.TempDir()
	runs := run.NewMemoryStore()
	runs.SetLatestSHA(99, "newer")
	planner := NewPlanner(repo, &MemoryClusterReader{}, vcs.NewMemoryCommentStore(), vcs.NewMemoryStatusPublisher(), runs, nil)
	evt := MergeRequestEvent{MergeReqID: 99, HeadSHA: "older", ChangedFiles: []string{"apps/a/x.yaml"}}
	if err := planner.PlanForEvent(context.Background(), evt); err != nil {
		t.Fatalf("expected no error on stale early exit: %v", err)
	}
}

func TestPlannerAggregatesMultipleProjectsIntoSingleComment(t *testing.T) {
	repo := t.TempDir()

	writeProject := func(name, ns string) {
		t.Helper()
		projectDir := filepath.Join(repo, "apps", name)
		if err := os.MkdirAll(filepath.Join(projectDir, "manifests"), 0o755); err != nil {
			t.Fatal(err)
		}
		cfg := fmt.Sprintf("version: v1\nproject: %s\nclusterRef: prod\nnamespace: %s\nrender:\n  mode: yaml\n  path: manifests\n", name, ns)
		if err := os.WriteFile(filepath.Join(projectDir, "thule.conf"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
		manifest := fmt.Sprintf("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: %s\n  namespace: %s\n", name, ns)
		if err := os.WriteFile(filepath.Join(projectDir, "manifests", "cm.yaml"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeProject("alpha", "team-a")
	writeProject("beta", "team-b")

	cluster := &MemoryClusterReader{ByClusterNS: map[string][]render.Resource{
		"prod/team-a": {},
		"prod/team-b": {},
	}}
	comments := vcs.NewMemoryCommentStore()
	runs := run.NewMemoryStore()
	planner := NewPlanner(repo, cluster, comments, vcs.NewMemoryStatusPublisher(), runs, policy.NewBuiltinEvaluator())

	evt := MergeRequestEvent{
		MergeReqID:   42,
		HeadSHA:      "abc123",
		ChangedFiles: []string{"apps/alpha/manifests/cm.yaml", "apps/beta/manifests/cm.yaml"},
	}
	if err := planner.PlanForEvent(context.Background(), evt); err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	items := comments.List(42)
	if len(items) != 1 {
		t.Fatalf("expected one aggregated comment, got %d", len(items))
	}
	body := items[0].Body
	for _, want := range []string{"Projects: `2`", "### Project: `alpha`", "### Project: `beta`"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in aggregated comment: %s", want, body)
		}
	}
	if got := runs.List(42, 1, 10); len(got) != 2 {
		t.Fatalf("expected two run records, got %+v", got)
	}
}

func TestPlannerMarksEarlierRunsFailedWhenLaterProjectConfigFails(t *testing.T) {
	repo := t.TempDir()

	alphaDir := filepath.Join(repo, "apps", "alpha")
	if err := os.MkdirAll(filepath.Join(alphaDir, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	alphaCfg := "version: v1\nproject: alpha\nclusterRef: prod\nnamespace: team-a\nrender:\n  mode: yaml\n  path: manifests\n"
	if err := os.WriteFile(filepath.Join(alphaDir, "thule.conf"), []byte(alphaCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	alphaManifest := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: alpha\n  namespace: team-a\n"
	if err := os.WriteFile(filepath.Join(alphaDir, "manifests", "cm.yaml"), []byte(alphaManifest), 0o644); err != nil {
		t.Fatal(err)
	}

	betaDir := filepath.Join(repo, "apps", "beta")
	if err := os.MkdirAll(betaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Missing required fields to force config.Load error.
	if err := os.WriteFile(filepath.Join(betaDir, "thule.conf"), []byte("version: v1\nproject: beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cluster := &MemoryClusterReader{ByClusterNS: map[string][]render.Resource{"prod/team-a": {}}}
	runs := run.NewMemoryStore()
	planner := NewPlanner(repo, cluster, vcs.NewMemoryCommentStore(), vcs.NewMemoryStatusPublisher(), runs, policy.NewBuiltinEvaluator())
	evt := MergeRequestEvent{
		MergeReqID:   43,
		HeadSHA:      "abc124",
		ChangedFiles: []string{"apps/alpha/manifests/cm.yaml", "apps/beta/thule.conf"},
	}
	if err := planner.PlanForEvent(context.Background(), evt); err == nil {
		t.Fatal("expected planner error")
	}
	records := runs.List(43, 1, 10)
	if len(records) == 0 {
		t.Fatalf("expected at least one run record, got %+v", records)
	}
	if records[0].State != run.StateFailed {
		t.Fatalf("expected earlier run to be marked failed, got %+v", records[0])
	}
}

func TestPlannerOnlyPlansChangedManifestFiles(t *testing.T) {
	repo := t.TempDir()
	projectDir := filepath.Join(repo, "apps", "payments")
	if err := os.MkdirAll(filepath.Join(projectDir, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "version: v1\nproject: payments\nclusterRef: prod\nnamespace: payments\nrender:\n  mode: yaml\n  path: manifests\n"
	if err := os.WriteFile(filepath.Join(projectDir, "thule.conf"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "manifests", "a.yaml"), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: only-changed\n  namespace: payments\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "manifests", "b.yaml"), []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: untouched\n  namespace: payments\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cluster := &MemoryClusterReader{ByClusterNS: map[string][]render.Resource{"prod/payments": {}}}
	comments := vcs.NewMemoryCommentStore()
	planner := NewPlanner(repo, cluster, comments, vcs.NewMemoryStatusPublisher(), run.NewMemoryStore(), policy.NewBuiltinEvaluator())

	evt := MergeRequestEvent{MergeReqID: 55, HeadSHA: "abc", ChangedFiles: []string{"apps/payments/manifests/a.yaml"}}
	if err := planner.PlanForEvent(context.Background(), evt); err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	body := comments.List(55)[0].Body
	if !strings.Contains(body, "only-changed") {
		t.Fatalf("expected changed resource in plan comment: %s", body)
	}
	if strings.Contains(body, "untouched") {
		t.Fatalf("did not expect untouched resource in plan comment: %s", body)
	}
}

func TestFilterDesiredByChangedFiles(t *testing.T) {
	desired := []render.Resource{
		{Name: "a", SourcePath: "/repo/apps/p/manifests/a.yaml"},
		{Name: "b", SourcePath: "/repo/apps/p/manifests/b.yaml"},
		{Name: "no-source", SourcePath: ""},
	}
	filtered := filterDesiredByChangedFiles(desired, []string{
		"apps/p/manifests/a.yaml",
		"README.md",
	}, "/repo")
	if len(filtered) != 1 || filtered[0].Name != "a" {
		t.Fatalf("unexpected filtered resources: %+v", filtered)
	}

	if got := filterDesiredByChangedFiles(desired, []string{"README.md"}, "/repo"); got != nil {
		t.Fatalf("expected nil result when no yaml files changed, got %+v", got)
	}
	if got := filterDesiredByChangedFiles(desired, nil, "/repo"); len(got) != len(desired) {
		t.Fatalf("expected unchanged resources when changed_files is empty, got %+v", got)
	}
}
