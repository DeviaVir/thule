package project

import "testing"

func TestDiscoverFromChangedFiles(t *testing.T) {
	projects := DiscoverFromChangedFiles([]string{"apps/payments/deploy.yaml", "apps/payments/values.yaml", "README.md", ""})
	if len(projects) == 0 {
		t.Fatal("expected discovered candidates")
	}
}

func TestDiscoverFromChangedFilesEmpty(t *testing.T) {
	projects := DiscoverFromChangedFiles(nil)
	if len(projects) != 0 {
		t.Fatalf("expected no projects, got %d", len(projects))
	}
}
