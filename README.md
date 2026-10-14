# Leafrun Infra — E04/M04 internal stand

The stand runs Generation, Storage, PostgreSQL 17.6, SeaweedFS 4.48, Temporal 1.32.1 and NATS 2.15.1 with JetStream. Images are pinned by digest. Generation's API, workflow worker, activity controller, dispatcher, reconciler, janitor and event relay run as independent processes. Storage has its own API and worker; an independent development consumer projects Generation events into its own DB.

All host ports bind to `127.0.0.1`; the stand remains internal until E06. Services require the explicit `dev` profile. Only Access is a development fixture: publication and pin-owner checks route to the real Generation owner API. Missing credentials, publication evidence or authoritative dependency responses fail closed. Fixtures require `LEAFRUN_DEV_FIXTURE=true` and separate credentials.

## Source layout and startup

Use Go 1.27.0, Docker Compose with named build contexts and PowerShell 7. During draft review, scripts create an ignored workspace for unreleased modules. `ci/dependencies.json` records the exact published source revisions.

```text
workspace/
  core/          leafrun-document-core E04 branch, including M02/E03
  contracts/     leafrun-contracts E04 branch
  platform/      leafrun-platform E04 branch
  generation/    leafrun-generation E04 branch
  storage/       leafrun-storage E03 branch
  infra/         this repository
  integrations/  leafrun-integrations M02 commit a4414f5fe3f12815d305d14edba5396a97e5738b
```

Set `LEAFRUN_INTEGRATIONS_DIR` when Integrations is elsewhere; scripts also recognize it one directory above this layout. Docker commands default to the local `desktop-linux` context; `-DockerContext default` selects a hosted Linux runner's engine. Scripts do not change the global context. Remote engines are outside this loopback stand's supported setup.

```powershell
cd infra
./scripts/start.ps1
docker --context desktop-linux compose --profile dev --profile e03-regression ps
```

`init-env.ps1` generates ignored random credentials and preserves existing values. Storage, Generation, Temporal and the consumer own separate databases and roles; PUBLIC database CONNECT grants are revoked. Storage unit tests and E03 regression services have separate catalogs, buckets and Temporal namespaces. SeaweedFS uses private versioned buckets and distinct administrator/service/test identities. Startup probes exact-version reads/deletes and multipart support. Do not apply bucket lifecycle expiration to retained resources.

| Service | Host endpoint | Owned state |
| --- | --- | --- |
| PostgreSQL | `127.0.0.1:58434` | Storage, Generation, Temporal history/visibility, consumer and isolated test DBs |
| SeaweedFS S3 | `127.0.0.1:58343` | Private versioned main/test/regression buckets |
| Temporal | `127.0.0.1:57243` | Persistent history, separate Generation and E03 test namespaces |
| NATS / monitor | `127.0.0.1:54222` / `58222` | Persistent JetStream, separate publisher/consumer/bootstrap ACLs |
| Generation API | `127.0.0.1:58090` | Jobs, leases, intents, candidates, bindings and outbox |
| Storage API | `127.0.0.1:58091` | Storage catalog and versioned objects |
| Dev Access fixture | `127.0.0.1:58092` | Explicit mutable access/outage evidence |
| E03 regression API / fixture | `127.0.0.1:58094` / `58095` | Independent regression catalog and evidence |

`contexts.ps1` exports tracked source only. Git metadata, credentials, local caches and generated PDFs stay outside build contexts. The controller and janitor are trusted processes with a Docker socket and private staging volume. Every renderer child instead uses an immutable image ID, no network or host mounts, nonroot/read-only root, one CPU, 512 MiB RAM and 256 MiB `/work` tmpfs. PID 1 enforces a 60-second monotonic deadline independently of the controller. Typst 0.15.1 runs only inside the renderer or the Core integration harness.

## Verification

Put native Typst 0.15.1 on PATH for host tests. Supply a verified Linux executable for the race runner:

```powershell
. ./scripts/test-env.ps1
go test -count=1 -v ./...
go vet ./...
go build ./...
./scripts/restart-check.ps1
./scripts/controller-crash-check.ps1
./scripts/broker-outage-check.ps1
./scripts/linux-checks.ps1 -TypstBinary C:/tools/typst-x86_64-unknown-linux-musl/typst
```

`ci-check.ps1 -TypstBinary <Linux executable>` runs startup, complete host tests, restart verification, controller-crash verification, broker-outage recovery and race/vet/build in all six repositories. `-VerifyPins` requires clean checkouts at the exact published dependency commits. `.github/workflows/e03-verify.yml` executes this E04 sequence on Ubuntu, retaining E03 regression coverage. Official actions and Typst downloads are pinned. Only sanitized source hashes/check outcomes are uploaded from `.cache/ci-evidence.json`; credentials remain in ignored env files. Use Compose `config --quiet` for validation because expanded config contains local secrets.

The E04 HTTP harness checks durable admission, exact numeric parameter bytes, schema and compile failures, full captured-font transfer, independent derived snapshots, publication, download, idempotency, access revocation and dependency outage. A real independent inbox/effect consumer must observe terminal events. PostgreSQL/NATS tests exercise lost publish ACKs, redelivery, ordering, quarantine, explicit replay and permissions. Restart verification preserves the job receipt, PDF bytes, original expiry and consumer projection across DB/S3/Temporal/NATS and service restarts. The crash check kills the controller during CPU-bound Typst execution, verifies independent exit 124, then exercises janitor recovery without another render. These scenarios cover D03, D05 and D17.

E03 scenarios retain D04, D10, D15, D18 and `soa/pin-gc-race`: captured JSON/SVG, empty directories, removed original fonts, atomic manifests, finalize replay, access/publishability, hard TTL, read/write leases, actual multipart/version inventory, orphan races and lost S3/DB acknowledgements. Tests never share the live catalog's inventory bucket.

## Lifecycle and delivery

PDF expiry is fixed at 24 hours from the first Storage finalize. Pins, replays, ordinary references and latest-PDF pointers cannot extend it. Generation purges parameters and raw diagnostics 24 hours after terminal state, idempotency after seven days, and resolved job/event history after 30 days when dependencies allow. Pending outbox/cleanup and unresolved quarantine survive age-based retention. Source/derived snapshot bytes follow Storage reference/pin/GC rules. Storage's defaults remain 24-hour orphan grace, 30-day cleanup history and a configurable five-minute Temporal sweep (`STORAGE_SWEEP_INTERVAL` in ignored `.env`). Release snapshots do not inherit PDF expiry.

The fixture's independently authenticated `PUT /dev/v1/state` controls access denial and dependency outage for the demo scope/principal. Real Access, collaboration and callbacks remain later stages. Temporal's internal frontend is unauthenticated only on this explicitly internal development stand.

Stop while preserving state with `docker --context desktop-linux compose --profile dev --profile e03-regression down`. Removing named volumes destroys development catalog/history and requires a separate deliberate action. Never commit `.env`, `.cache`, credentials or PDFs. E03/E04 draft delivery depends on E01/E02 acceptance; merges, releases and formal Linear closure remain separate. No project license has been selected yet.
