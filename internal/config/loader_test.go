package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeAndValidateAcceptsYAML(t *testing.T) {
	input := []byte("version: v1\nproject: payments\nclusterRef: prod-eu-1\nnamespace: payments\nrender:\n  mode: kustomize\n  path: .\n")
	cfg, err := Decode(input)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
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
	path := filepath.Join(tempDir, "thule.yaml")
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
