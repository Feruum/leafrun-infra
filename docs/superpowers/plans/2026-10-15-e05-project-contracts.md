# E05 Project Contracts Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task by task. Use one implementation subagent at a time and independent spec then quality review. The user selected Luna 6 Max subagents and authorized continuing without approval checkpoints.

**Goal:** A technically valid immutable published template generates a PDF through releaseId with the shared project/parameter/file/profile contract.

**Architecture:** Core resolves immutable project contents. Projects stores revision descriptors, releases and Storage pin intents in its own PostgreSQL database. Generation persists resolved execution inputs separately from selection idempotency and executes its existing isolated pipeline.

**Tech Stack:** Go 1.27.0, Typst 0.15.1, Draft 2020-12 JSON Schema, PostgreSQL, SeaweedFS S3, Temporal, JetStream, HTTP/OpenAPI, PowerShell Compose tooling.

Design: `../specs/2026-10-15-e05-project-contracts-design.md`. Requirement copies and full Linear issues are in the task parent's `requirements/` directory. Source checkouts are siblings of Infra under `.cache/leafrun-work/m05/`.

## Task 0: Close repeated E04 review findings

- [x] Reproduce API clock-skew pin expiry; remove process-clock expiry authority, preserve exact binding checks and ask Storage to confirm activation even for active metadata. Test source and derived pins, authoritative expiry and lost ACK.
- [x] Reproduce expired writer upload recovery for PDF and derived resources. Replay saved operations first; only typed `409 writer_expired` permits immutable upload-generation rotation. Preserve successful receipts and publication fencing.
- [x] Complete cleanup through an authorized exact-binding released/absent receipt, including purged pin metadata. Every HTTP error remains pending. Verify the real terminal producer payload and policy. Remove committed staging with guarded cleanup and durable recovery.
- [x] Run Generation tests/vet/build, PostgreSQL regressions and Linux race checks. Independent spec and quality reviews passed. Six linked E04 drafts and exact dependency pins are qualified by all eight checks in [CI run 38657849632](https://github.com/Feruum/leafrun-infra/actions/runs/38657849632); downloaded evidence matches every final clean ref. E05 Generation branches from the final reviewed implementation.

## Task 1: Core manifest and target descriptors (LEA-34, LEA-35 foundation)

Files: Core `project.go`, `project_validate.go`, `project_target.go`, `project_test.go`, `testdata/project-contract/`, `docs/project-contract.md`.

- [x] Add failing fixture tests using the documented three-template manifest. Cover unknown/duplicate fields, unsupported version, duplicate IDs, missing files/schema/template/target, target-schema inheritance and conflict with its template, logical-path escape, bundle order/step references/cycles and credentials/ACL fields.
- [x] Implement typed version-1 structures and field diagnostics. Public API:

```go
func ReadProject(ctx context.Context, files fs.FS) (ProjectManifest, error)
func DescribeTarget(ctx context.Context, files fs.FS, manifest ProjectManifest,
    snapshot SnapshotManifest, targetID, profile string) (TargetDescriptor, error)
```

`TargetDescriptor` fixes target kind, template/entrypoint/schema, all profile definitions and default profile, selected resolved profile, ordered bundle steps, snapshot content ID and digest. The base digest freezes the entire allowed profile set; a separate execution digest binds the selected profile. Storage snapshot ID/revision remain Projects fields. Descriptor slices/maps returned to callers are independent copies.

- [x] Validate required regular resources and closed schemas structurally without compiling Typst. Output paths cannot overwrite project resources. Bundle descriptors are supported; execution stays E07. Shared default-annotation checks are added by Task 2.
- [x] Run Core test/vet/build with real pinned Typst; spec and quality reviews passed and commit a63f799 records this unit. Review fixes bind the supplied manifest to captured leafrun.json, remove the unintended entrypoint-size cap, and index template lookup.

## Task 2: Core parameter defaults, fileRefs and profiles (LEA-36–LEA-38)

Files: Core `project_parameters.go`, `project_schema.go`, `project_files.go`, `project_profile.go`, corresponding `_test.go`, `snapshot_input.go`, `snapshot_input_test.go`, `render_snapshot.go`, `paths.go`, `typst.go`, `go.mod`, `go.sum`.

- [x] Add failing tests for absent-vs-null defaults, nested objects/arrays/local refs/allOf, invalid/conflicting/conditional defaults, recursive expansion, exact number lexemes, schema budgets and pointer diagnostics.
- [x] Implement shared preparation with the strict Contracts JSON codec and pinned schema library. Core API:

```go
func PrepareParameters(ctx context.Context, files fs.FS, schemaPath string,
    raw []byte, refs []FileRef) (PreparedInput, error)
func CreatePreparedSnapshot(ctx context.Context, base *Snapshot,
    prepared PreparedInput, open InputFileOpener,
    options CreateInputSnapshotOptions) (*Snapshot, error)
```

`FileRef` freezes JSON Pointer, resource ID, safe filename, size and SHA-256; `PreparedInput` owns normalized JSON, generated file descriptors and sys.inputs. `InputFileOpener` supplies only caller-authorized frozen bytes. Core validates all descriptor bindings, count/byte budgets, reserved root, exact digest/size and snapshot limits.

- [x] Add invalid resolver metadata, escaped-pointer, wrong hash/size, collision, duplicate-content and concurrent snapshot tests. Exercise exact bounds: 64 refs, 25 MiB unique file bytes, 4 MiB schemas, 1 MiB request/prepared JSON. Reject direct user paths and defaults on file fields without a bound ref. Source/project/external fonts remain unchanged; derived snapshots remain usable after source closure. Adapter cross-scope checks belong to Task 5.
- [x] Add real Typst tests using `json(sys.inputs.at("leafrun-data"))` and an input SVG. Verify typed nested values, nested template paths and independent parallel results. Run with the captured external font after deleting its original tree.
- [x] Resolve draft/production/custom profiles into bounded template string inputs and validated export options. Reserve `leafrun-data`, freeze resource/font/dependency digests, pass sorted CLI `--input` and verified `--pdf-standard` values. No artificial Typst mode flags.
- [x] Run Core test/vet/build and Linux race; independent spec and quality reviews passed. Commit aec2051 includes incremental default byte budgets, memoized shared-schema traversal, ancestor file-default rejection, field diagnostics and instance-aware file pointer checks.

## Task 3: Shared Projects/render-selection contracts

Files: Contracts `projects/v1/{types,validate,client,openapi}.go`, tests and OpenAPI document; `generation/v1/{types,request,client,openapi}.go`; `renderwire/v1/{messages,codec,bundle}.go`, tests and protocol documentation.

- [x] Add failing contract tests for revision registration, target resolution, release create/read/publish/unpublish/delete, catalog/form and internal resolve/current-state checks. DTOs carry scope, exact revision/snapshot/content IDs and immutable descriptor digest separately.
- [x] Define `RenderSelectionRequest` with either releaseId or snapshotId+targetId, parameters, fileRefs, profile and optional cachePolicy. Normalize without coercion, bound identifiers and pointers, preserve exact parameter numeric bytes and fingerprint the entire selection.
- [x] Define the server-owned `ResolvedExecution` DTO and its separate canonical fingerprint for Task 6 persistence. Public selection decoding rejects unknown fields and caller-supplied resource digests; legacy request decoding remains strict. Task 6 adds these checks to both HTTP routes.
- [x] Extend renderer frames for frozen input-file metadata and bounded content. Plan file order deterministically; enforce count/size/SHA-256, duplicates, terminal/EOF order and the total source+input budget. No credentials enter wire frames.
- [x] Add bounded clients with fixed credentials, deadlines, strict decoding and no redirects. Error DTOs expose field pointers without private dependency text. Add schema/DTO parity tests.
- [x] Run Contracts test/vet/build and Linux race/vet/build; independent spec and quality reviews passed. Commit 2db8a87 records the contracts, including strict expected response status and named OpenAPI error types.

## Task 4: Projects service, revisions and release pins (LEA-35, LEA-39)

Prerequisite from Task 3: implement Storage `GET /internal/v1/resources/{id}` and the Contracts `GetResource` client/OpenAPI operation. Files: Storage `internal/storage/{http,reads}.go` and metadata integration tests; Contracts `storage/v1/operations.go` and the Storage OpenAPI source. Reuse Storage's own transaction clock, ready/class checks and `resource.read` authorization. Test private scope, unauthorized/missing/deleting resources, exact hash/size and successful metadata with real PostgreSQL/S3. Review the small dependency change separately and include its own linked draft PR.

Files: Projects `go.mod`, `go.sum`, `cmd/leafrun-projects/main.go`, `internal/{config,store,service,storage}/`, `migrations/001_projects.sql`, tests, README and service documentation.

- [x] Create service roles `migrate`, `api`, `reconcile`; configure separate DB and fixed Access/Storage/service-owner credentials. Missing configured dependencies fail startup; readiness checks own DB/schema, and runtime downstream uncertainty fails closed per operation.
- [x] Add PostgreSQL tests before implementation: revision binding/immutability, multiple independent target descriptors, target lookup errors, version/idempotency conflicts and transactional release intent/outbox creation.
- [x] Implement snapshot retrieval with scoped, verified Storage resources and shared Core technical validation. Never run Typst. Protect revision metadata with current permissions; Task 5 adds schema/form delivery.
- [x] Implement durable pending release and pin acquisition/activation intent state. Persist before every external effect, validate exact receipt bindings, recover lost replies, rotate only authoritative expired pending pins, and fence concurrent reconcilers. Owner callback returns referenced/absent/unknown.
- [x] Test failures between acquire/persist/activate/final commit, pin expiry, activation/delete race, uncertain owner and process restart. Only confirmed active pin permits a usable release. Deleting a working revision leaves its release reference intact.
- [x] Add durable pin-release cleanup after tombstoning an unpublished release, finite metadata retention and an idempotent transactional outbox. Unresolved work stays until reconciled. Task 7 wires the existing Platform relay.
- [x] Run real PostgreSQL service tests, unit/vet/build and Linux race/vet/build; independent spec and quality reviews passed. Commit 3a49ee1 includes the reviewed tombstone registration fix and the tracked service entrypoint.

## Task 5: Publication, catalog and form contract (LEA-40)

Files: Projects `internal/service/{publication,catalog,forms,resolve}.go`, `internal/store/publication.go`, relevant tests and examples.

- [x] Test and implement publication CAS independently from immutable release content. Publish technically valid sources even if Typst would fail; no preview invocation/counter changes.
- [x] Implement current authorized catalog, exact versioned form/schema/default policy/profile choices/API example, withdrawal, explicit old release selection and republishing. Withdrawal denies new resolve/admission; delete requires unpublished state.
- [x] Internal resolve validates the fixed Generation credential and caller principal asserted by Generation's server-side authentication (never public JSON), current rights, release state, exact selector/descriptor and accessible file resources. Bind response to the asserted actor and selector. Call shared Core preparation once and return normalized frozen execution inputs. Fail closed on Access/Storage uncertainty. Reject bundle execution with typed unsupported_target before any Generation state or pins are created.
- [x] Test generation permission without source-read, foreign project fileRef rejection and schema field errors. Review spec then quality, run Projects test/vet/build/race and commit. Commits a37eec0 and 2a5dea5 include publication/resolution and the reviewed transactional event relay.

## Task 6: Generation render-selection integration

Files: Generation `internal/service/{render_selection,projects_client}.go`, `internal/store/{admission,execution_input,types}.go`, additive migration, `internal/controller/{types,runtime,publication}.go`, `internal/renderer/run.go`, tests and runtime configuration.

- [x] Add red tests for release/working selection, forbidden source bypass, input mutation on same key, withdrawal/dependency outage, original selection replay after unrelated revisions and server-owned fields on both public routes. Bundle selection must leave job/reservation/pin/workflow counts unchanged. Republished old release profiles must resolve from the frozen profile set.
- [x] Implement the new selection route using current user access and Projects owner resolution. Add bounded selection_request/selection_fingerprint fields; keep generation_jobs.fingerprint/normalized_request as resolved execution authority and store original selection hash in generation_admission_receipts.fingerprint. Validate both canonical forms in the admission transaction; legacy fields retain equal fingerprints. Replays return saved jobs without Projects/current revision resolution.
- [x] Add per-job input-resource rows in the admission transaction: pointer/resource ID, exact scope/class, safe filename/generated path, size/SHA-256 and pin ID/acquisition key/state. Generalize pin-intent validation only to that immutable allowlist; Generation's owner callback checks all applicable pin receipt fields against those rows. Activate all input-resource pins before the source pin records workflow start; pending recovery resumes in that order. Test crash/lost ACK before each activation and assert no early workflow start. Release all input pins on terminal cleanup. Stream exact file bytes to the renderer with no child credentials. Recheck resource identity and rights on privileged reads.
- [x] Renderer verifies the exact canonical prepared JSON/metadata through shared Core and assembles the snapshot; it does not reapply defaults. Revalidate all file/descriptor/profile bindings and pass resolved sys.inputs/export configuration. Keep old E04 request behavior compatible.
- [x] Check current release publication and caller permissions immediately before result publication; uncertainty leaves output private. Preserve source-read diagnostic redaction, PDF 24-hour original expiry, no rerender on finalize adoption and fixed attempt budget.
- [x] Run Generation regression tests with PostgreSQL, real Typst renderer/contract tests, vet/build and Linux race. Independent spec then quality review and commit. Commit 7a585a0 includes typed pending-pin expiry, credential separation, attempt-bound input reads, terminal receipt replay and exact lost-acquire ownership fixes; final native and Linux race/vet/build passed.

## Task 7: Real internal stand and cross-service verification

Files: Infra `compose.yaml`, `postgres/init.sh`, `Dockerfile.projects`, `Dockerfile.generation`, `internal/ownerrouter/`, `scripts/{workspace,init-env,start,test-env,linux-checks,ci-check}.ps1`, `e05/`, CI workflows and verification docs.

- [x] Add Projects API/reconciler and separate PostgreSQL databases/roles and Storage holder credentials. Route Projects pin-owner checks to Projects and Generation checks to Generation, using strict configured credentials. Keep explicit dev profile and loopback ports.
- [ ] Export only tracked safe files; include pinned Projects source in build contexts and Linux workspace verification. Keep secrets and generated PDFs outside tracked content.
- [x] Add real E2E publication without render, releaseId PDF, shared defaults/files/profile, denied foreign files, unpublish/new-admission denial, old release republish, process/DB/Temporal restarts and original artifact expiry checks.
- [x] Exercise D16: remove working reference, run Storage reconciliation and verify published release snapshot survives; tombstone final unpublished release reference, reconcile release cleanup/GC and verify collectibility. Exercise acquire/delete races and lost activation response with real Storage.
- [x] Run inherited E03/E04 scenarios plus E05, all affected unit/contract tests, vet/build and Linux race. Record exact tested source trees and evidence, fix and re-review all actual findings.

## Task 8: Linked draft delivery and Linear evidence

- [x] Verify clean reviewed commits and all local checks. Create/update writable forks via stored GitHub authorization; never overwrite a remote branch changed outside this task.
- [x] Publish linked Core/Contracts/Storage/Projects/Generation/Infra draft PRs with exact local-tree/remote-tree equality. Attach every created PR to this chat. Pin cross-repository hosted CI to exact published commits.
- [ ] Verify hosted CI at the final refs, download and validate bounded evidence. Update Linear LEA-33–LEA-40 with accurate implemented scope, review fixes, PR links and qualification results; leave formal Done/merge/release for separately accepted prerequisites.
- [ ] Report concrete finished work and any actual remaining limits, with links. Do not mark a task complete from implementation alone if required integration or review remains.
