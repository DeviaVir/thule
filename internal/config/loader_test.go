package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeAndValidateAcceptsYAML(t *testing.T) {
	input := []byte("version: v1\nproject: payments\nclusterRef: prod-eu-1\nnamespace: payments\nrender:\n  mode: kustomize\n  path: .\ndiff:\n  prune: true\n  ignoreFields:\n    - metadata.annotations\ncomment:\n  maxResourceDetails: 25\n")
	cfg, err := Decode(input)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !cfg.Diff.Prune || len(cfg.Diff.IgnoreFields) != 1 || cfg.Comment.MaxResourceDetails != 25 {
		t.Fatalf("expected parsed phase2 fields: %+v", cfg)
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestValidateBytesRejectsInvalidConfigs(t *testing.T) {
	tests := []string{"version", "version: v1\nproject: p\n", "version: v1\nproject: p\nclusterRef: c\nnamespace: n\nrender:\n  mode: unknown\n  path: .\n"}
	for _, tc := range tests {
		if err := ValidateBytes([]byte(tc)); err == nil {
			t.Fatal("expected validation error")
		}
	}
}

func TestLoadReadsAndValidatesFile(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "thule.conf")
	payload := "version: v1\nproject: payments\nclusterRef: prod-eu-1\nnamespace: payments\nrender:\n  mode: yaml\n  path: ./manifests\n"
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.Project != "payments" || cfg.Render.Path != "./manifests" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestDecodeJSONConfig(t *testing.T) {
	input := []byte(`{"version":"v1","project":"p","clusterRef":"c","namespace":"n","render":{"mode":"yaml","path":"."}}`)
	cfg, err := Decode(input)
	if err != nil {
		t.Fatalf("decode json failed: %v", err)
	}
	if cfg.Project != "p" || cfg.Render.Mode != "yaml" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestDecodeSimpleYAMLLists(t *testing.T) {
	input := []byte("version: v1\nproject: p\nclusterRef: c\nnamespace: n\nrender:\n  mode: flux\n  path: .\n  flux.includeKinds:\n    - Kustomization\n    - HelmRelease\n  helm.valuesFiles:\n    - values.yaml\n")
	cfg, err := Decode(input)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(cfg.Render.Flux.IncludeKinds) != 2 || cfg.Render.Flux.IncludeKinds[0] != "Kustomization" {
		t.Fatalf("unexpected includeKinds: %+v", cfg.Render.Flux.IncludeKinds)
	}
	if len(cfg.Render.Helm.ValuesFiles) != 1 || cfg.Render.Helm.ValuesFiles[0] != "values.yaml" {
		t.Fatalf("unexpected values files: %+v", cfg.Render.Helm.ValuesFiles)
	}
}

func TestDecodeRejectsInvalidPayload(t *testing.T) {
	if _, err := Decode([]byte("not: valid")); err == nil {
		t.Fatal("expected error for invalid payload")
	}
}
