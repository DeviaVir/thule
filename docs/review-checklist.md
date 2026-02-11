# Architecture & Code Review Checklist

This checklist reviews implementation against `docs/thule-architecture-roadmap.md`.

## 1) Does Thule provide plans?

**Yes (prototype level).**

- Planner computes create/patch/delete/no-op diffs.
- Comments include summary, per-resource details, risk tags, policy findings, and read-only disclaimer.

## 2) Does it avoid apply behavior?

**Yes.**

- No apply call exists in API/worker/planner paths.
- Output explicitly states Thule is read-only.

## 3) Does it support GitLab integration?

**Yes (webhook + command routing).**

- Merge request webhooks are translated to plan events.
- Note events with `/thule plan` are routed as manual plan triggers.

## 3.5) Atlantis-style project locking

**Yes (memory-backed prototype).**

- MR events attempt to acquire locks per discovered project root.
- Conflicting MRs touching same folder receive lock conflict errors.
- Close/merge events release locks for the owning MR.

## 3.6) Optional approval behavior

**Yes (optional in-memory prototype).**

- Successful plans can mark MR as approved.
- Lock conflicts and planning failures request changes.

## 4) Phase alignment

- Phase 0: service skeleton + schema + webhook/queue foundation ✅
- Phase 1: project discovery + render + diff + plan comment ✅
- Phase 2: helm/flux render extensions + diff controls/risk + richer reporting ✅
- Phase 3: policy integration + run tracking/status/pagination hooks ✅
- Phase 4: command routing + local CLI parity + docs/quality pass ✅

## 5) Remaining gaps to production

- Persistent queue, run store, and status/comment adapters (GitLab API/Postgres).
- Real cluster reader credentials and multi-cluster auth policy.
- Full Helm/Flux source graph resolution.
- Structured API endpoints for historical drift dashboard.
