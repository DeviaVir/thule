package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAcceptsValidConfig(t *testing.T) {
	input := []byte(`{
		"version":"v1",
		"project":"payments",
		"clusterRef":"prod-eu-1",
		"namespace":"payments",
		"render":{"mode":"kustomize","path":"."}
	}`)

	if err := Validate(input); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestValidateRejectsInvalidConfigs(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{
			name: "invalid-json",
			json: `{"version"`,
		},
		{
			name: "missing-required-fields",
			json: `{"version":"v1","project":"p"}`,
		},
		{
			name: "invalid-render-mode",
			json: `{"version":"v1","project":"payments","clusterRef":"prod-eu-1","namespace":"payments","render":{"mode":"unknown","path":"."}}`,
		},
		{
			name: "missing-render-path",
			json: `{"version":"v1","project":"payments","clusterRef":"prod-eu-1","namespace":"payments","render":{"mode":"yaml","path":""}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate([]byte(tc.json)); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestLoadReadsAndValidatesFile(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "thule.json")
	payload := `{
		"version":"v1",
		"project":"payments",
		"clusterRef":"prod-eu-1",
		"namespace":"payments",
		"render":{"mode":"yaml","path":"./manifests"}
	}`
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

func TestLoadMissingFileReturnsError(t *testing.T) {
	if _, err := Load("/definitely/missing.json"); err == nil {
		t.Fatal("expected error for missing file")
	}
}
