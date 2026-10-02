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

- **What**: Equivalent Kubernetes quantities must not produce a PATCH, changed path, diff line, or risk.
  **Where**: `internal/diff/diff.go`, `internal/diff/quantity_test.go`
  **How**: After live-field projection, `normalizeQuantityPairs` walks copied bodies and reuses desired quantity representations only when `parseQuantity` (exact `math/big` value) succeeds on both values and they are equal; recognize quantity map leaves and `sizeLimit` at any nesting depth.
  **Gotchas**: Do not import `k8s.io/apimachinery/pkg/api/resource`: it makes the go command rewrite go.mod to `go 1.25.0`, which the GitLab `code_intelligence_go` job (lsif-go, Go 1.18) cannot parse. The parser is stricter than apimachinery (rejects `.`, `+`, exponents beyond +/-1000, and does not round below `1n`), which only keeps a PATCH visible. Keep env values and resource claims as ordinary strings. Normalize after projection so injected list items do not shift quantity pairing.
  **Owner/Docs**: DevOps / Thule

- **What**: Keyed list projection hides injected items while preserving apply-owned removals and real reorders.
  **Where**: `internal/diff/diff.go`, `internal/diff/diff_test.go`
  **How**: With `IgnoreActualExtraFields`, pair list-of-map items by the first unique scalar key from `name`, `mountPath`, `containerPort`, `port`, `devicePath`, trying fields used by the apply manager's FieldsV1 `k:` entries first; project matched live items in live order with their `k:` ownership subtrees and keep owned live-only items verbatim.
  **Gotchas**: SSA records list-key fields as owned even when defaulted (`ports[].protocol`), so strip them from the item's ownership subtree or they render as removals. Unowned extras (Reloader `STAKATER_*` env) disappear; lists without a qualifying key keep positional projection.
  **Owner/Docs**: DevOps / Thule

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

- **What**: Empty apply ownership can describe a typed-round-trip default, but also a real atomic-field removal.
  **Where**: `internal/diff/diff.go`, `internal/diff/defaults_test.go`
  **How**: During live-field projection, omit an absent desired key with exactly empty merged ownership only for an empty map/list or an exact match to one of the `emptyOwnershipDefaults` forms by API version, kind, and path. StatefulSet updateStrategy accepts RollingUpdate with partition 0 or no rollingUpdate key. JSON comparison preserves numeric equivalence without equating numbers and strings.
  **Gotchas**: Keep non-default atomic structs, child or `.` ownership, and defaults at other kinds/versions/paths visible. The explicit table covers apps/v1 Deployment strategy and StatefulSet/DaemonSet updateStrategy; do not generalize empty ownership to unowned.
  **Owner/Docs**: DevOps / Thule
