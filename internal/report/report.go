package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/example/thule/internal/diff"
	"github.com/example/thule/internal/guard"
	"github.com/example/thule/internal/policy"
)

const (
	defaultMaxResourceDetails = 200
	maxCommentChars           = 900000
	maxYAMLCharsPerBlock      = 12000
	collapsedChangesSummary   = "Show changes"
	collapsedFindingsSummary  = "Show policy findings"
	sizeLimitTruncationLine   = "- ... truncated (comment size limit)\n"
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
			b.WriteString("\n" + sizeLimitTruncationLine)
			break
		}
		b.WriteString(header)
		sLine := summaryLine(p.Summary) + "\n\n"
		if b.Len()+len(sLine) > maxCommentChars {
			b.WriteString(sizeLimitTruncationLine)
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
	changesHeadingLine := changesHeading + "\n"
	if b.Len()+len(changesHeadingLine) > maxCommentChars {
		if b.Len()+len(sizeLimitTruncationLine) <= maxCommentChars {
			b.WriteString(sizeLimitTruncationLine)
		}
		return
	}
	b.WriteString(changesHeadingLine)
	detailsStart := fmt.Sprintf("<details>\n<summary>%s</summary>\n\n", collapsedChangesSummary)
	detailsEnd := "\n</details>\n"
	if b.Len()+len(detailsStart)+len(detailsEnd) > maxCommentChars {
		if b.Len()+len(sizeLimitTruncationLine) <= maxCommentChars {
			b.WriteString(sizeLimitTruncationLine)
		}
		return
	}
	b.WriteString(detailsStart)
	changesLimit := maxCommentChars - len(detailsEnd)
	writeChangeContent := func(s string) bool {
		if b.Len()+len(s) > changesLimit {
			return false
		}
		b.WriteString(s)
		return true
	}
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
			if !writeChangeContent(fmt.Sprintf("- ... truncated (%d additional resources)\n", nonNoopTotal-printed)) {
				sizeTruncated = true
			}
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
		if !writeChangeContent(line + "\n") {
			sizeTruncated = true
			break
		}
		if details := renderChangeDetails(c); details != "" {
			if !writeChangeContent(details) {
				sizeTruncated = true
				break
			}
		}
		printed++
	}
	if sizeTruncated {
		if !writeChangeContent(fmt.Sprintf("- ... truncated (%d additional resources; comment size limit)\n", nonNoopTotal-printed)) {
			_ = writeChangeContent("- ... truncated\n")
		}
	}
	if nonNoopTotal == 0 {
		_ = writeChangeContent("- none\n")
	}
	b.WriteString(detailsEnd)

	findingsHeadingLine := "\n" + findingsHeading + "\n"
	if b.Len()+len(findingsHeadingLine) > maxCommentChars {
		if b.Len()+len(sizeLimitTruncationLine) <= maxCommentChars {
			b.WriteString(sizeLimitTruncationLine)
		}
		return
	}
	b.WriteString(findingsHeadingLine)
	findingsStart := fmt.Sprintf("<details>\n<summary>%s</summary>\n\n", collapsedFindingsSummary)
	findingsEnd := "\n</details>\n"
	if b.Len()+len(findingsStart)+len(findingsEnd) > maxCommentChars {
		if b.Len()+len(sizeLimitTruncationLine) <= maxCommentChars {
			b.WriteString(sizeLimitTruncationLine)
		}
		return
	}
	b.WriteString(findingsStart)
	findingsLimit := maxCommentChars - len(findingsEnd)
	writeFindingContent := func(s string) bool {
		if b.Len()+len(s) > findingsLimit {
			return false
		}
		b.WriteString(s)
		return true
	}
	if len(findings) == 0 {
		_ = writeFindingContent("- none\n")
		b.WriteString(findingsEnd)
		return
	}
	for _, f := range findings {
		line := fmt.Sprintf("- `%s` `%s` %s (%s)\n", f.Severity, f.RuleID, f.Message, f.ResourceID)
		if !writeFindingContent(line) {
			_ = writeFindingContent(sizeLimitTruncationLine)
			break
		}
	}
	b.WriteString(findingsEnd)
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

// BuildGuardWarning renders a banner for repository guard violations;
// callers prepend it to whatever plan comment is posted.
func BuildGuardWarning(violations []guard.Violation) string {
	if len(violations) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## :rotating_light: Guard violations\n\n")
	b.WriteString("This MR violates repository guards configured in `" + guard.ConfigFilename + "`.\n\n")
	for _, v := range violations {
		b.WriteString(fmt.Sprintf("- %s\n", v.Message()))
		if v.Guard.Description != "" {
			b.WriteString(fmt.Sprintf("  - %s\n", v.Guard.Description))
		}
	}
	b.WriteString("\n---\n\n")
	return b.String()
}
