# E05 project contracts and immutable releases

This design implements LEA-34–LEA-40 from the current Linear E05 epic (LEA-33). It builds on the reviewed E04 drafts. Merge, release and formal milestone acceptance remain separate from implementation delivery.

## Ownership and delivery

- Document Core owns the versioned `leafrun.json` format, technical validation, target/profile resolution, JSON Schema preparation, static defaults and isolated input snapshots. Clients and services call this same API.
- Projects owns revision-bound descriptors, immutable releases, publication state and release pins, in its own PostgreSQL database. It does not own working files or run Typst.
- Generation owns admission receipts, jobs, attempts and PDF publication. Its existing E04 execution pipeline remains the renderer for published documents.
- Contracts owns HTTP DTOs, OpenAPI and bounded clients. Infra owns the internal dev stand and cross-service verification.

Deliver linked draft PRs for Core, Contracts, Storage, Projects, Generation and Infra. Preserve E04 compatibility, including the existing explicit-entrypoint Job API. Storage needs a small resource-metadata endpoint: E04 exposes content only, while E05 must obtain authoritative file scope/class/hash/size before admission. Platform remains unchanged.

## Manifest and descriptors (LEA-34, LEA-35)

Use the documented format version 1 and example: `formatVersion`, `name`, `engine.typstVersion`, `templates`, `targets`, `profiles`, `defaultProfile`. Unknown fields, duplicate JSON keys, malformed Unicode, traversal/native paths and unsupported format versions are rejected with file and JSON Pointer diagnostics. Credentials, ACLs and arbitrary connection configuration are not accepted manifest fields.

Template IDs are unique in the template namespace; target IDs are unique in the target namespace. Document targets resolve exactly one template and schema. An omitted target schema inherits its template schema; an explicit target schema must use that same logical path, otherwise validation reports a schema conflict. Every referenced entrypoint, schema, dependency and output configuration is technically checked against the immutable project filesystem. Outputs are logical result paths and cannot overwrite source resources. The supported engine is pinned Typst 0.15.1.

Validate bundle target steps, ID uniqueness, references, order and acyclic dependencies. Return their descriptors, but actual Batch/DAG execution belongs to E07. E05's runnable published target is a PDF document; requesting bundle execution returns a clear unsupported-target error before admission.

Projects records an immutable descriptor set under `(workspaceId, projectId, revision)` with its Storage snapshot ID and Core content ID as separate fields. Re-registering the same revision with changed bytes conflicts. Multiple templates/targets remain independent. The base descriptor digest includes the target, schema contract, all frozen profile definitions/default choice and captured resource versions. A separate execution digest includes the selected resolved profile. Releases freeze the base descriptor/profile set, so every allowed profile remains selectable without changing release bytes; requests never consult newer working definitions. No native path or live source lookup participates in resolution.

## Shared parameters and files (LEA-36, LEA-37)

Core uses Draft 2020-12 schemas, the existing strict JSON numeric-lexeme representation and a closed snapshot-only reference loader. No HTTP or host file schema loads occur. Schema resources are bounded to 4 MiB in total; request and prepared JSON to 1 MiB; nesting is bounded. Field failures carry JSON Pointers and stable codes. Values are never coerced; user formulas and computed defaults remain Typst code.

Static defaults are deep copied only into absent properties. Explicit null/false/zero/empty values are preserved. Existing nested objects and array items receive their own defaults; missing objects are materialized only by an explicit object default, and arrays are not extended by tuple-item defaults. Unconditional local refs and `allOf` defaults are supported; conflicting defaults or defaults beneath ambiguous conditional/union branches fail technical validation. Validate default literals against their subschemas. Bound recursive expansion so recursive schemas cannot create unbounded defaults. The same Core preparation routine and fixtures define client/server parity.

File input schemas use the Leafrun extension `x-leafrun-file: true` on string fields, defined by this versioned contract. The public request uses `fileRefs` keyed by RFC 6901 field pointer, each with a Storage `resourceId` and safe `filename`. A file field is populated only through a bound fileRef; user-provided arbitrary paths and default file paths are rejected. Resolve refs under the caller's current permissions, require the selected project scope and resource class, and freeze resource size and SHA-256 before admission. Storage's new metadata read enforces current access and lifecycle in its own database; callers never supply authoritative hashes or read Storage SQL. No URLs or native paths are accepted. Limit refs to 64 and total unique input-file bytes to 25 MiB.

Prepared JSON contains stable logical paths under reserved `.leafrun-input/`, derived from content hash and a safe filename extension. Reject a source tree that already owns this reserved root. Core assembles a new independent snapshot containing exact prepared `input.json` and verified input files; the source remains unchanged. Captured external fonts, font order, provenance and content ID rules remain M02 compatible. Parallel preparations own independent copies. Set string `sys.inputs["leafrun-data"] = "/input.json"`; Typst loads typed values with `json(...)`.

## Profiles (LEA-38)

Draft, Production and custom names are Leafrun profiles. Resolve the selected or default profile before admission; never emulate a Typst mode flag. A profile holds bounded string `inputs` for template options and explicit PDF export options. `leafrun-data` is reserved to Core. Resolve the exact engine version, export options, captured font/resource/dependency digests and profile name into an immutable descriptor. E05 supports only CLI options verified against pinned Typst 0.15.1; unknown options fail validation.

Extend Core's snapshot rendering options to pass sorted `--input key=value` arguments and supported PDF-standard export options without shell interpretation. Existing callers retain identical behavior when options are omitted.

## Release lifecycle (LEA-39, LEA-40)

Creating a release fixes `releaseId`, project scope, targetId, version, revision, snapshotId, contentId and descriptor digest. Technical validation includes schemas, manifest, DAG and captured resource integrity, plus current rights. It never invokes Typst or requires a successful preview. Unique target/version and idempotency constraints prevent duplicate or mutable releases.

Persist the release candidate and exact acquisition intent before calling Storage. The Storage holder is the fixed Projects service principal, with retention policy `release`. Protocol: acquire pending pin; persist pending release reference; activate; read receipt after lost response; mark the release usable only after confirmed activation. An owner callback distinguishes referenced, absent and unknown. Bind every pin field to scope, object and holder. Storage/DB time is authoritative; expiry rotation requires an authoritative expiry response. A deleting snapshot or late activation cannot yield a usable release. Reconciliation resumes all incomplete steps after crashes without publishing an unpinned release.

Content is immutable; publication state is mutable and versioned separately. Publishing and unpublishing require current permissions and expected publication version. Catalog and form APIs expose only currently authorized releases; the form returns the exact schema, supported UI annotations, defaults policy, profile choices and an API example. Earlier releases can be selected and republished without changing their bytes. Unpublishing denies new generation admissions. Historical successful jobs retain their own status and original PDF expiry.

An unpublished release remains available for explicit rollback and retains its release reference. An explicit delete of an unpublished release tombstones it and durably schedules release-pin cleanup; after its final valid reference is removed the snapshot becomes collectible (D16). Deleting the working revision/copy cannot delete a release pin. Tombstones, resolved operations and outbox history have finite retention; unresolved pin work is retained until reconciled.

## Generation integration

Add a published/working render-selection API accepting scope plus either `releaseId` or `snapshotId + targetId`, `parameters`, `fileRefs`, profile and an idempotency key. Resolve through the Projects owner API using a fixed service credential and an authenticated caller identity; current access is checked independently. Published generation permission need not grant source-read. Working generation still requires source-read. An absent, withdrawn or unauthorized release fails closed.

Keep the original selection fingerprint separate from the fully resolved execution fingerprint. `generation_admission_receipts.fingerprint` stores the original normalized selection hash; `generation_jobs.fingerprint` and `normalized_request` continue to bind the fully resolved execution, renderer and artifact. Add bounded `selection_request`/`selection_fingerprint` fields and validate both forms in the admission transaction. Legacy explicit-entrypoint admissions keep their existing equality path. Same caller/key with changed selection conflicts; an authorized replay returns the recorded job without calling current revision resolution. It creates no new execution; withdrawal checks apply to fresh admission and subsequent result publication.

The internal Projects resolver accepts only a fixed authenticated Generation service identity. The asserted user principal is constructed from Generation's server-side token resolver, never copied from public JSON. Bind the response to this principal, exact scope and normalized selection; reject server-owned descriptors/resource digests in both public request variants. Projects resolves/authorizes refs and calls Core preparation once; Generation persists those exact canonical prepared bytes. The child verifies frozen descriptors/bytes through Core and assembles the isolated snapshot; it does not apply a second defaults implementation.

Extend the bounded renderer wire protocol for frozen input-file descriptors and streams. The privileged controller performs current resource access and Storage reads; the isolated child gets only verified bytes and resolved options, with no credentials or network. Verify the already prepared canonical bytes/descriptors through Core, then assemble the execution snapshot. Existing controller fencing, stop confirmation, finalize adoption and 24-hour artifact TTL continue to apply. Recheck current release publication and rights before new admission and before publishing a new result; dependency uncertainty blocks publication.

Generation's admission transaction also persists a scoped allowlist of input-resource identities: field pointer, resource ID, exact scope/class, safe filename/generated path, size/SHA-256, and the corresponding pin ID/acquisition key/state. Generalize the existing pin-intent validation to these exact rows; activate input-resource pins first and activate the source pin/record the workflow-start intent last. A crash before the final step leaves a recoverable pending admission and cannot start a renderer with unprotected inputs. Generation's owner callback validates every pin receipt binding (scope, object, holder, policy, pin ID and acquisition state) against its durable rows; terminal cleanup releases the input pins as well as snapshot pins.

## Verification and boundaries

Unit and contract checks cover malformed manifests and field paths, all local-reference and DAG cases, multiple independent targets, nested defaults and numeric lexemes, invalid defaults, recursive expansion, file authorization and digest/path attacks, concurrent preparations and profile option resolution. Core's real Typst tests prove `sys.inputs` and supplied fileRefs render with the captured fonts.

Projects PostgreSQL checks cover revision immutability, target/version conflicts, idempotent release creation, pin/commit/activate crash windows, lost replies, authoritative expiry, deletion races, publication CAS and cleanup. Service checks cover current rights, generation-only vs source-read access, dependency failures and withdrawal between resolution and publication.

The real Compose stand includes Projects API/reconciler and separate Projects DB credentials. Host ports remain loopback and fixtures require an explicit dev profile. End-to-end checks prove a release can publish without a render, then render through releaseId, survive working-copy cleanup, deny new jobs after withdrawal, republish an older release without byte changes, and become collectible after explicit final reference removal. Include restarts and pin GC races. Run unit/contract tests, `go vet`, builds and Linux race checks for affected repositories, plus inherited E04 regression scenarios.

E06 supplies real identity/Access owners and E08 supplies web forms. E05 publishes the exact form contract and usable API, with explicit dev Access fixtures. E07 owns batch execution and E15 owns PDF cache reuse; neither is silently implemented by E05.
