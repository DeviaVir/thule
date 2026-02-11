package report

import (
	"fmt"
	"strings"

	"github.com/example/thule/internal/diff"
)

func BuildPlanComment(project string, sha string, changes []diff.Change, summary diff.Summary) string {
	var b strings.Builder
	b.WriteString("## Thule Plan\n\n")
	b.WriteString(fmt.Sprintf("Project: `%s`  \n", project))
	b.WriteString(fmt.Sprintf("Commit: `%s`\n\n", sha))
	b.WriteString(fmt.Sprintf("Summary: CREATE=%d PATCH=%d DELETE=%d NO-OP=%d\n\n", summary.Creates, summary.Patches, summary.Deletes, summary.NoOps))
	b.WriteString("### Changes\n")
	for _, c := range changes {
		b.WriteString(fmt.Sprintf("- `%s` %s\n", c.Action, c.ID))
	}
	b.WriteString("\n> Thule is read-only and did not apply these changes. Flux or repository operators must reconcile/apply.\n")
	return b.String()
}
