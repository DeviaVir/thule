# Thule

**Thule is a read-only merge request planner for Kubernetes GitOps repositories** — think *Atlantis, but for Flux/kustomize/Helm repos*.

When someone opens or updates a merge request against your GitOps repo, Thule renders the Kubernetes resources that MR would produce, diffs them against what is actually running in the target cluster, and posts the result as an MR comment — before anyone merges. It **never applies anything**; Flux (or whatever reconciles your repo) remains the only write path to the cluster.

```
GitLab MR webhook ──► thule-api ──► Redis queue ──► thule-worker
                                                       │
                                          clone repo, discover projects,
                                          render manifests, diff vs live
                                          cluster state (read-only)
                                                       │
                                    ◄── plan comment + commit status ───┘
```

## Why

GitOps closes the loop *after* merge: a bad change is discovered when Flux applies it. Thule moves that discovery to review time:

- **Plan comments** — a `terraform plan`-style summary (CREATE / PATCH / DELETE / no-op) per project, with rendered resource detail, posted on the MR and superseded on every push.
- **Commit statuses** — `thule/plan` reflects plan success/failure, so you can make it a required check.
- **Policy findings** — built-in checks (e.g. Secret changes, cluster-wide RBAC changes) surface in the plan comment.
- **Repository guards** — MR-wide rules over the shape of the change set (e.g. "don't roll the same app in two regions in one MR"), configured per repo in `.thule.yaml`.
- **Locking** — Atlantis-style per-project locks prevent conflicting parallel plans on the same paths.

## How it works

1. **`thule-api`** receives GitLab MR webhooks (secret-validated), deduplicates deliveries, and enqueues jobs to Redis.
2. **`thule-worker`** dequeues, syncs a local clone of the repo, and discovers affected *projects*: for every changed file it walks up the directory tree looking for a `thule.conf`.
3. For each project it renders the desired state (`yaml`, `kustomize`, `helm`, or `flux` mode), lists the corresponding live resources from the target cluster (read-only; `get`/`list`/`watch` is all the RBAC it needs), and computes a diff.
4. The aggregated plan is posted as an MR comment (previous plans are marked superseded) and a commit status is set.

Stale runs are abandoned when a newer SHA arrives; run records and artifacts are kept for debugging.

## Quick start

Prerequisites: Go 1.22+, and Redis if you run api/worker (unit tests need neither).

```bash
# run the test suite
go test ./...

# render + diff + policy locally, printing the plan comment body
go run ./cmd/thule plan --project ./apps/payments --sha local
```

Run the service pair:

```bash
# API: receives webhooks, enqueues
THULE_API_ADDR=:8080 THULE_WEBHOOK_SECRET=supersecret go run ./cmd/thule-api

# worker: plans
THULE_REPO_ROOT=$(pwd) go run ./cmd/thule-worker
```

To post real MR comments and commit statuses, give the worker a GitLab token:

```bash
THULE_GITLAB_TOKEN=<token> \
THULE_GITLAB_PROJECT_PATH=group/your-gitops-repo \
THULE_GITLAB_BASE_URL=https://gitlab.example.com/api/v4 \
go run ./cmd/thule-worker
```

See [docs/gitlab-setup.md](docs/gitlab-setup.md) for webhook configuration, `/thule plan` comment command routing, and lock behavior.

## Configuring a repository

### Per-project: `thule.conf`

Drop a `thule.conf` at the root of each *project* (any directory tree that plans as one unit — typically one per cluster or per app). Changed files map to the nearest `thule.conf` above them.

```yaml
version: v1
project: payments            # name shown in the plan comment
clusterRef: prod-eu-1        # which cluster credentials to use
namespace: payments
render:
  mode: flux                 # yaml | kustomize | helm | flux
  path: manifests
  flux:
    includeKinds:
      - HelmRelease
      - Kustomization
diff:
  prune: false               # also report would-be deletions
  ignoreFields:
    - metadata.annotations
policy:
  profile: baseline          # baseline | strict
comment:
  maxResourceDetails: 100    # cap rendered detail per comment
```

`clusterRef` resolves through a credential catalog — see
[docs/cluster-access-examples.md](docs/cluster-access-examples.md) for GKE and bare-metal examples.

### Repo-wide: `.thule.yaml`

Optional, at the repository root. Where `thule.conf` configures one project, `.thule.yaml` configures MR-wide behavior: guards over the shape of the whole change set, and a follow-up comment posted after each plan.

```yaml
guards:
  # Fail when one MR modifies the same app under two or more groups of a
  # guarded tree laid out as <prefix>/<group>/<app>/... -- a "group" is
  # whatever failure domain the path encodes (region, site, shard).
  - name: prod-regions
    description: region-redundant apps must roll one region at a time
    type: same-app-across-groups
    prefix: clusters/prod
    exempt:
      - flux-system

followUp:
  # Posted as a standalone comment after each plan comment (never
  # superseded). Useful to trigger downstream bots once the plan exists.
  # Placeholders: {sha}, {summary}.
  comment: '/review --extra="Thule planned {summary} at {sha}"'
  # Posted instead of `comment` when the MR maps to no project plan (its
  # changed files are not rendered by any Thule project, e.g. alerting
  # rules). Without it such MRs get no follow-up at all. There is no plan to
  # reference, so keep it plain. Placeholders: {sha}, {summary} (always 0).
  commentNoPlan: '/review'
```

Guard results surface in three places: a banner on top of the plan comment, a `thule/guards` commit status (failed on violation, success once the MR is split; absent when no guarded tree is touched), and loud failure of the status when `.thule.yaml` itself is invalid. Make `thule/guards` a required check to enforce guards rather than just surface them.

## Deployment

Both binaries ship in one container image (see [Dockerfile](Dockerfile)). A typical production layout:

- `thule-api` Deployment behind an internal ingress, receiving GitLab webhooks
- `thule-worker` Deployment with: an SSH key for cloning the GitOps repo, a GitLab API token for comments/statuses, kubeconfigs (or workload identity) for the clusters it diffs against
- Redis as the queue between them (no persistence required; jobs are re-derivable from webhooks)

Keep the worker's cluster credentials **read-only** — Thule plans, it never applies.

## Architecture notes

- Roadmap: [docs/thule-architecture-roadmap.md](docs/thule-architecture-roadmap.md)
- Implementation phases: [docs/phase0-implementation.md](docs/phase0-implementation.md) … [docs/phase4-implementation.md](docs/phase4-implementation.md)
- CI: unit + integration workflows with a 90% unit-coverage gate ([docs/ci-testing.md](docs/ci-testing.md))

## Status

Functional and in production use against GitLab. In-memory adapters exist for every external dependency (queue, run store, comments, statuses, cluster reads), so the full planning pipeline is testable without infrastructure.
