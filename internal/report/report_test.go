package report

import (
	"strings"
	"testing"

	"github.com/example/thule/internal/diff"
)

func TestBuildPlanComment(t *testing.T) {
	body := BuildPlanComment("payments", "abc", []diff.Change{{ID: "x", Action: diff.Create, ChangedKeys: []string{"spec"}, Risks: []string{"workload-spec-change"}}}, diff.Summary{Creates: 1}, 10)
	for _, want := range []string{"Thule Plan", "payments", "abc", "CREATE=1", "read-only", "changed=[spec]", "risks=[workload-spec-change]"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body: %s", want, body)
		}
	}
}

func TestBuildPlanCommentTruncates(t *testing.T) {
	changes := []diff.Change{{ID: "1", Action: diff.Create}, {ID: "2", Action: diff.Create}}
	body := BuildPlanComment("p", "sha", changes, diff.Summary{Creates: 2}, 1)
	if !strings.Contains(body, "truncated") {
		t.Fatalf("expected truncation marker: %s", body)
	}
}
