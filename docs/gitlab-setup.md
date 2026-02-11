# GitLab Setup Guide

This guide shows how to route GitLab webhook events to Thule.

## Endpoint

- Webhook URL: `https://<thule-host>/webhook`
- Method: `POST`
- Signature header (optional but recommended): `X-Thule-Signature: sha256=<hmac>`

## Events to enable in GitLab

1. **Merge request events**
   - Triggers automatic planning from MR updates.
2. **Note events**
   - Enables manual `/thule plan` comment command.

## Supported payload styles

Thule currently accepts:

- Native internal envelope:
  - `delivery_id`, `event_type`, `repository`, `merge_request_id`, `head_sha`, `changed_files`
- GitLab MR webhook (`object_kind: merge_request`)
- GitLab note webhook (`object_kind: note`) with command in `object_attributes.note`

## Manual command

In MR comments:

```text
/thule plan
```

Thule routes this into a `comment.plan` planning event.

## Example GitLab MR payload (minimal)

```json
{
  "object_kind": "merge_request",
  "event_id": "evt-1",
  "project": {"path_with_namespace": "group/repo"},
  "changed_files": ["apps/payments/deploy.yaml"],
  "object_attributes": {
    "iid": 42,
    "last_commit": {"id": "abcdef"}
  }
}
```

## Example GitLab note payload (manual plan)

```json
{
  "object_kind": "note",
  "event_id": "evt-2",
  "project": {"path_with_namespace": "group/repo"},
  "merge_request": {"iid": 42, "last_commit": "abcdef"},
  "object_attributes": {"note": "/thule plan"}
}
```

## Project lock behavior (Atlantis-style)

When a merge request changes files under a Thule project folder, Thule attempts to lock that project path for the MR.

- Lock key: `<repository>/<project-root>`
- Owner: MR IID
- Conflict behavior: a second MR touching the same project path is rejected until lock owner closes/merges and a close event releases locks.

This mirrors Atlantis-style project-level serialization and prevents conflicting concurrent plan pipelines for the same folder.

## Optional approval behavior

When approval integration is enabled, Thule can publish MR approval decisions:

- `approved`: plan succeeded and lock checks passed.
- `request_changes`: lock conflict or plan failure occurred.
