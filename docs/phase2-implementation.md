# Phase 2 Implementation Notes

This update delivers a practical Phase 2 expansion with focus on rendering modes and diff/report depth.

## Delivered

- **Render modes**
  - Add `helm` mode support (consumes rendered manifests path).
  - Add `flux` mode support with kind filtering (`HelmRelease`, `Kustomization`, `GitRepository`, `OCIRepository` by default, customizable via config).
- **Diff controls**
  - Add per-project `diff.prune` control to include/exclude delete actions.
  - Add per-project `diff.ignoreFields` path removal before comparison.
- **Richer change plan output**
  - Patch changes now include `changed` keys and `risks` tags.
  - Built-in risk tags include workload spec changes, metadata changes, and CRD changes.
  - Comment output respects `comment.maxResourceDetails` and truncates with marker when exceeded.

## Scope notes

- Helm mode expects rendered YAML path for this iteration.
- Flux mode is kind-aware filtering in this phase (not full source graph resolution).
