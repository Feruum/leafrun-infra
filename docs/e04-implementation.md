# E04 / M04 implementation record

Approved by the user on 2026-10-14. Delivery is five related draft PRs;
merging, releases and formal completion remain separate acceptance steps.

## Scope and sequencing

1. Core: independent `CreateInputSnapshot` preserving exact parameter bytes,
   captured fonts, provenance and the M02 identity algorithm.
2. Contracts: Generation HTTP/OpenAPI/SDK, versioned JSON normalization and
   bounded framed renderer transport. Storage v1 remains compatible.
3. Platform: transactional outbox/inbox, ACK ordering, durable application
   attempts, quarantine/replay and aggregate version handling.
4. Generation: PostgreSQL admission/reservations/intents, Temporal orchestration,
   isolated renderer and janitor, derivation/finalize recovery, publication epochs,
   access checks, cancellation, events and retention.
5. Infra: independent processes and databases, persisted JetStream, owner routing,
   pinned source/image CI and recovery tests including E03 regressions.

Each implementation unit receives specification review followed by quality review
using Luna 6 max. Workers own distinct repositories and preserve others' edits.

## Required invariants

- Required workspace/initiator idempotency keys; project and complete normalized
  request are fingerprinted. Replays recheck current credentials and access.
- HTTP admission returns 202 only after durable reservation, activated snapshot
  pin and durable workflow-start intent. Expired pending pins use a new acquire
  intent/key while preserving job ID and public receipt.
- JSON sorts object keys, preserves arrays and numeric lexemes, rejects duplicate
  keys, invalid Unicode, depth >64, non-int64 integer tokens and nonfinite floats.
  Exact normalized parameter bytes are saved before external effects.
- Draft 2020-12 schema validation uses jsonschema/v6 v6.0.2 and json.Number,
  no coercion/defaults, annotation-only format, and a closed immutable-base
  snapshot resolver. Schema errors are asynchronous validation failures.
- Logical `input.json` replaces a file only; base snapshots remain immutable.
  In-project font subtrees including `.` are recomputed; external fonts remain
  captured, ordered and independent of native source paths.
- A stable `render/{jobId}` workflow cannot restart after completion. API,
  workflow, controller/activity and janitor are separate processes. Durable
  dispatch/reconciliation recover lost starts and enforce the admission deadline.
- Renderer has no network, credentials, Docker socket or host bind mounts.
  Nonroot/read-only root; PID 1 enforces monotonic timeout independently of the
  controller and kills the process group. Unknown Docker state retains slots.
- Attach before start; logging disabled. Bounded stdin/stdout frames carry the
  bundle, parameters, derived bundle, PDF and terminal envelope. Host staging is
  private. No upload/finalize before verified full terminal output and confirmed
  exit 0. Partial streams are discarded. Core temps/PDF stay under /work tmpfs.
- Persist exact derivation/upload/finalize requests before calls. Recover through
  GetUpload and identical finalize replay, never prepublication GetArtifact.
  Verify scope/snapshot/target/fingerprint/render attempt/hash/expiry.
- Publication takeover increments a separate epoch and can adopt an already
  finalized old attempt without rerendering. Terminal commit atomically checks
  epoch, lease, state and cancellation, then writes success/binding/outbox.
- Publishable means an exact committed scope/job/artifact success. Pin owner
  verifies full bindings with the fixed Generation service principal. Dependency
  uncertainty fails closed; authoritative absence differs from unknown.
- Access is checked before deferred execution, publication and reading.
  Source-read gates parameters and raw diagnostics. Events contain IDs/versions
  only. Compile/validation/resource-limit/cancel are nonretryable.
- Local state and outbox share a transaction. Publish ACK precedes sent marking;
  consumer effect/inbox commit precedes ACK. Five durable application attempts
  lead to durable quarantine before ACK, resilient to crash/redelivery. Reordered
  versions are ignored, gaps use the owner API. Replay is explicit.

## Limits and retention

Go 1.27.0; Typst 0.15.1. Attempt: 1 CPU, 512 MiB memory, 256 MiB tmpfs,
60 seconds. Lease/heartbeat 30/10 seconds; three attempts; deadline 15 minutes
from first durable admission. Slots global/workspace 2/1; queues 50/10; overflow
429. DB time and transactional locks govern quotas and transitions.

HTTP 1 MiB; each source/derived snapshot 100 MiB; PDF 25 MiB; schema resources
4 MiB total; bundle metadata 12 MiB compatible with Storage's 16 MiB request;
input/output streams 128/160 MiB; envelope 1 MiB.

Ordinary snapshot pins use durable release intents. PDF expiry is 24 hours from
first Storage finalize and cannot be extended. An expired receipt cannot become
a new success; committed success remains historical. Generation parameters and
diagnostics expire 24 hours after terminal state; derived input.json follows
Storage reference/pin/GC retention. Idempotency lasts seven days. Completed job
and resolved event history lasts 30 days only without unresolved dependencies.
Pending outbox/cleanup and unresolved quarantine never expire solely by age.

## Validation and delivery

Test concurrency/idempotency, lost pin/start/finalize/commit ACKs, stale epochs,
cancellation versus success, finalized adoption without rerender, expiry,
canonical numeric/Unicode fixtures, schema refs/$id/host-network canaries,
font/base immutability, controller crash and independent timeout, unknown Docker,
bounded/partial frames/tmpfs, real PG/S3/Temporal/NATS restart/outage,
duplicate/reordered events, quarantine/replay and aging guards.

Run unit/contract/integration, vet, builds and Linux race detector; preserve E03.
Cover D03/D05/D17 and corresponding SOA/diagnostics cases. Stand host ports are
loopback. LEA-26–32 are delivered; LEA-36/37 record only the implemented subset.
Full leafrun.json/releases/profiles/DAG/Batch/callback/cache remain later scope.

## Progress

- [x] Clean isolated E04 branches from verified E03 source trees.
- [x] Core implementation and two-stage review. Native complete tests/vet/build
  passed with Typst 0.15.1. Fixed review finding: retain trusted capture roots and
  reject temporary storage inside original project/fonts or base snapshot storage.
- [x] Contracts implementation and two-stage review. Unit/vet/build passed;
  strict JSON/field decoding, exact numeric lexemes, OpenAPI parity, binary
  framing and manifest/descriptor bounds covered. Linux race/vet/build passed.
- [x] Platform implementation and two-stage review. Unit/real PostgreSQL/vet/build
  passed on fresh private test schemas. Durable replay after quarantine ACK,
  immutable BYTEA envelopes, five-attempt ledger and retention guards covered.
  Live NATS and Linux race/vet/build qualification passed in Infra verification.
- [x] Generation implementation, specification/quality review and final
  integration review. Full native tests/vet/build and real job publication,
  access, schema/compile failure and restart checks passed. Hosted CI passed on
  Linux/Windows/macOS; fixed a Windows workspace-path issue. Pending-pin recovery
  now handles Storage's authoritative 409 `pin_expired` and expired metadata.
- [x] Infra integration, full recovery verification and two-stage review.
  Actual controller SIGKILL leaves PID 1 to stop the renderer with exit 124 at
  60 seconds; janitor records one stopped resource-limit attempt and removes
  the exact container. Full PG/S3/Temporal/NATS/service restart preserves
  job/artifact/PDF/expiry/projection receipts. With NATS stopped, a job still
  succeeds and its PDF downloads while the outbox remains pending; broker
  recovery delivers the event without rerender. Real service ACL/anonymous
  denial checks and all six repositories' Linux race/vet/build passed.
- [ ] Five linked draft PRs, pinned CI and Linear delivery metadata.
