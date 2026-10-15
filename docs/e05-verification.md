# E05 verification record

E05 implements LEA-34–LEA-40 across Core, Contracts, Storage, Projects,
Generation and Infra. Platform and the M02 integration adapters retain their
reviewed E04 revisions. The exact published dependency commits belong in
`ci/dependencies.json`; the hosted evidence records every source commit and
tree, including Infra itself.

| Linear scope | Behavior and verification |
| --- | --- |
| LEA-34 | Closed version-1 leafrun.json; duplicate/unknown/version/path/schema/resource errors; technical validation without Typst |
| LEA-35 | Immutable revision and target descriptors; document and bundle descriptions; selected execution digest independent of base digest |
| LEA-36 | Draft 2020-12 validation, bounded missing-only defaults, local refs/allOf and JSON Pointer diagnostics with exact numeric tokens |
| LEA-37 | Authorized same-scope file metadata, 64-reference/25-MiB budgets, pinned file streaming and independent prepared snapshots |
| LEA-38 | Frozen draft/production/custom profiles, sorted template string inputs and validated PDF export configuration |
| LEA-39 | Durable release acquire/activate protocol, lost-response recovery, fenced reconciliation, exact owner receipts and cleanup |
| LEA-40 | Publication CAS, current catalog, versioned form/API, releaseId generation, withdrawal, original replay and old release republishing |

## Reviews and local checks

Implementation units receive independent specification and quality reviews.
Core, Contracts and Projects units passed both reviews and real native tests,
vet/build and Linux race checks. PostgreSQL-backed tests use private schemas and
separate owned test databases. Storage metadata tests use actual PostgreSQL and
versioned S3. Generation passed native and Linux race/vet/build after independent review fixes
for typed pin expiry, credential separation, attempt-bound input reads, terminal
receipt replay and exact uncertain source/derived pin ownership. Infra passed
independent specification and quality review, full native tests/vet/build, and
the explicit no-admission/no-Storage-pin regression.

The real E05 HTTP stand checks:

- Publishing a technically valid project whose Typst source would fail,
  without invoking the renderer.
- A generation-only caller receiving a versioned form and PDF while source
  snapshots, revision details and source resource bytes remain inaccessible.
- Shared missing-only defaults, exact `1.0` JSON, a nested template reading an
  uploaded SVG through a root-relative generated path, a frozen custom profile
  and captured external fonts after deleting the original font directory.
- Unsupported bundle, invalid schema, missing file and hidden foreign file
  rejection before any durable Generation admission; an authorized working
  selection then renders successfully.
- Withdrawal denying new admissions, original selection replay returning the
  saved job, changed inputs conflicting, old release republishing preserving
  its descriptor/source identity and the original 24-hour PDF expiry.
- D16 using actual Temporal GC: deleting a working revision retains the
  release-pinned snapshot; deleting the final unpublished release eventually
  makes that exact snapshot collectible.

`restart-check.ps1` records only public IDs and deterministic fixture inputs,
restarts PostgreSQL/S3/Temporal/NATS and all owners, then checks the same release,
prepared content, admission receipt, PDF hash and original expiry. It preserves
the inherited E03/E04 receipts and independent event projection.

`broker-outage-check.ps1` stops only the captured local NATS container. Both
Generation success and Projects publication/withdrawal remain available while
their events stay unsent in each owner's DB. After restarting that same broker,
the checks verify the exact existing event IDs and payload hashes, acknowledged
outbox rows and unchanged job/release/artifact state. Projects uses a separate
producer ACL; it cannot publish another domain, subscribe to events or manage
streams. Lost publish-ACK deduplication is also tested through the Platform relay
against a real Projects database.

Final local qualification passed full native Infra tests/vet/build, E03/E04/E05
persistent restart, independent PID 1 controller-crash recovery, complete
two-domain NATS outage/recovery, and Linux race/vet/build in all seven repositories.

Hosted Core also caught a Windows CRLF fixture-mutation helper issue; the
same three failures were reproduced locally before repairing the test helper.
The full native suite passed with a CRLF fixture. Production project parsing
remains unchanged and direct fixture reads still exercise native line endings.

The first full hosted run passed ten checks, then exposed a one-shot Linux
supervisor test observation immediately after SIGKILL. The assertion now polls
all visible members of the process group for a bounded second and still rejects
any live member. Linux-only helpers are build-tagged; Windows compilation and
repeated Linux race tests passed. Production termination behavior is unchanged.

## Reproducible complete qualification

```powershell
./scripts/ci-check.ps1 -DockerContext desktop-linux -TypstBinary C:/tools/typst-x86_64-unknown-linux-musl/typst
```

The workflow repeats startup, full host tests/vet/build, restart, controller
crash, broker outage and all seven repositories' Linux race/vet/build checks.
Inherited D03/D04/D05/D10/D15/D17/D18 and `soa/pin-gc-race` remain enabled.
Use `-VerifyPins` only with clean exact dependency checkouts. Hosted runners
retain a bounded sanitized `e05-evidence` artifact; credentials and generated
PDFs remain ignored.

This is draft delivery. Real Access belongs to E06, bundle execution to E07,
form UI to E08 and cache reuse to E15. Merge, release and formal Linear closure
follow prerequisite acceptance separately.
