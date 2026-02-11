package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/example/thule/internal/render"
	"github.com/example/thule/internal/vcs"
)

func TestPlannerPlanForEvent(t *testing.T) {
	repo := t.TempDir()
	projectDir := filepath.Join(repo, "apps", "payments")
	if err := os.MkdirAll(filepath.Join(projectDir, "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "version: v1\nproject: payments\nclusterRef: prod\nnamespace: payments\nrender:\n  mode: yaml\n  path: manifests\n"
	if err := os.WriteFile(filepath.Join(projectDir, "thule.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n  namespace: payments\n"
	if err := os.WriteFile(filepath.Join(projectDir, "manifests", "cm.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	cluster := &MemoryClusterReader{ByClusterNS: map[string][]render.Resource{"prod/payments": {}}}
	comments := vcs.NewMemoryCommentStore()
	planner := NewPlanner(repo, cluster, comments)

	evt := MergeRequestEvent{MergeReqID: 10, HeadSHA: "abc", ChangedFiles: []string{"apps/payments/manifests/cm.yaml"}}
	if err := planner.PlanForEvent(context.Background(), evt); err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	items := comments.List(10)
	if len(items) != 1 {
		t.Fatalf("expected one comment, got %d", len(items))
	}
	if items[0].Body == "" {
		t.Fatal("expected comment body")
	}
}

func TestPlannerSkipsMissingConfig(t *testing.T) {
	repo := t.TempDir()
	comments := vcs.NewMemoryCommentStore()
	planner := NewPlanner(repo, &MemoryClusterReader{}, comments)
	evt := MergeRequestEvent{MergeReqID: 11, HeadSHA: "abc", ChangedFiles: []string{"apps/ghost/file.yaml"}}
	if err := planner.PlanForEvent(context.Background(), evt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := len(comments.List(11)); got != 0 {
		t.Fatalf("expected no comments, got %d", got)
	}
}
