package report

import (
	"fmt"
	"strings"

	"github.com/example/thule/internal/diff"
	"github.com/example/thule/internal/policy"
)

func BuildPlanComment(project string, sha string, changes []diff.Change, summary diff.Summary, findings []policy.Finding, maxResourceDetails int) string {
	if maxResourceDetails <= 0 {
		maxResourceDetails = 200
	}
	var b strings.Builder
	b.WriteString("## Thule Plan\n\n")
	b.WriteString(fmt.Sprintf("Project: `%s`  \n", project))
	b.WriteString(fmt.Sprintf("Commit: `%s`\n\n", sha))
	b.WriteString(fmt.Sprintf("Summary: CREATE=%d PATCH=%d DELETE=%d NO-OP=%d\n\n", summary.Creates, summary.Patches, summary.Deletes, summary.NoOps))
	b.WriteString("### Changes\n")
	for i, c := range changes {
		if i >= maxResourceDetails {
			b.WriteString(fmt.Sprintf("- ... truncated (%d additional resources)\n", len(changes)-maxResourceDetails))
			break
		}
		line := fmt.Sprintf("- `%s` %s", c.Action, c.ID)
		if len(c.ChangedKeys) > 0 {
			line += fmt.Sprintf(" changed=%v", c.ChangedKeys)
		}
		if len(c.Risks) > 0 {
			line += fmt.Sprintf(" risks=%v", c.Risks)
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\n### Policy Findings\n")
	if len(findings) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, f := range findings {
			b.WriteString(fmt.Sprintf("- `%s` `%s` %s (%s)\n", f.Severity, f.RuleID, f.Message, f.ResourceID))
		}
	}

	b.WriteString("\n> Thule is read-only and did not apply these changes. Flux or repository operators must reconcile/apply.\n")
	return b.String()
}
