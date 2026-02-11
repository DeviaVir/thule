# CI Testing and Coverage Gates

This repository uses two GitHub Actions workflows:

1. `unit-tests.yml`
   - Runs unit tests across `./internal/...` and `./pkg/...`.
   - Produces `unit.out` coverage profile.
   - Fails the build if total unit coverage is below **90%**.

2. `integration-tests.yml`
   - Runs integration tests under `./integration/...`.

## Local verification

```bash
go test ./internal/... ./pkg/... -covermode=atomic -coverprofile=unit.out
./scripts/check_coverage.sh 90 unit.out
go test ./integration/... -v
```
