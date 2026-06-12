# Thule

Thule is an Atlantis-inspired **read-only MR planner for Kubernetes GitOps repositories**.

It watches Merge Request changes, renders desired Kubernetes resources, diffs against cluster state, and publishes a plan comment. It **never applies** resources.

## Current capabilities


- MR webhook ingestion and deduplicated queueing.
- Atlantis-style project locking: changed project folders are locked per MR to prevent conflicting parallel plans.
- Changed-file project discovery with per-project `thule.conf`.
- Rendering modes: `yaml`, `kustomize` (path-based), `helm` (rendered YAML input), `flux` (kind-aware filtering).
- Diffing with create/patch/delete/no-op actions, ignore paths, prune control, risk tags.
- Policy findings integrated into plan comments.
- Run/status plumbing for reliability (run lifecycle, stale SHA checks, artifacts, status checks).
- CI with unit/integration tests and 90% unit coverage gate.

## Quick start

### 1) Prerequisites

- Go 1.22+

### 2) Run tests

```bash
go test ./...
```

### 3) Local plan preview (Phase 4 local parity)

```bash
go run ./cmd/thule plan --project ./apps/payments --sha local
```

This reads `./apps/payments/thule.conf`, renders manifests, runs diff/policy, and prints the same style plan comment body.

### 4) Run API

```bash
THULE_API_ADDR=:8080 THULE_WEBHOOK_SECRET=supersecret go run ./cmd/thule-api
```

### 5) Run worker

```bash
THULE_REPO_ROOT=$(pwd) go run ./cmd/thule-worker
```

To publish real GitLab MR comments/statuses from the worker, also set:

```bash
THULE_GITLAB_TOKEN=<token> \
THULE_GITLAB_PROJECT_PATH=infrastructure/devops/kubernetes \
THULE_GITLAB_BASE_URL=https://gl.blockstream.io/api/v4 \
go run ./cmd/thule-worker
```

## Configuration (`thule.conf`)

```yaml
version: v1
project: payments
clusterRef: prod-eu-1
namespace: payments
render:
  mode: flux # yaml|kustomize|helm|flux
  path: manifests
  flux:
    includeKinds:
      - HelmRelease
      - Kustomization
diff:
  prune: false
  ignoreFields:
    - metadata.annotations
policy:
  profile: strict
comment:
  maxResourceDetails: 100
```

## Repository configuration (`.thule.yaml`)

Optional, at the repository root. Where `thule.conf` configures one project,
`.thule.yaml` configures MR-wide behavior: guards over the shape of the
whole change set, and a follow-up comment posted after each plan.

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
```

Guard results surface in three places: a banner on top of the plan comment,
a `thule/guards` commit status (failed on violation, success once the MR is
split; absent when no guarded tree is touched), and loud failure of the
status when `.thule.yaml` itself is invalid. Make `thule/guards` a required
check to enforce guards rather than just surface them.

## GitLab integration

See [docs/gitlab-setup.md](docs/gitlab-setup.md) for webhook event examples, `/thule plan` comment command routing, and lock behavior notes.

## Cluster credential examples

See [docs/cluster-access-examples.md](docs/cluster-access-examples.md) for `thule.conf` examples that target GKE and bare-metal clusters, plus an example external cluster credential catalog keyed by `clusterRef`.

## Architecture and implementation phases

- Architecture plan: [docs/thule-architecture-roadmap.md](docs/thule-architecture-roadmap.md)
- Phase notes:
  - [docs/phase0-implementation.md](docs/phase0-implementation.md)
  - [docs/phase1-implementation.md](docs/phase1-implementation.md)
  - [docs/phase2-implementation.md](docs/phase2-implementation.md)
  - [docs/phase3-implementation.md](docs/phase3-implementation.md)
  - [docs/phase4-implementation.md](docs/phase4-implementation.md)

## Status

This repository currently provides a functional prototype across phases 0-4 architecture milestones, with in-memory adapters for queue, run store, comments, status, and cluster reading.
