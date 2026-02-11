package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMainPlanCommand(t *testing.T) {
	dir := t.TempDir()
	manifests := filepath.Join(dir, "manifests")
	if err := os.MkdirAll(manifests, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	manifest := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm1\n  namespace: default\n"
	if err := os.WriteFile(filepath.Join(manifests, "cm.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	cfg := "version: v1\nproject: demo\nclusterRef: demo-cluster\nnamespace: default\nrender:\n  mode: yaml\n  path: manifests\n"
	if err := os.WriteFile(filepath.Join(dir, "thule.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	oldArgs := os.Args
	oldStdout := os.Stdout
	t.Cleanup(func() {
		os.Args = oldArgs
		os.Stdout = oldStdout
	})

	os.Args = []string{"thule", "plan", "--project", dir, "--sha", "abc123"}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	main()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "## Thule Plan") {
		t.Fatalf("unexpected output: %s", string(out))
	}
}

func TestUsageOutput(t *testing.T) {
	oldStdout := os.Stdout
	t.Cleanup(func() { os.Stdout = oldStdout })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	usage()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "thule <command>") {
		t.Fatalf("unexpected usage output: %s", string(out))
	}
}
