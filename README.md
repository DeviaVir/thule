# Thule

Thule is an Atlantis-inspired **read-only MR planner for Kubernetes GitOps repositories**.

It watches Merge Request changes, renders desired Kubernetes resources, diffs against cluster state, and publishes a plan comment. It **never applies** resources.

## Current capabilities

- MR webhook ingestion and deduplicated queueing.
- Atlantis-style project locking: changed project folders are locked per MR to prevent conflicting parallel plans.
- Changed-file project discovery with per-project `thule.yaml`.
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

This reads `./apps/payments/thule.yaml`, renders manifests, runs diff/policy, and prints the same style plan comment body.

### 4) Run API

```bash
THULE_API_ADDR=:8080 THULE_WEBHOOK_SECRET=supersecret go run ./cmd/thule-api
```

### 5) Run worker

```bash
THULE_REPO_ROOT=$(pwd) go run ./cmd/thule-worker
```

## Configuration (`thule.yaml`)

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

## GitLab integration

See [docs/gitlab-setup.md](docs/gitlab-setup.md) for webhook event examples, `/thule plan` comment command routing, and lock behavior notes.

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
