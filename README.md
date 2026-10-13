# Leafrun Infra — E03/M03 internal stand

The stand runs the Storage API and a separate Temporal worker with PostgreSQL 17.6, SeaweedFS 4.48, and Temporal 1.32.1. Images are pinned by digest. PostgreSQL and SeaweedFS use named persistent volumes; Temporal history and visibility have their own databases and login, independently of Storage migrations and credentials.

All published ports bind to `127.0.0.1`; the stand stays internal until E06. The Access/Generation/pin-owner fixture and Storage services require the explicit Compose `dev` profile. Storage has no permissive fallback. The fixture refuses startup unless `LEAFRUN_DEV_FIXTURE=true` and separate owner/admin credentials are supplied. Missing publication evidence denies download; unknown pin ownership protects retained data.

## Source layout and startup

Use Go 1.27.0, Docker Compose with named build contexts, PowerShell 7, and the four E03 branches. Core depends on the M02 integrations checkout. Libraries remain unreleased during draft review: scripts create an ignored external workspace instead of publishing local module replacements.

```text
workspace/
  core/          leafrun-document-core E03 branch including M02
  contracts/     leafrun-contracts E03 branch
  storage/       leafrun-storage E03 branch
  infra/         this repository
  integrations/  leafrun-integrations M02 commit a4414f5fe3f12815d305d14edba5396a97e5738b
```

Set `LEAFRUN_INTEGRATIONS_DIR` if Integrations is elsewhere. Scripts also recognize it one directory above this layout, matching the existing M02 checkout. Docker commands default to the existing local `desktop-linux` context; pass `-DockerContext` to choose another local engine context. Host-side bootstrap and tests use that engine's local loopback ports, so remote Docker hosts are outside this stand's supported setup. Scripts never change the global context.

```powershell
cd infra
./scripts/start.ps1
docker --context desktop-linux compose --profile dev ps
```

`init-env.ps1` generates ignored random credentials and preserves existing values. It writes private SeaweedFS identities under `.cache`. Bootstrap uses its own S3 administrator; Storage receives read/write/list/tag permissions only on `leafrun-storage`. Tests have a separate role limited to `leafrun-storage-test`, so their catalog inventory cannot treat stand objects as orphans. Both buckets are versioned before the API starts. Startup probes exact-version operations and multipart support; a provider without required capabilities is rejected. Anonymous S3 access has no identity grant. Do not apply bucket lifecycle expiry to prefixes containing retained resources.

`contexts.ps1` copies only tracked source into ignored build contexts; Git metadata, credentials, generated PDFs and local caches stay outside images. Dockerfiles create temporary workspaces. Nonroot runtime images contain the service executable and base certificates.

| Service | Host endpoint | Internal database/storage |
| --- | --- | --- |
| PostgreSQL | `127.0.0.1:58432` | `storage`, `storage_test`, `temporal`, `temporal_visibility` |
| SeaweedFS S3 | `127.0.0.1:58333` | separate private versioned `leafrun-storage` / `leafrun-storage-test` |
| Temporal | `127.0.0.1:57233` | PostgreSQL history/visibility; namespace `default`, 30-day history retention |
| Storage API | `127.0.0.1:58081` | Storage-only catalog |
| Dev owner fixture | `127.0.0.1:58082` | explicit mutable test evidence |

PUBLIC database CONNECT grants are revoked. Storage cannot connect to Temporal databases, and Temporal cannot connect to Storage databases. `storage_test` and its separate S3 bucket/role are dedicated to destructive aging/race scenarios. `TestS3CatalogIsolation` proves both roles cannot read or inventory the other's versions and anonymous object requests are denied. The internal Temporal server explicitly permits unauthenticated frontend traffic on this development stand; production authentication is later infrastructure work.

## Verification

Typst 0.15.1 runs through Core and the harness. Storage does not invoke Typst. Put the native executable on PATH for host checks:

```powershell
. ./scripts/test-env.ps1
go test -count=1 -v ./...
go vet ./...
go build ./...
go -C ../storage test -count=1 ./...
./scripts/restart-check.ps1
./scripts/linux-checks.ps1 -TypstBinary C:/tools/typst-x86_64-unknown-linux-musl/typst
```

The Linux runner uses the race detector with cgo, Git, OpenSSH and the supplied Linux Typst binary. It verifies all four repositories, executes real PostgreSQL/S3 scenarios and executes/replays a persisted Temporal workflow. Linux build/module caches use separate named volumes. Credentials pass through ignored env files without being printed. Validate Compose using `config --quiet`; expanded config contains local secrets.

The HTTP harness captures Typst, JSON, SVG, empty directories and external Fira Sans; deletes the original font folder; exports project and ordered font trees; stores resources and an atomic manifest; saves a pending owner reference before pin activation; restores an independent bundle with the same content ID; and renders a PDF. It verifies finalize replay, denial for unpublished jobs/current access/dependency outage, successful exact-byte download, and preservation of the old PDF after a failed next render. These exercise D04, D10, D15 and D18 through public interfaces.

Storage tests cover `soa/pin-gc-race`, acquire/delete orderings, pending expiry, owner uncertainty, hard result TTL under active pins, bounded readers/writers, actual multipart/version inventory, lost S3 acknowledgements and a dropped successful PostgreSQL COMMIT acknowledgement. `restart-check.ps1` restarts PostgreSQL, SeaweedFS, Temporal and Storage without removing volumes, then verifies scoped snapshot/pin records, original PDF expiry and exact S3 bytes. Ignored restart receipts contain IDs and hashes only.

## Fixture controls and lifecycle

Owner checks use `/internal/v1/checks/access`, `/publishable` and `/pin-owner` with Contracts DTOs. The fixture admits the configured demo scope and `demo`/`core` principal. Its independently authenticated `PUT /dev/v1/state` supplies exact artifact/job publication bindings, per-pin `referenced`/`absent`/`unknown` evidence, denial, or dependency outage. Missing pin evidence starts as unknown. This API belongs exclusively to the explicitly enabled internal stand.

PDF lifetime is fixed at 24 hours from first finalize; pins, replays, ordinary references and latest-PDF pointers cannot extend it. Defaults are 24-hour orphan grace, 30-day cleanup history, and a configurable five-minute Temporal sweep. Release snapshots do not inherit PDF expiry. Real Access, jobs, collaboration and callbacks remain later stages.

Set `STORAGE_SWEEP_INTERVAL` in ignored `.env` to configure the worker's `LEAFRUN_STORAGE_SWEEP_INTERVAL`; startup validates it and updates the same persistent Temporal schedule after a restart.

Stop while preserving data with `docker --context desktop-linux compose --profile dev down`. Removing named volumes destroys the development catalog/history and requires a separate deliberate action. Never commit `.env`, `.cache`, credentials or PDFs. E03 remains draft delivery dependent on E01/E02 acceptance; merge, release and formal Linear closure are separate.

No project license has been selected yet.
