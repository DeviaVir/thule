package render

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/example/thule/pkg/thuleconfig"
)

func TestRenderProjectYAML(t *testing.T) {
	dir := t.TempDir()
	manifests := filepath.Join(dir, "manifests")
	if err := os.MkdirAll(manifests, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm1\n  namespace: ns1\n---\napiVersion: v1\nkind: Secret\nmetadata:\n  name: s1\n  namespace: ns1\n"
	if err := os.WriteFile(filepath.Join(manifests, "all.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := thuleconfig.Config{Render: thuleconfig.Render{Mode: "yaml", Path: "manifests"}}
	out, err := RenderProject(dir, cfg)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if len(out) != 2 || out[0].Name == "" {
		t.Fatalf("unexpected resources: %+v", out)
	}
}

func TestRenderProjectUnsupportedMode(t *testing.T) {
	_, err := RenderProject(".", thuleconfig.Config{Render: thuleconfig.Render{Mode: "helm", Path: "."}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRenderProjectKustomizePath(t *testing.T) {
	dir := t.TempDir()
	content := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm2\n"
	p := filepath.Join(dir, "k.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := thuleconfig.Config{Render: thuleconfig.Render{Mode: "kustomize", Path: "k.yaml"}}
	out, err := RenderProject(dir, cfg)
	if err != nil || len(out) != 1 {
		t.Fatalf("unexpected render result: %v %+v", err, out)
	}
}

func TestRenderProjectMissingPath(t *testing.T) {
	_, err := RenderProject(".", thuleconfig.Config{Render: thuleconfig.Render{Mode: "yaml", Path: "definitely-missing.yaml"}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseYAMLDocumentsRejectsInvalid(t *testing.T) {
	_, err := parseYAMLDocuments("kind: ConfigMap\nmetadata:\n  name: only\n")
	if err == nil {
		t.Fatal("expected parse error")
	}
}
