# thule Agents Guide

This file is the source of truth for agent instructions in this repository.
If you are an agent working anywhere under this repo, read this file first and follow it.

## Scope

- Keep entries specific to Thule workflows, code paths, and runbook details.
- Keep entries concise and practical (paths, commands, gotchas).

## Update rule

- Add or revise one entry when work reveals a Thule-specific lesson; keep generic guidance out. Git history is the changelog.

## Entry template

- **What**: Short description of the task or skill.
  **Where**: Folder/file path(s) it applies to.
  **How**: Minimal steps or implementation detail.
  **Gotchas**: Caveats, limits, or behavior differences.
  **Owner/Docs**: Team or local doc reference (if known).

## Skills & context

- **What**: Thule plan comments collapse change details and policy findings by default to keep large PR comments/notes readable.
  **Where**: `internal/report/report.go`, `internal/report/report_test.go`
  **How**: Wrap `Changes` and `Policy Findings` section contents in markdown `<details><summary>...</summary> ... </details>` while keeping summary lines visible.
  **Gotchas**: Keep truncation safeguards intact (`maxCommentChars`, `maxYAMLCharsPerBlock`) so massive comments still hard-limit safely; reserve room for closing `</details>` before writing collapsible content to avoid malformed markdown.
  **Owner/Docs**: DevOps / Thule

- **What**: Unit coverage gate is strict at 90%; report size-limit branches need explicit tests to prevent regressions below threshold.
  **Where**: `scripts/check_coverage.sh`, `.github/workflows/unit-tests.yml`, `internal/report/report_test.go`
  **How**: Reproduce with `go test ./internal/... ./pkg/... -covermode=atomic -coverprofile=unit.out` then `./scripts/check_coverage.sh 90 unit.out`; keep tests for `appendPlanSections` overflow paths (changes details start/end, findings start, oversized findings lines).
  **Gotchas**: CI runs in Go 1.25; if using `golang:1.25` container, ensure `/usr/local/go/bin` is on `PATH` when invoking `go` from `sh`.
  **Owner/Docs**: DevOps / Thule
