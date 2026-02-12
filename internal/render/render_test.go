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
	if out[0].SourcePath == "" || out[1].SourcePath == "" {
		t.Fatalf("expected source paths on rendered resources: %+v", out)
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

func TestParseYAMLDocumentsNestedKindDoesNotOverrideResourceKind(t *testing.T) {
	content := "apiVersion: kustomize.toolkit.fluxcd.io/v1\nkind: Kustomization\nmetadata:\n  name: flux-system\n  namespace: flux-system\nspec:\n  sourceRef:\n    kind: GitRepository\n    name: flux-system\n"
	out, err := parseYAMLDocuments(content)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one resource, got %+v", out)
	}
	if out[0].Kind != "Kustomization" {
		t.Fatalf("expected Kustomization kind, got %s", out[0].Kind)
	}
}

func TestParseYAMLDocumentsInvalidYAMLReturnsError(t *testing.T) {
	if _, err := parseYAMLDocuments("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: ["); err == nil {
		t.Fatal("expected yaml parse error")
	}
}

func TestRenderProjectSkipsInvalidNonManifestYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "values.yaml"), []byte("foo: bar\nfoo: baz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n"
	if err := os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := RenderProject(dir, thuleconfig.Config{Render: thuleconfig.Render{Mode: "yaml", Path: "."}})
	if err != nil {
		t.Fatalf("expected non-manifest invalid yaml to be ignored, got %v", err)
	}
	if len(out) != 1 || out[0].Kind != "ConfigMap" {
		t.Fatalf("unexpected resources: %+v", out)
	}
}

func TestRenderProjectFailsForInvalidManifestYAML(t *testing.T) {
	dir := t.TempDir()
	content := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: [\n"
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderProject(dir, thuleconfig.Config{Render: thuleconfig.Render{Mode: "yaml", Path: "."}}); err == nil {
		t.Fatal("expected parse error for malformed manifest file")
	}
}

func TestLooksLikeKubernetesManifest(t *testing.T) {
	if !looksLikeKubernetesManifest("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: x\n") {
		t.Fatal("expected manifest pattern match")
	}
	if looksLikeKubernetesManifest("foo: bar\nkindness: true\n") {
		t.Fatal("did not expect non-manifest pattern match")
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
