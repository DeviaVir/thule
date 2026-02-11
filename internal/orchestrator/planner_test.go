package orchestrator

import (
	"context"
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
	if err := os.WriteFile(filepath.Join(projectDir, "thule.yaml"), []byte(cfg), 0o644); err != nil {
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
	if got := len(comments.List(11)); got != 0 {
		t.Fatalf("expected no comments, got %d", got)
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
	if err := os.WriteFile(filepath.Join(projectDir, "thule.yaml"), []byte(cfg), 0o644); err != nil {
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
