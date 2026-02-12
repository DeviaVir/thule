package report

import (
	"strings"
	"testing"

	"github.com/example/thule/internal/diff"
	"github.com/example/thule/internal/policy"
)

func TestBuildPlanComment(t *testing.T) {
	body := BuildPlanComment("payments", "abc", []diff.Change{{ID: "x", Action: diff.Create, ChangedKeys: []string{"spec"}, ChangedPaths: []string{"spec.replicas"}, Risks: []string{"workload-spec-change"}, DesiredYAML: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n    name: x\n"}}, diff.Summary{Creates: 1}, []policy.Finding{{RuleID: "r1", Severity: policy.SeverityWarn, Message: "m1", ResourceID: "id1"}}, 10)
	for _, want := range []string{"Thule Plan", "payments", "abc", "CREATE=1", "read-only", "changed=[spec]", "paths=[spec.replicas]", "risks=[workload-spec-change]", "Policy Findings", "r1", "# desired"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body: %s", want, body)
		}
	}
}

func TestBuildPlanCommentTruncates(t *testing.T) {
	changes := []diff.Change{{ID: "1", Action: diff.Create}, {ID: "2", Action: diff.Create}}
	body := BuildPlanComment("p", "sha", changes, diff.Summary{Creates: 2}, nil, 1)
	if !strings.Contains(body, "truncated") {
		t.Fatalf("expected truncation marker: %s", body)
	}
}

func TestBuildPlanCommentTruncatesOnCommentSizeLimit(t *testing.T) {
	blob := strings.Repeat("x", maxYAMLCharsPerBlock)
	changes := make([]diff.Change, 0, 120)
	for i := 0; i < 120; i++ {
		changes = append(changes, diff.Change{
			ID:          "r",
			Action:      diff.Create,
			DesiredYAML: blob,
		})
	}
	body := BuildPlanComment("p", "sha", changes, diff.Summary{Creates: len(changes)}, nil, len(changes))
	if !strings.Contains(body, "comment size limit") {
		t.Fatalf("expected size-limit truncation, got: %s", body)
	}
}

func TestRenderChangeDetailsTruncatesLargeYAML(t *testing.T) {
	huge := strings.Repeat("a", maxYAMLCharsPerBlock+100)
	body := renderChangeDetails(diff.Change{Action: diff.Create, DesiredYAML: huge})
	if !strings.Contains(body, "# ... truncated ...") {
		t.Fatalf("expected yaml truncation marker, got: %s", body)
	}
}

func TestBuildNoChangesComment(t *testing.T) {
	body := BuildNoChangesComment("sha", []string{"apps/a.yaml", "apps/b.yaml"}, 1)
	if !strings.Contains(body, "no diffs generated") {
		t.Fatalf("expected no-change summary, got %s", body)
	}
	if !strings.Contains(body, "did not match any configured Thule project") {
		t.Fatalf("expected reason for unmatched project config, got %s", body)
	}
	if !strings.Contains(body, "apps/a.yaml") {
		t.Fatalf("expected changed file list, got %s", body)
	}
	if !strings.Contains(body, "truncated") {
		t.Fatalf("expected truncation note, got %s", body)
	}
}

func TestBuildNoChangesCommentWithEmptyFiles(t *testing.T) {
	body := BuildNoChangesComment("sha", nil, 10)
	if !strings.Contains(body, "- none") {
		t.Fatalf("expected none marker, got %s", body)
	}
	if !strings.Contains(body, "no changed files were detected") {
		t.Fatalf("expected empty-changes reason, got %s", body)
	}
}

func TestRenderPatchDetails(t *testing.T) {
	body := BuildPlanComment("p", "sha", []diff.Change{{
		ID:          "x",
		Action:      diff.Patch,
		ChangedKeys: []string{"metadata"},
		AttributeDiff: []string{
			"- metadata.labels.app: \"old\"",
			"+ metadata.labels.app: \"new\"",
		},
	}}, diff.Summary{Patches: 1}, nil, 10)
	for _, want := range []string{"```diff", "- metadata.labels.app: \"old\"", "+ metadata.labels.app: \"new\""} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body: %s", want, body)
		}
	}
	for _, dontWant := range []string{"# current", "# desired"} {
		if strings.Contains(body, dontWant) {
			t.Fatalf("unexpected %q in body: %s", dontWant, body)
		}
	}
}

func TestRenderChangeDetailsCreateDeleteAndNoop(t *testing.T) {
	createDetails := renderChangeDetails(diff.Change{
		Action:      diff.Create,
		DesiredYAML: "kind: ConfigMap\nmetadata:\n  name: create-me\n",
	})
	if !strings.Contains(createDetails, "# desired") {
		t.Fatalf("expected desired details for create, got: %s", createDetails)
	}

	deleteDetails := renderChangeDetails(diff.Change{
		Action:      diff.Delete,
		CurrentYAML: "kind: ConfigMap\nmetadata:\n  name: delete-me\n",
	})
	if !strings.Contains(deleteDetails, "# current") {
		t.Fatalf("expected current details for delete, got: %s", deleteDetails)
	}

	if got := renderChangeDetails(diff.Change{Action: diff.NoOp}); got != "" {
		t.Fatalf("expected empty details for noop, got: %s", got)
	}

	if got := renderChangeDetails(diff.Change{Action: "UNKNOWN"}); got != "" {
		t.Fatalf("expected empty details for unknown action, got: %s", got)
	}
}

func TestBuildPlanCommentNoopOnlyShowsNone(t *testing.T) {
	body := BuildPlanComment("p", "sha", []diff.Change{{ID: "x", Action: diff.NoOp}}, diff.Summary{NoOps: 1}, nil, 10)
	if !strings.Contains(body, "NO-OP=1") {
		t.Fatalf("expected noop summary, got: %s", body)
	}
	if !strings.Contains(body, "### Changes\n- none") {
		t.Fatalf("expected no-op details suppressed, got: %s", body)
	}
}

func TestBuildAggregatedPlanComment(t *testing.T) {
	body := BuildAggregatedPlanComment("sha", []ProjectPlan{
		{
			Project: "a",
			Changes: []diff.Change{{ID: "x", Action: diff.Create}},
			Summary: diff.Summary{Creates: 1},
			Findings: []policy.Finding{
				{RuleID: "r1", Severity: policy.SeverityWarn, Message: "warn", ResourceID: "x"},
			},
		},
		{
			Project: "b",
			Changes: []diff.Change{{ID: "y", Action: diff.Patch}},
			Summary: diff.Summary{Patches: 1},
		},
	}, 10)
	for _, want := range []string{
		"Projects: `2`",
		"Summary: CREATE=1 PATCH=1 DELETE=0 NO-OP=0",
		"### Project: `a`",
		"### Project: `b`",
		"#### Changes",
		"#### Policy Findings",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body: %s", want, body)
		}
	}
}

func TestBuildAggregatedPlanCommentHidesNoChangeProjects(t *testing.T) {
	body := BuildAggregatedPlanComment("sha", []ProjectPlan{
		{
			Project: "changed",
			Changes: []diff.Change{{ID: "x", Action: diff.Patch}},
			Summary: diff.Summary{Patches: 1},
		},
		{
			Project: "unchanged",
			Changes: []diff.Change{{ID: "y", Action: diff.NoOp}},
			Summary: diff.Summary{NoOps: 1},
		},
	}, 10)
	if strings.Contains(body, "### Project: `unchanged`") {
		t.Fatalf("expected unchanged project hidden, got: %s", body)
	}
	if !strings.Contains(body, "### Project: `changed`") {
		t.Fatalf("expected changed project included, got: %s", body)
	}
	if !strings.Contains(body, "Projects: `1`") {
		t.Fatalf("expected visible project count, got: %s", body)
	}
}

func TestBuildAggregatedPlanCommentNoProjects(t *testing.T) {
	body := BuildAggregatedPlanComment("sha", nil, 10)
	if !strings.Contains(body, "no diffs generated") {
		t.Fatalf("expected no-project summary, got: %s", body)
	}
}

func TestBuildAggregatedPlanCommentAllNoChangeUsesNoDiffComment(t *testing.T) {
	body := BuildAggregatedPlanComment("sha", []ProjectPlan{
		{Project: "a", Summary: diff.Summary{NoOps: 2}},
		{Project: "b", Summary: diff.Summary{NoOps: 1}},
	}, 10)
	if !strings.Contains(body, "no CREATE/PATCH/DELETE changes") {
		t.Fatalf("expected no-diff summary, got: %s", body)
	}
	if strings.Contains(body, "### Project:") {
		t.Fatalf("did not expect project sections, got: %s", body)
	}
}

func TestBuildAggregatedPlanCommentSortsProjects(t *testing.T) {
	body := BuildAggregatedPlanComment("sha", []ProjectPlan{
		{Project: "zeta", Summary: diff.Summary{Creates: 1}},
		{Project: "alpha", Summary: diff.Summary{Creates: 1}},
	}, 1)
	if strings.Index(body, "### Project: `alpha`") > strings.Index(body, "### Project: `zeta`") {
		t.Fatalf("expected alphabetical project order, got: %s", body)
	}
}

func TestRenderPatchDetailsFallsBackToYAML(t *testing.T) {
	body := renderChangeDetails(diff.Change{
		Action:      diff.Patch,
		CurrentYAML: "kind: ConfigMap\nmetadata:\n  name: before\n",
		DesiredYAML: "kind: ConfigMap\nmetadata:\n  name: after\n",
	})
	for _, want := range []string{"# current", "# desired"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body: %s", want, body)
		}
	}
}

func TestTruncateDiffLines(t *testing.T) {
	if got := truncateDiffLines(nil); got != "" {
		t.Fatalf("expected empty diff block, got %q", got)
	}

	short := truncateDiffLines([]string{"- a: 1", "+ a: 2"})
	for _, want := range []string{"- a: 1", "+ a: 2"} {
		if !strings.Contains(short, want) {
			t.Fatalf("missing %q in %q", want, short)
		}
	}

	longLine := strings.Repeat("x", maxYAMLCharsPerBlock+64)
	truncated := truncateDiffLines([]string{"- a: 1", "+" + longLine})
	if !strings.Contains(truncated, "# ... truncated ...") {
		t.Fatalf("expected truncation marker, got %q", truncated)
	}
}

func TestBuildNoDiffComment(t *testing.T) {
	body := BuildNoDiffComment("sha", 3)
	for _, want := range []string{"sha", "no CREATE/PATCH/DELETE changes", "Projects checked: `3`"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in body: %s", want, body)
		}
	}
}
