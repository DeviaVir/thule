# Thule Architecture Plan & Roadmap (Atlantis-for-Kubernetes)

## 1. Vision and Product Boundaries

**Goal:** Build `thule` as a Merge Request (MR) automation system for Kubernetes-native GitOps repos, analogous to Atlantis for Terraform, but intentionally focused on **YAML/Helm/Flux Kubernetes changes**.

### Core principles

1. **MR-first UX**: all feedback is posted directly on the MR.
2. **Read-only by design**: Thule never applies changes to clusters.
3. **Deterministic previews**: same commit should produce same rendered + diff output.
4. **Cluster-aware context**: diff is computed against the target cluster defined by repo config.
5. **Fast iteration loop**: every MR update triggers a new diff and supersedes old output.

### Explicit non-goals (v1)

- No direct `kubectl apply` / server-side apply by Thule.
- No cluster mutation webhooks.
- No replacing Flux/Argo reconciliation ownership.

---

## 2. User Workflow (v1)

1. Developer opens or updates an MR changing manifests under a configured folder.
2. Thule detects affected deployment units and resolves target cluster(s) from local config (e.g., `thule.yaml`).
3. Thule renders manifests (raw YAML / Kustomize / Helm / Flux artifacts where applicable).
4. Thule performs a read-only compare against live cluster objects.
5. Thule posts an MR comment with:
   - summary (create/update/delete/no-op counts),
   - per-resource change plan,
   - safety/policy findings,
   - explicit note that Thule does not apply and Flux/MR author must do so.
6. On new MR commits or metadata-relevant updates, Thule reruns and posts a new comment while marking previous Thule plan comment as superseded (collapsed/hidden/edited per VCS API capability).

---

## 3. High-Level Architecture

## 3.1 Components

1. **Webhook/API Gateway**
   - Receives MR events (opened, updated/synchronize, reopened, label changes, comment commands).
   - Validates signatures and enqueues jobs.

2. **Job Orchestrator**
   - Deduplicates events per MR/commit SHA.
   - Handles retries, concurrency limits, cancellation of stale runs.

3. **Repo Fetcher + Workspace Builder**
   - Clones target SHA.
   - Identifies changed paths and impacted Thule projects.

4. **Project Resolver**
   - Reads per-folder config (e.g., `thule.yaml`).
   - Resolves cluster context, namespace scopes, render mode, policy profiles.

5. **Renderer Engine**
   - Supports:
     - Plain YAML
     - Kustomize builds
     - Helm template (with values overlays)
     - Flux-oriented sources (HelmRelease/Kustomization resolution strategy)

6. **Cluster State Reader**
   - Uses read-only credentials to query live objects.
   - Normalizes fields to reduce noise (managedFields, status, timestamps, etc.).

7. **Diff Engine**
   - Produces per-resource action (`CREATE`, `PATCH`, `DELETE`, `NO-OP`).
   - Supports structural diff and strategic merge patch views.
   - Computes risk score (e.g., immutable field changes, workload restarts).

8. **Policy & Guardrails Engine (optional in v1, recommended early)**
   - Runs OPA/Conftest/Kyverno policies on rendered manifests.
   - Annotates plan with warnings/errors.

9. **MR Comment Publisher**
   - Creates/upserts a canonical Thule plan comment.
   - Collapses or marks previous comments as superseded.
   - Adds commit SHA and run ID for traceability.

10. **Storage & Metadata**
   - Tracks run history, MR-to-comment mapping, superseded comment IDs.

11. **Observability Stack**
   - Structured logs, traces, metrics, run dashboards.

## 3.2 Deployment topology

- **Thule Control Plane**: stateless API + workers (Kubernetes deployment).
- **Queue**: SQS/NATS/Redis streams/Kafka (pick one for event durability).
- **State DB**: Postgres for run metadata and idempotency.
- **Secrets**: External secret manager (Vault/Cloud provider secret service).
- **Cluster Access**:
  - Prefer per-cluster read-only service account / OIDC role.
  - Optional hub-and-spoke credential brokering.

---

## 4. Repository Configuration Model

Use a per-project config file, e.g.:

```yaml
# apps/payments/thule.yaml
version: v1
project: payments-app
clusterRef: prod-eu-1
namespace: payments
render:
  mode: kustomize # yaml|kustomize|helm|flux
  path: .
  helm:
    releaseName: payments
    valuesFiles:
      - values-prod.yaml
diff:
  prune: false
  ignoreFields:
    - metadata.annotations["kubectl.kubernetes.io/last-applied-configuration"]
policy:
  profile: baseline
comment:
  maxResourceDetails: 150
```

### Config layering

1. Global org defaults.
2. Repo-level `.thule/config.yaml`.
3. Project-level `thule.yaml`.
4. MR overrides via labels/comments (allowlist only).

---

## 5. Event & Execution Lifecycle

1. **Event intake**
   - Trigger on MR open/update/reopen + optional `/thule plan` comment command.
2. **Change detection**
   - Map changed files to project roots containing `thule.yaml`.
3. **Execution graph**
   - Fan out one job per impacted project/cluster pair.
4. **Render + diff**
   - Render desired state.
   - Fetch actual state.
   - Compute normalized diff.
5. **Aggregate report**
   - Roll up job outputs into one MR comment with expandable sections.
6. **Supersede old plan**
   - Edit prior comment header to "Superseded by run <id>" and collapse if provider supports.
7. **Finalize status checks**
   - Set commit status (success/warn/fail).

### Idempotency & stale update handling

- Key by `{mr_id, head_sha, project_id}`.
- If newer SHA appears, cancel in-flight older jobs.
- Ignore duplicate webhooks using delivery ID + SHA cache.

---

## 6. Diff Semantics and Output

### Diff classes

- **Create**: object absent in cluster.
- **Patch**: object exists and meaningful spec/metadata diff.
- **Delete**: object present but removed from desired set (optional/prune-gated).
- **No-op**: equivalent after normalization.

### Normalization strategy

Ignore noisy fields by default:
- `metadata.managedFields`
- `metadata.resourceVersion`
- `metadata.uid`
- `metadata.creationTimestamp`
- `status`

Allow per-project custom ignore rules.

### MR comment structure

1. Run metadata (project count, duration, commit SHA).
2. Summary table by project.
3. Resource-by-resource change details.
4. Policy results.
5. Explicit disclaimer:
   - "Thule is read-only and did not apply these changes. Flux (or repository operators) remains responsible for reconciliation."

---

## 7. Security and Compliance

1. **Least privilege RBAC**
   - `get/list/watch` only where possible.
2. **Credential isolation**
   - Per-cluster credentials scoped by namespace/project.
3. **Auditability**
   - Every run tied to MR SHA + actor + clusterRef.
4. **Supply-chain hardening**
   - Pin renderer tool versions (helm/kustomize/kubectl libs).
5. **Multi-tenant controls**
   - Repo/org allowlists and clusterRef access policies.

---

## 8. Scale and Performance

- Parallelize by project with queue backpressure.
- Cache repo clones and rendered artifacts keyed by SHA.
- Batch Kubernetes reads by GVK/namespace when possible.
- Enforce per-run output limits (truncate with downloadable artifact links).

---

## 9. Compatibility with Atlantis-style Features

Atlantis-inspired capabilities to consider:

1. **Autoplan equivalent**: auto-run on MR updates.
2. **Manual rerun commands**: `/thule plan [project]`.
3. **Project-level isolation**: only changed projects run.
4. **Policy checks as required status**.
5. **Locks (read lock flavor)**: avoid noisy concurrent plans on same project/SHA.
6. **Custom workflows**: per-project render/diff customization.
7. **Comment-driven UX**: single authoritative bot comment per SHA.

Features to intentionally defer:
- Apply workflows.
- Drift auto-remediation.
- ChatOps mutation commands.

---

## 10. Proposed Go Package Structure

```text
cmd/thule-api
cmd/thule-worker
internal/
  webhook/
  vcs/
  queue/
  orchestrator/
  repo/
  project/
  render/
    yaml/
    kustomize/
    helm/
    flux/
  cluster/
  diff/
  policy/
  report/
  storage/
  auth/
  observability/
pkg/
  thuleconfig/
  kubernetes/
  patchview/
```

---

## 11. Milestone Roadmap

## Phase 0 — Foundations (1–2 weeks)

- Finalize product scope and terminology.
- Define `thule.yaml` schema + JSONSchema validation.
- Choose VCS target first (GitLab or GitHub) and design adapter interface.
- Establish local dev harness with kind cluster + fixture repo.

**Exit criteria:** RFC approved; skeleton services booting; webhook to queue path proven.

## Phase 1 — Minimal Viable Planner (3–5 weeks)

- Webhook intake for MR open/update.
- Changed-path to project resolution.
- YAML + Kustomize render support.
- Live cluster read + normalized diff.
- Single MR plan comment with summary + disclaimer.
- Supersede previous plan comment on MR updates.

**Exit criteria:** End-to-end MR update loop works for YAML/Kustomize projects.

## Phase 2 — Helm + Flux-Aware Expansion (3–4 weeks)

- Helm template renderer with values overlays.
- Flux resource awareness (HelmRelease/Kustomization path interpretation).
- Better diff visualization (inline patches + risk flags).
- Per-project ignore rules and prune behavior controls.

**Exit criteria:** Mixed YAML/Kustomize/Helm repos supported for core planning workflows.

## Phase 3 — Policies, Reliability, and Enterprise Readiness (4–6 weeks)

- OPA/Conftest policy integration.
- Idempotency hardening, retries, stale run cancellation.
- Status checks + run artifacts + pagination.
- Multi-cluster credential governance and audit exports.

**Exit criteria:** Production pilot with 3–5 teams and defined SLOs.

## Phase 4 — Developer Experience & Ecosystem (ongoing)

- `/thule plan` command routing.
- Rich MR UX (collapsible sections, component ownership tagging).
- Historical drift snapshots and trend dashboard.
- Optional IDE/CLI local preview parity (`thule plan --local`).

---

## 12. Risks and Mitigations

1. **Render vs runtime divergence**
   - Mitigate with strict tool version pinning + render metadata in comments.
2. **Cluster access sprawl**
   - Mitigate with brokered short-lived credentials and per-project policies.
3. **Noisy diffs reduce trust**
   - Mitigate via robust normalization defaults + opt-in strict mode.
4. **Large MR output limits**
   - Mitigate by summarizing + attaching full report artifact.
5. **Flux timing mismatch**
   - Mitigate by clearly labeling that plan is pre-reconciliation estimate.

---

## 13. Suggested Next Build Step

Start with **Phase 1 slice**:

- implement `thule.yaml` parser + validator,
- webhook ingestion for MR synchronize events,
- project discovery from changed files,
- YAML/Kustomize render + normalized diff,
- one canonical superseding MR comment.

This gives immediate user value and a stable base for Helm/Flux enhancements.
