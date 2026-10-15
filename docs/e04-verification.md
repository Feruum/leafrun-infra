# E04 verification record

Local qualification on 2026-10-15 used Go 1.27.0, Typst 0.15.1 (9dfd3a08),
Windows amd64 and Docker Desktop's Linux engine. The internal stand used actual
PostgreSQL, SeaweedFS, Temporal and persisted NATS JetStream. Dependency revisions
are recorded in `ci/dependencies.json`; publication verifies each reviewed local
tree against the corresponding remote tree.

## Delivery map

| Scope | Implementation and checks |
| --- | --- |
| LEA-26 | Contracts Generation v1/OpenAPI/SDK; HTTP snapshot/target/entrypoint/parameters admission and safe status/cancel |
| LEA-27 | Owned job DB, complete normalized-payload idempotency, activated source pins and durable start intents; quota/concurrency/replay regressions |
| LEA-28 | Separate API/workflow/controller; stable Temporal ID; durable start dispatcher and bounded deadline |
| LEA-29 / D17 | Fixed isolated renderer, independent PID 1 timeout, fencing, cancellation, technical retry budget and janitor cleanup |
| LEA-30 / D03 | Exact upload/finalize receipts, no-local-staging recovery, candidate adoption, separate publication epochs and terminal/outbox transaction |
| LEA-31 / D05 | Platform outbox/inbox, ACK ordering, duplicates/reorder/gap repair, five-attempt quarantine and explicit replay; real NATS outage/ACL checks |
| LEA-32 | Lost ACK/response and stale-owner DB regressions; complete stand restart, controller crash and broker outage recovery |
| LEA-36/37 subset | Exact normalized JSON, closed Draft 2020-12 schema resolver and independent captured `input.json` snapshots |

Related draft PRs: [Core #4](https://github.com/leafrun/leafrun-document-core/pull/4),
[Contracts #2](https://github.com/leafrun/leafrun-contracts/pull/2),
[Platform #1](https://github.com/leafrun/leafrun-platform/pull/1),
[Generation #1](https://github.com/leafrun/leafrun-generation/pull/1).
Storage remains the unchanged [E03 draft](https://github.com/leafrun/leafrun-storage/pull/1).

## Evidence

- Complete native Core, Contracts, Platform and Generation tests/vet/build passed;
  server suites used private PostgreSQL test schemas.
- Infra host tests/vet/build passed with actual HTTP, Temporal, Docker, S3 and
  NATS. A captured project retained JSON, SVG, empty directories and external
  fonts after the original font folder was removed. Derived `input.json`
  preserved integer/float/exponent tokens and the base snapshot stayed unchanged.
- Schema and compile failures were asynchronous terminal errors. Access denial,
  unknown dependency, idempotency conflict and fixed 24-hour PDF expiry checks
  passed. A terminal event reached an independent consumer inbox/effect DB.
- `restart-check.ps1` preserved E03 and E04 snapshot/job/artifact/expiry/PDF hash
  receipts and the consumer projection across PG/S3/Temporal/NATS and service
  restarts without removing persistent volumes.
- `controller-crash-check.ps1` found the exact isolated Typst child, stopped the
  janitor and SIGKILLed only the controller. PID 1 independently exited 124 at
  60 seconds. Janitor then recorded one stopped, confirmed `resource_limit`
  attempt, no PDF and removal of the exact renderer container. Both services
  were restored. Docker's classic tmpfs representation was checked via
  `HostConfig.Tmpfs` even when its `Mounts` list was empty.
- `broker-outage-check.ps1` stopped only the captured NATS container. Admission
  and PDF success/download remained valid while one terminal outbox row stayed
  pending. Restarting that same broker delivered the existing event to the
  independent consumer; job/artifact/hash/expiry and one finished attempt stayed
  unchanged. Finally restoration returned NATS to healthy state.
- Real NATS tests covered lost publish ACK, duplicate inbox effects, durable
  quarantine/replay and service ACLs. Publishers cannot publish foreign domains
  or subscribe to event subjects; consumers cannot publish events or administer
  streams; anonymous connections are rejected. Admin denial probes use a
  nonexistent stream so they cannot delete live delivery state.
- `linux-checks.ps1` passed `go test -race ./... -count=1`, `go vet ./...` and
  `go build -buildvcs=false ./...` in Core, Contracts, Storage, Generation,
  Platform and Infra. E03 regressions remained green.
- Pinned actionlint and PowerShell syntax/Compose validation passed. The
  Generation hosted matrix passed Linux/Windows/macOS after using relative
  workspace paths. Core's containment test now resolves native temporary-path
  aliases before comparing physical locations, preserving the actual storage
  contract on Windows and macOS.

The hosted Infra workflow runs the complete sequence again from clean, exact
published revisions with `-VerifyPins`; its sanitized artifact records source
commits/trees and each check outcome. Its final run URL is attached to the PR.

## Repeated review qualification, 2026-10-15

The final dependency pins include the repeated E04 review fixes. Storage is
authoritative for pending/active pin expiry, including skewed API clocks and
definite activation rejection. Publication replays completed operation receipts
before rotating an expired writer; its three-rotation budget persists across
controller restarts. Successful stopped attempts schedule guarded staging cleanup.

Pin cleanup now uses the exact scope/object/holder/retention-policy receipt from
Storage. Authorization precedes lookup, so a hidden 404 cannot falsely acknowledge
cleanup and an already purged reference can safely return a bound absent receipt.
Generation verifies the actual terminal producer payload, scope and receipt.
Independent specification and quality reviews passed; complete native Contracts,
Storage and Generation tests, vet and builds passed with real PostgreSQL/S3/Temporal.
The pinned hosted workflow repeats the inherited stand and Linux race checks.

## Acceptance boundary

This is draft delivery. E01/E02 acceptance, E03 integration, merges, releases and
formal Linear completion remain separate. E05 defaults/client parity, fileRefs,
profiles/releases/DAG/Batch/callback/cache are not claimed by the parameter subset.
