package report

import (
	"strings"
	"testing"

	"github.com/example/thule/internal/diff"
)

func TestBuildPlanComment(t *testing.T) {
	body := BuildPlanComment("payments", "abc", []diff.Change{{ID: "x", Action: diff.Create}}, diff.Summary{Creates: 1})
	for _, want := range []string{"Thule Plan", "payments", "abc", "CREATE=1", "read-only"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body: %s", want, body)
		}
	}
}
