# Phase 0 Implementation Notes

This repository now includes the **Phase 0 foundations** outlined in the roadmap.

## What was implemented

- Go module scaffold and command entrypoints:
  - `cmd/thule-api`
  - `cmd/thule-worker`
- Initial package layout under `internal/` and `pkg/` for:
  - webhook intake,
  - orchestration,
  - queue,
  - idempotency storage,
  - config typing/validation.
- `thule` configuration schema (JSON Schema):
  - canonical file at `schemas/thule.schema.json`,
  - embedded copy at `internal/config/thule.schema.json`.
- Webhook-to-queue path proof:
  - `/webhook` accepts MR event payloads,
  - orchestrator validates and deduplicates by `delivery_id`,
  - jobs are enqueued for worker processing.
- Base org/repo defaults stub in `.thule/config.yaml`.

## Current scope caveats

- Queue/storage are in-memory placeholders for Phase 0 only.
- Worker and API are intentionally skeleton services.
- Rendering/diffing/cluster integrations are out of scope for Phase 0 and are targeted in later phases.

## How to validate

Run:

```bash
go test ./...
go run ./cmd/thule-api
```

Example webhook payload:

```json
{
  "delivery_id": "evt-001",
  "event_type": "merge_request.updated",
  "repository": "org/repo",
  "merge_request_id": 123,
  "head_sha": "deadbeef"
}
```

Then POST it to `http://localhost:8080/webhook`.
