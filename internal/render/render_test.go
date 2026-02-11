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

func TestRenderProjectHelmMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "rendered.yaml")
	content := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm2\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := thuleconfig.Config{Render: thuleconfig.Render{Mode: "helm", Path: "rendered.yaml"}}
	out, err := RenderProject(dir, cfg)
	if err != nil || len(out) != 1 {
		t.Fatalf("unexpected render result: %v %+v", err, out)
	}
}

func TestRenderProjectFluxModeIncludesAll(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "flux.yaml")
	content := "apiVersion: helm.toolkit.fluxcd.io/v2\nkind: HelmRelease\nmetadata:\n  name: hr\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := thuleconfig.Config{Render: thuleconfig.Render{Mode: "flux", Path: "flux.yaml"}}
	out, err := RenderProject(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("unexpected flux render count: %+v", out)
	}
}

func TestRenderProjectFluxModeIncludeKinds(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "flux.yaml")
	content := "apiVersion: helm.toolkit.fluxcd.io/v2\nkind: HelmRelease\nmetadata:\n  name: hr\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n---\napiVersion: v1\nkind: Secret\nmetadata:\n  name: s1\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := thuleconfig.Config{Render: thuleconfig.Render{Mode: "flux", Path: "flux.yaml", Flux: thuleconfig.Flux{IncludeKinds: []string{"ConfigMap"}}}}
	out, err := RenderProject(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("unexpected flux filtering: %+v", out)
	}
	kinds := map[string]struct{}{}
	for _, r := range out {
		kinds[r.Kind] = struct{}{}
	}
	if _, ok := kinds["HelmRelease"]; !ok {
		t.Fatalf("expected HelmRelease included: %+v", out)
	}
	if _, ok := kinds["ConfigMap"]; !ok {
		t.Fatalf("expected ConfigMap included: %+v", out)
	}
}

func TestRenderProjectUnsupportedMode(t *testing.T) {
	_, err := RenderProject(".", thuleconfig.Config{Render: thuleconfig.Render{Mode: "bad", Path: "."}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRenderProjectMissingPath(t *testing.T) {
	_, err := RenderProject(".", thuleconfig.Config{Render: thuleconfig.Render{Mode: "yaml", Path: "definitely-missing.yaml"}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseYAMLDocumentsRejectsInvalid(t *testing.T) {
	out, err := parseYAMLDocuments("kind: ConfigMap\nmetadata:\n  name: only\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected invalid manifest to be skipped, got %+v", out)
	}
}

func TestResourceIDUsesClusterNamespaceFallback(t *testing.T) {
	r := Resource{APIVersion: "v1", Kind: "ConfigMap", Name: "cm1"}
	if got := r.ID(); got != "v1|ConfigMap|_cluster|cm1" {
		t.Fatalf("unexpected id: %s", got)
	}
	r.Namespace = "ns1"
	if got := r.ID(); got != "v1|ConfigMap|ns1|cm1" {
		t.Fatalf("unexpected id with namespace: %s", got)
	}
}
