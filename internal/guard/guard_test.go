package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ConfigFilename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadConfigMissingFileIsEmpty(t *testing.T) {
	cfg, err := LoadConfig(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Guards) != 0 {
		t.Fatalf("expected empty config, got %+v", cfg)
	}
}

func TestLoadConfigValidatesGuards(t *testing.T) {
	cases := map[string]string{
		"missing name":   "guards:\n  - type: same-app-across-groups\n    prefix: a/b\n",
		"unknown type":   "guards:\n  - name: g\n    type: nope\n    prefix: a/b\n",
		"missing prefix": "guards:\n  - name: g\n    type: same-app-across-groups\n",
		"bad yaml":       "guards: [",
	}
	for name, content := range cases {
		if _, err := LoadConfig(writeConfig(t, content)); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}
}

func validSpec() Spec {
	return Spec{
		Name:   "prod-regions",
		Type:   TypeSameAppAcrossGroups,
		Prefix: "clusters/prod",
		Exempt: []string{"flux-system"},
	}
}

func TestEvaluateFlagsSameAppAcrossGroups(t *testing.T) {
	cfg := Config{Guards: []Spec{validSpec()}}
	violations, touched := Evaluate(cfg, []string{
		"clusters/prod/eu/db/statefulset.yaml",
		"clusters/prod/us/db/statefulset.yaml",
		"clusters/prod/asia/cache/deploy.yaml",
	})
	if !touched {
		t.Fatal("expected touched")
	}
	if len(violations) != 1 {
		t.Fatalf("expected one violation, got %+v", violations)
	}
	v := violations[0]
	if v.App != "db" || strings.Join(v.Groups, ",") != "eu,us" {
		t.Fatalf("unexpected violation: %+v", v)
	}
	if !strings.Contains(v.Message(), "prod-regions") {
		t.Fatalf("message should name the guard: %s", v.Message())
	}
}

func TestEvaluateSingleGroupIsClean(t *testing.T) {
	cfg := Config{Guards: []Spec{validSpec()}}
	violations, touched := Evaluate(cfg, []string{
		"clusters/prod/eu/db/statefulset.yaml",
		"clusters/prod/eu/cache/deploy.yaml",
		"clusters/staging/eu/db/x.yaml", // different prefix
		"unrelated/file.txt",
	})
	if !touched {
		t.Fatal("expected touched")
	}
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %+v", violations)
	}
}

func TestEvaluateExemptAndUntouched(t *testing.T) {
	cfg := Config{Guards: []Spec{validSpec()}}
	violations, touched := Evaluate(cfg, []string{
		"clusters/prod/eu/flux-system/gotk-components.yaml",
		"clusters/prod/us/flux-system/gotk-components.yaml",
	})
	if len(violations) != 0 {
		t.Fatalf("exempt app should not violate, got %+v", violations)
	}
	if !touched {
		t.Fatal("expected touched for guarded prefix")
	}

	_, touched = Evaluate(cfg, []string{"clusters/staging/eu/db/x.yaml"})
	if touched {
		t.Fatal("expected untouched outside guarded prefix")
	}
}

func TestEvaluateMultipleGuards(t *testing.T) {
	second := validSpec()
	second.Name = "edge-sites"
	second.Prefix = "clusters/edge"
	cfg := Config{Guards: []Spec{validSpec(), second}}
	violations, _ := Evaluate(cfg, []string{
		"clusters/prod/eu/db/a.yaml",
		"clusters/prod/us/db/a.yaml",
		"clusters/edge/site1/cdn/a.yaml",
		"clusters/edge/site2/cdn/a.yaml",
	})
	if len(violations) != 2 {
		t.Fatalf("expected two violations, got %+v", violations)
	}
}
