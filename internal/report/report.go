package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/example/thule/internal/diff"
	"github.com/example/thule/internal/policy"
)

const (
	defaultMaxResourceDetails = 200
	maxCommentChars           = 900000
	maxYAMLCharsPerBlock      = 12000
)

type ProjectPlan struct {
	Project  string
	Changes  []diff.Change
	Summary  diff.Summary
	Findings []policy.Finding
}

func BuildPlanComment(project string, sha string, changes []diff.Change, summary diff.Summary, findings []policy.Finding, maxResourceDetails int) string {
	if maxResourceDetails <= 0 {
		maxResourceDetails = defaultMaxResourceDetails
	}

	var b strings.Builder
	b.WriteString("## Thule Plan\n\n")
	b.WriteString(fmt.Sprintf("Project: `%s`  \n", project))
	b.WriteString(fmt.Sprintf("Commit: `%s`\n\n", sha))
	b.WriteString(summaryLine(summary) + "\n\n")
	appendPlanSections(&b, changes, findings, maxResourceDetails, "### Changes", "### Policy Findings")
	b.WriteString("\n> Thule is read-only and did not apply these changes. Flux or repository operators must reconcile/apply.\n")
	return b.String()
}

func BuildAggregatedPlanComment(sha string, projects []ProjectPlan, maxResourceDetails int) string {
	if maxResourceDetails <= 0 {
		maxResourceDetails = defaultMaxResourceDetails
	}
	if len(projects) == 0 {
		return BuildNoChangesComment(sha, nil, 0)
	}

	visible := make([]ProjectPlan, 0, len(projects))
	for _, p := range projects {
		if hasActionableChanges(p) {
			visible = append(visible, p)
		}
	}
	if len(visible) == 0 {
		return BuildNoDiffComment(sha, len(projects))
	}

	sort.SliceStable(visible, func(i, j int) bool {
		return visible[i].Project < visible[j].Project
	})

	total := diff.Summary{}
	for _, p := range visible {
		total.Creates += p.Summary.Creates
		total.Patches += p.Summary.Patches
		total.Deletes += p.Summary.Deletes
		total.NoOps += p.Summary.NoOps
	}

	var b strings.Builder
	b.WriteString("## Thule Plan\n\n")
	b.WriteString(fmt.Sprintf("Commit: `%s`  \n", sha))
	b.WriteString(fmt.Sprintf("Projects: `%d`\n\n", len(visible)))
	b.WriteString(summaryLine(total) + "\n\n")

	for i, p := range visible {
		if i > 0 {
			b.WriteString("\n")
		}
		header := fmt.Sprintf("### Project: `%s`\n", p.Project)
		if b.Len()+len(header) > maxCommentChars {
			b.WriteString("\n- ... truncated (comment size limit)\n")
			break
		}
		b.WriteString(header)
		sLine := summaryLine(p.Summary) + "\n\n"
		if b.Len()+len(sLine) > maxCommentChars {
			b.WriteString("- ... truncated (comment size limit)\n")
			break
		}
		b.WriteString(sLine)
		appendPlanSections(&b, p.Changes, p.Findings, maxResourceDetails, "#### Changes", "#### Policy Findings")
	}

	b.WriteString("\n> Thule is read-only and did not apply these changes. Flux or repository operators must reconcile/apply.\n")
	return b.String()
}

func hasActionableChanges(plan ProjectPlan) bool {
	if plan.Summary.Creates > 0 || plan.Summary.Patches > 0 || plan.Summary.Deletes > 0 {
		return true
	}
	return len(plan.Findings) > 0
}

func appendPlanSections(b *strings.Builder, changes []diff.Change, findings []policy.Finding, maxResourceDetails int, changesHeading, findingsHeading string) {
	b.WriteString(changesHeading + "\n")
	printed := 0
	nonNoopTotal := 0
	for _, c := range changes {
		if c.Action != diff.NoOp {
			nonNoopTotal++
		}
	}
	sizeTruncated := false
	for _, c := range changes {
		if c.Action == diff.NoOp {
			continue
		}
		if printed >= maxResourceDetails {
			b.WriteString(fmt.Sprintf("- ... truncated (%d additional resources)\n", nonNoopTotal-printed))
			break
		}
		line := fmt.Sprintf("- `%s` %s", c.Action, c.ID)
		if len(c.ChangedKeys) > 0 {
			line += fmt.Sprintf(" changed=%v", c.ChangedKeys)
		}
		if len(c.ChangedPaths) > 0 {
			line += fmt.Sprintf(" paths=%v", c.ChangedPaths)
		}
		if len(c.Risks) > 0 {
			line += fmt.Sprintf(" risks=%v", c.Risks)
		}
		if b.Len()+len(line)+1 > maxCommentChars {
			sizeTruncated = true
			break
		}
		b.WriteString(line + "\n")
		if details := renderChangeDetails(c); details != "" {
			if b.Len()+len(details) > maxCommentChars {
				sizeTruncated = true
				break
			}
			b.WriteString(details)
		}
		printed++
	}
	if sizeTruncated {
		b.WriteString(fmt.Sprintf("- ... truncated (%d additional resources; comment size limit)\n", nonNoopTotal-printed))
	}
	if printed == 0 {
		b.WriteString("- none\n")
	}

	b.WriteString("\n" + findingsHeading + "\n")
	if len(findings) == 0 {
		b.WriteString("- none\n")
		return
	}
	for _, f := range findings {
		line := fmt.Sprintf("- `%s` `%s` %s (%s)\n", f.Severity, f.RuleID, f.Message, f.ResourceID)
		if b.Len()+len(line) > maxCommentChars {
			b.WriteString("- ... truncated (comment size limit)\n")
			return
		}
		b.WriteString(line)
	}
}

func summaryLine(summary diff.Summary) string {
	return fmt.Sprintf("Summary: CREATE=%d PATCH=%d DELETE=%d NO-OP=%d", summary.Creates, summary.Patches, summary.Deletes, summary.NoOps)
}

func renderChangeDetails(c diff.Change) string {
	switch c.Action {
	case diff.Create:
		if c.DesiredYAML == "" {
			return ""
		}
		return "\n```yaml\n# desired\n" + truncateYAMLBlock(c.DesiredYAML) + "\n```\n"
	case diff.Delete:
		if c.CurrentYAML == "" {
			return ""
		}
		return "\n```yaml\n# current\n" + truncateYAMLBlock(c.CurrentYAML) + "\n```\n"
	case diff.Patch:
		if len(c.AttributeDiff) > 0 {
			return "\n```diff\n" + truncateDiffLines(c.AttributeDiff) + "\n```\n"
		}
		parts := []string{}
		if c.CurrentYAML != "" {
			parts = append(parts, "```yaml\n# current\n"+truncateYAMLBlock(c.CurrentYAML)+"\n```")
		}
		if c.DesiredYAML != "" {
			parts = append(parts, "```yaml\n# desired\n"+truncateYAMLBlock(c.DesiredYAML)+"\n```")
		}
		if len(parts) == 0 {
			return ""
		}
		return "\n" + strings.Join(parts, "\n") + "\n"
	default:
		return ""
	}
}

func BuildNoChangesComment(sha string, changedFiles []string, maxFiles int) string {
	if maxFiles <= 0 {
		maxFiles = 50
	}
	var b strings.Builder
	b.WriteString("## Thule Plan\n\n")
	b.WriteString(fmt.Sprintf("Commit: `%s`\n\n", sha))
	b.WriteString("Summary: no diffs generated.\n")
	if len(changedFiles) == 0 {
		b.WriteString("Reason: no changed files were detected for this event.\n\n")
	} else {
		b.WriteString("Reason: changed files did not map to rendered Kubernetes resources in configured Thule projects.\n\n")
	}
	b.WriteString("### Changed files\n")
	if len(changedFiles) == 0 {
		b.WriteString("- none\n")
	} else {
		for i, f := range changedFiles {
			if i >= maxFiles {
				b.WriteString(fmt.Sprintf("- ... truncated (%d additional files)\n", len(changedFiles)-maxFiles))
				break
			}
			b.WriteString(fmt.Sprintf("- `%s`\n", f))
		}
	}
	b.WriteString("\n> Thule is read-only and did not apply any changes.\n")
	return b.String()
}

func BuildNoDiffComment(sha string, discoveredProjects int) string {
	var b strings.Builder
	b.WriteString("## Thule Plan\n\n")
	b.WriteString(fmt.Sprintf("Commit: `%s`\n\n", sha))
	b.WriteString("Summary: no CREATE/PATCH/DELETE changes for touched manifests.\n")
	if discoveredProjects > 0 {
		b.WriteString(fmt.Sprintf("Projects checked: `%d`\n", discoveredProjects))
	}
	b.WriteString("\n> Thule is read-only and did not apply any changes.\n")
	return b.String()
}

func truncateYAMLBlock(input string) string {
	trimmed := strings.TrimSpace(input)
	if len(trimmed) <= maxYAMLCharsPerBlock {
		return trimmed
	}
	return strings.TrimSpace(trimmed[:maxYAMLCharsPerBlock]) + "\n# ... truncated ..."
}

func truncateDiffLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	remaining := maxYAMLCharsPerBlock
	for i, line := range lines {
		if i > 0 {
			if remaining <= 1 {
				b.WriteString("\n# ... truncated ...")
				break
			}
			b.WriteByte('\n')
			remaining--
		}
		if len(line) <= remaining {
			b.WriteString(line)
			remaining -= len(line)
			continue
		}
		if remaining > 0 {
			b.WriteString(line[:remaining])
		}
		b.WriteString("\n# ... truncated ...")
		break
	}
	return strings.TrimSpace(b.String())
}
