# Phase 1 Implementation Notes

This change implements the Phase 1 core planning slice:

- webhook event ingestion with idempotent enqueue semantics,
- changed-file project discovery (`thule.yaml` rooted projects),
- YAML/Kustomize render path support,
- normalized resource diffing (`CREATE/PATCH/DELETE/NO-OP`),
- canonical MR plan comment generation with read-only disclaimer,
- superseding previous plan comments in a VCS comment store.

## Important fixes included

1. **Atomic dedupe reserve**
   - delivery dedupe now uses `Reserve` in one atomic operation.
2. **No event loss on enqueue failure**
   - reservation is released when enqueue fails; committed only after successful enqueue.
3. **YAML config support**
   - config loader now decodes JSON first and falls back to YAML parsing for advertised `thule.yaml` files.

## Current limits

- Cluster reads and VCS comments are in-memory adapters for now.
- Kustomize mode currently consumes YAML from the configured path in this Phase 1 cut.
