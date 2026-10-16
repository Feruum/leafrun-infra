# E06 verification evidence

This is an incremental record. E06 delivery is in progress; no milestone acceptance, merge or release is claimed.

## Solo delivery qualification — 2026-10-16

The user requested solo implementation and review. Subsequent manual specification and quality checks are by the root agent; they are not described as fresh independent subagent approvals.

- Authenticated API checks under Feruum found the original public Access/Audit repositories. Their organization Git endpoints remained unavailable, so verified writable forks were created. The connector account is ilarvne and is not the publication identity. All nine remote baseline trees match the recorded local baselines.
- Ten Go modules completed Linux race tests, vet and build: 1,559 passing test events, zero failures, 68 explicitly skipped tests. This is an iteration receipt, not an assertion that every external fixture ran. Platform PostgreSQL, legacy Storage/Temporal fixtures, browser-coordinated OAuth/guest fixtures, Windows-only tests and the exclusive Audit durable probe have separate opt-ins and evidence. E03/E04/E05 regression and real browser receipts remain separate.
- A real local Git commit was captured by Core, including external fonts; the original font directory was deleted, then the bundle restored with the same content ID/provenance. Public Projects registered and published it. A human and generate-only bot rendered separate parameter sets through Temporal to concrete S3 object versions; authenticated PDF hashes and fixed 24-hour expiry matched. Both domain API replicas read shared state, and all six rejected the exact logged-out session. Replica authorization uses Access B.
- The renderer SDK sandbox additionally passed its real Linux race test against the immutable E06 renderer image. Native CI for Access/Audit and the existing three-OS Generation/Core matrix report success on the exact published commits. Final Infra hosted qualification is still pending.
- PowerShell entry points now generate their workspace, require Access/Audit for E06, accept an explicit verified Linux Typst path, cover all ten modules, record test skips and working-file hashes, and reject source changes during a run. Pinned CI requires exact clean commit/tree inputs. Build exports record source/export byte hashes; runtime image IDs are recorded separately.

The first full verifier attempt stopped after Contracts because Git was invoked from a Windows-linked worktree inside Linux. The command now uses `/tmp` for Git configuration. Earlier MVP fixture reds (an incorrect forbidden-status expectation and a PowerShell 5 launch) are retained; Storage's intentional denied-artifact response is exactly 404 with no PDF bytes. No product access check was relaxed.

The initial hosted legacy regression passed, while hosted E06 stopped before tests at S3 bootstrap. A diagnostic follow-up established that Seaweed restarted with a permission error: its pinned entrypoint drops to uid1000 and cannot read the Linux host-owned private0600 bind file. A separate Linux-volume fixture reproduced that exact failure. The fix delivers a private uid1000 copy into a dedicated read-only runtime config volume through a network-isolated one-off initializer; the host file keeps its permissions. The same fixture then passed both authenticated HeadBucket calls, with config mode0600 and directory mode0700. Final hosted reruns remain pending.

Drafts published so far: [Contracts #4](https://github.com/leafrun/leafrun-contracts/pull/4), [Platform #2](https://github.com/leafrun/leafrun-platform/pull/2), [Access #1](https://github.com/leafrun/leafrun-access/pull/1), [Audit #1](https://github.com/leafrun/leafrun-audit/pull/1), [Storage #4](https://github.com/leafrun/leafrun-storage/pull/4), [Projects #2](https://github.com/leafrun/leafrun-projects/pull/2), [Generation #3](https://github.com/leafrun/leafrun-generation/pull/3). Infra follows with immutable remote dependency pins. No merge, release or formal Linear Done has been performed.

## Planning — 2026-10-15

Source: [LEA-41](https://linear.app/bulnik/issue/LEA-41/e06-polzovateli-organizacii-roli-i-boty), children LEA-42–LEA-49 and their linked architecture documents.

- Isolated m06 worktrees created from qualified E04/E05 commits; existing m05 stand remains running.
- Access and Audit source obtained through authenticated GitHub API after Git clone transport returned 404. Their initial repositories contain README only.
- Design and implementation plan committed: ad1df66, 12a8e92, 94df513.
- Independent spec review: GPT-6.1 Sol High, approved after four findings were corrected: hierarchical permission scopes/unique tenant registry, immutable token authorities across OAuth refresh, disjoint audit subjects and organization/project management boundaries.
- Follow-up owner exploration identified pending-pin identity loading, explicit migration/readiness registration and final checks after row-lock waits. These are included in Task 5.
- Task 1 Contracts/Platform foundation is complete after independent specification and quality reviews plus native and Linux race checks. Task 2 (LEA-42, Access/Kratos) follows.

## Baseline checks

Windows Go 1.27.0, m06/e06.work, before production changes:

| Repository | Starting commit | Check | Result |
| --- | --- | --- | --- |
| Contracts | a7e06b7017b7322d8b5a26fb4dfecbdf9cff5b53 | go test ./... | passed |
| Platform | d4c049b27928706efc27ea8e34d57ffa855e6455 | go test ./... | passed |

Database integration flags were not enabled for these baseline unit/contract runs.

An overlay test on the unchanged m05 Contracts package established the golden fingerprint for selectionFixture/resolvedFixture:
`003fcc2088e8df1b88c15bf6e2e60013be42147c59936870c382800bbd5f4fe6`.
The baseline worktree was not modified. The fixture's selection fingerprint is
`47440c507366c08b53ce8354ffe6ee35c650e9f0b9c03474449072f04e7dd24a`.

## Ory image preflight

Pulled and independently inspected Linux amd64 images on the local Docker engine:

| Image | Verified digest |
| --- | --- |
| oryd/kratos:v26.2.0 | sha256:2a13bb8d362c7a7ae33bd7c0f5168aee46921f15c916a06346db91c06dc76643 |
| oryd/keto:v26.2.0 | sha256:bfdb8b9e283aedbd9e1d005c44eb1bbefd2096b2cb5b5aa0b7360b6e39c178d2 |
| oryd/hydra:v26.2.0 | sha256:ff67c7fb5f95074fa53374d41151713554960504b340cd3f95b09e65deaea2a9 |

Isolated, network-disabled, automatically removed containers confirmed sh/wget and nonroot users for health checks. No provider database was started by this preflight. Planned E06 loopback ports were free.

## Task 1 — reviewed foundation

Reviewed clean commits:

- Contracts: 90358b854499f3650a46a01d78c1df6a1baba3bf.
- Platform: 64b6be6970d7660e50696433feb60bb6d503e103.

The implementation adds stable identity aliases, separate authority references, hierarchical Access scopes, strict HTTP/authentication clients, generated OpenAPI, v2 hierarchical event envelopes and a closed Audit action contract. E05 fingerprints and valid v1 event bytes remain unchanged.

Independent specification review found and verified fixes for two P2s: missing Unicode trim/control constraints in the OpenAPI ID schemas, and unintended acceptance of organizationId in v1 events. Re-review approved current sources, with executable version probes and 175 ECMA-262 cases across seven ID properties. Independent quality/security review approved with additional nested JSON, raw credential ambiguity, response-body deadline and audit sanitization probes.

Fresh native go test ./..., go vet ./..., go build ./... passed for Contracts and Platform. Generation, Projects and Storage consumer tests passed with native Typst 0.15.1. Linux Go 1.27 go test -race -count=1 ./... passed for both changed modules with the network disabled. Initial Linux setup attempts lacked older declared dependency versions in the separate Docker cache; the complete module graph was downloaded before the final network-disabled pass. No production source was changed to address cache setup.

Nine PostgreSQL tests remain skipped for this foundation qualification; no PostgreSQL/Ory integration is claimed. Detailed local receipts and logs are in m06/reviews/task01, including foundation-implementation-evidence.md, spec-review-fix-evidence.md, spec-probes/reviewer-fixed-approval.md, quality-review.md and linux-race-final.log.

## Task 2 — LEA-42 reviewed Access and Kratos sessions

Reviewed clean source commits:

- Access: 2121785d06c265683a36a550a51af7fccb417731.
- Contracts: 3bc4bd4d381047b733ab150a75960901332d8887.
- Platform: 1cbd6f2907df2c8be3a7524b756207612205b2c1.
- Infra runtime/provisioning: 780254d35c44d43eea6a1a21bde0870d965fa786.

Access now resolves real Kratos sessions into immutable nonsecret authority references, revalidates exact session/actor/expiry bindings, separates public and fixed service authentication, and provides a minimal CSRF-preserving browser login/logout and typed public session client. The owned leafrun-e06 stand has separate Access/Kratos databases and credentials, two API nodes, private admin endpoints and loopback host ports. Both API nodes use one canonical browser return origin on Access A.

Early independent root probes found CSP redirect blocking, ambient HTTP cookie-jar injection and the node-B return-origin mismatch. All were fixed with red/green tests and independently rechecked. Actual Chromium proved missing-CSRF denial, successful password/CSRF login through A and B, identical identity/ref across nodes, and retained-old-cookie401 on both after browser logout. Private temporary browser profiles were removed. Direct extraction/hash comparison of the two running executables gave the same SHA-256, 688597e67463fe0aa731747714882337a905699de1cbb845f77dd8f3ca11ff94; different Docker provenance indexes are separately recorded.

Independent specification review approved after fresh native tests/vet/build, independent provider-wire/body-timeout probes and Linux race with real PostgreSQL/Kratos: 50 top-level passes, zero integration skips. Real scenarios include migration/binding concurrency, restart persistence, revoked and inactive identities, a genuine five-second provider session expiry, actual browser CSRF failures/success, B-entry/B-logout and fixed-service credential denial. The owned outage/restart receipt proves liveness200, readiness/session503 during provider outage, restored200 and stable authority after restarts/migration rerun. Database provisioning proves distinct owners/credentials and cross-domain connection denial.

Independent quality/security review approved with 42 fresh targeted tests, zero skips, vet and adversarial browser-target probes. No material review findings remain. Native broad checks explicitly skipped real-provider/PG cases without integration settings and nine existing Platform event PG tests; the separate final real Access integration/race run executed its integration tests, so those native skips are not presented as integration passes.

Detailed ignored receipts: m06/reviews/task02/implementation-summary.md, spec-probes/spec-review.md, quality-review.md, browser-qa.md, node-b-browser-receipt.json, runtime-binary-receipt.json and 24-canonical-linux-race-real-integration.txt. This completes local Task 2 qualification; draft publication and formal milestone acceptance remain later delivery gates. Task 3 / LEA-43 follows.

## Task 3 — LEA-43 reviewed tenant hierarchy

Reviewed clean source commits: Access756e1d0fe0c99ca709f5edbed28adea4bdedeb53 and Contractsee26c1149710ae55c7b96fa9822bc13f5449a9ed. Platform1cbd6f2907df2c8be3a7524b756207612205b2c1 and Infra runtime780254d35c44d43eea6a1a21bde0870d965fa786 are unchanged; later Infra commits contain root planning/verification documentation.

Access now persists organizations, owner/member relationships, organizational workspaces, teams, personal/team projects and one-use hash-only invitations. Composite tenant keys and globally unique workspace/exact project-scope registry prevent cross-tenant binding. Public creation allocates IDs. Organization-row locking protects last-owner and personal-owner invariants, with a bounded final exact initiating-authority check after lock waits. Scoped list/detail APIs use bounded keyset cursors and strict public JSON/media/origin handling; typed hierarchy clients and generated OpenAPI cover the routes.

Early independent root probes found valid pages exceeding the shared authentication response cap and literal dot IDs being interpreted as URL path segments. Separate bounded hierarchy responses and exact segment encoding fixed both; identical independent probes passed. Worker self-review also fixed maximum-width cursor encoding with structured context hashing and strict canonical decoding. Red/green receipts remain under ignored m06/reviews/task03.

Final implementation qualification passed native tests/vet/build and OpenAPI byte-match; native counts are108 top-level passes and15 explicit top-level skips. Real PostgreSQL separately passed10 top-level tests with zero skips. Final Linux race/runtime passed70 top-level tests with zero skips, including actual Kratos-backed A/B hierarchy/invitations and an API observed waiting on an organization lock, followed by exact-session revocation and rollback. Root independently created hierarchy/membership records through the public APIs, restarted both owned API processes and read those same records and authority on both nodes; process start times changed, no volume was reset and the native probe session was logged out.

Fresh independent specification review approved with49 top-level passes, zero skips, five new PG/cursor probes, vet/build and one fresh Linux-race actual A/B lock/revocation pass. Fresh independent quality/security review approved with24 top-level passes, zero skips, including nine new adversarial probes and five real-PG tests. It covered invitation revoke/accept races, rollback across every mutation class, membership/read revocation, body bounds, provider binding, client redirects and deadlines. No material review finding remains. These counts are separate reviewed runs, not combined totals or claims that mocked transport replaces real Ory.

Evidence: m06/reviews/task03/implementation-summary.md,49-test-counts.json,48-final-linux-race-runtime.txt,root-early-findings.md,root-final-probes-green.txt,root-runtime-restart-receipt.json,spec-probes/spec-review.md andquality-review.md. Actual verified legacy-owner import remains explicitly retained for Task5; Keto/credential ceilings and other subsequent permissions are not claimed here. This completes local Task3 qualification and permits Task4/LEA-44; draft publication, merge and formal acceptance remain later gates.

## Task 4 — LEA-44 reviewed Keto roles and reconciliation

Reviewed clean source commits: Access `8bb43787b1a326b00ff1067efdadb3ab7b4c19d7`, Contracts `cd1cca2361157cabf854c5e1fed69efe336ac5d8`, Platform `a67b50f39d7b42145bc489bf0e77b6037b4c921a` and Infra runtime `ffc082d74072f9d17a26b4ce1883daf81335a5f7`. Subsequent root qualification documentation does not change runtime code. Storage/Generation/Projects domain integration remains Task 5.

Access now uses actual pinned Keto for four project roles and independently grantable generate/download/source/resource capabilities. Current credential ceilings apply to internal checks and public hierarchy/role operations. Immutable exact tenant bindings, complete epoch-qualified graphs, durable manifests, missing-only replay, desired/ready fences and exact resumable obsolete cleanup prevent stale positive decisions and obsolete publication. Provider readiness covers both read/write endpoints and registered policy tables. Early review corrections include current collection/target gates, migration 004 tenant-key immutability and the read/write readiness probe. Platform a67 is a one-line technical-health test fixture correction, with no production permission widening.

Fresh quality review reproduced a P1: a legal source mutation from 5000 to 5001 graph objects made the fixed first-ten scheduler repeatedly stop before healthy tenants. The correction adds checksummed migration 005 with a durable owned sequence/order and exactly one atomic epoch-row SKIP LOCKED claim before a fresh bounded 30-second attempt. Failure, advisory contention or cancellation yields future turns; epoch-row locks are skipped. The 250ms process ticker and full graph safety remain. Rejected intermediate designs rotated unattempted batches or repeatedly gave healthy work insufficient residual time; retained red probes distinguish those failures from intrinsically impossible deadlines. One tenant per pass lowers batch throughput. Over-capacity tenants remain pending503 and may need operator repair; source APIs do not reserve graph capacity. Fair progress assumes recurring passes and enough available capacity/budget for a healthy graph.

Final fixed-source native owned-PG checks passed 54 top-level/89 total tests, with 15 explicit provider skips; vet/build passed. Actual PostgreSQL/Keto/Kratos/A-B Linux race passed 69 top-level/107 total tests with zero skips/failures and covers all 15 native provider cases. Literal process kill after confirmed actual Keto insertion and fresh durable replay passed 1/1, with explicit actual-provider mode, final checks image and exit0. Separate real-process fairness/lifecycle proved 14 successful node-stage checks: healthy work behind 11 failed manifests, another healthy tenant after reconciler restart, failures remaining pending, all 11 disposable fixtures repaired, Keto outage/recovery and API restart. Test session logout and expiry-issuer shutdown completed; all owned services were restored without changing root fixtures or resetting volumes. Interrupted and Docker-unavailable runs are retained separately and never counted as passes.

Fresh independent specification re-review approved with 11 top-level/11 total actual-PG passes, zero skips, plus an expected prior-source regression failure. It independently verified concurrent/idempotent 4-to-5 upgrade, unchanged checksums 1–4, sequence ownership/readiness, one-claim metadata safety and 12 fresh subprocesses progressing behind 11 failures. Fresh independent quality/security re-review approved with 11/11 passes, zero skips, including the original oversized-source regression, seven scheduler cases, active-builder/new-epoch/cleanup concurrency, source churn and cancellation-before-claim. The identical quality regression also freshly failed against the prior scheduler as expected. These are separate runs, not combined unique-test totals.

Final Access 48 / Contracts 78 / Platform 21 tracked export files byte-match their source pins; live schema checksums 1–5 match committed SQL. API A/B/reconciler use image sha256:8a4904b5641d197b67f3faf5a2e883d2e31fac1f52da3bd6b8e16ca5bb8d0057; checks use sha256:cec7fe28fdc7796f42b89f8ade93f83237b0a0b9226b44cb4041733a34692f65. Older 249aa50/c0e0737 runtime evidence is historical and is superseded for this production fix. Unchanged Contracts/Platform checks remain retained qualification, including nine unrelated Platform event-PG skips; they are not claimed newly exercised. Pinned Keto can mask some failures as allowed:false, which still denies, and final checks do not create distributed atomicity or an arbitrary-late-provider-write guarantee.

Evidence: m06/reviews/task04/scheduler-fix-summary.md, scheduler-final-receipt.json, scheduler-literal-crash-command-receipt.json, scheduler-command-exit-receipt.json, scheduler-process-recovery-receipt.json, fix-spec-probes/report.md and fix-quality-probes/report.md. This completes local Task 4 qualification and permits Task 5; draft delivery and formal acceptance remain later gates.

## Task 5 — LEA-45 reviewed domain authority and separate permissions

Task 5 is locally qualified after corrected SPEC and ordinary QUALITY re-review. Its immutable reviewed source set is Access `a8c5cdd4ee580f4361ced74db1e2435bf42f7771`, Contracts `f2bb1247d786ea0038c5dd51b48d181878a9d4a6`, Platform `7da6189af12cfd92e7556826420e6e831ad58ffb`, Generation `061bc61ee34402e6e7e87b02c86fefd9161503db`, Projects `f74cf4416d91b312df86a79fb408dd33d800a77e`, Storage `ffc6ceb554e7e494a7cc8ec78f15ad48da93e28c` and Infra `82a1986b9fc600e87a36ddb6be014c15f5639bc7`. Core remains `3d2c5a9dd142dc05d4ba5c349bdd096a946b2a80`. Later bot development is separately reviewed; these are historical Task 5 pins, not a claim about mutable repository HEADs.

Owners enforce current Access decisions and preserve the initiating nonsecret authority separately from content identity and fingerprints. Published generate-only work privately consumes its captured project/fonts; working inputs retain source permission and selected file parameters require independent resource permission. Status/error/diagnostic visibility, current artifact download, fixed TTL and exact version/read lease remain separate. Deferred work cannot replace a revoked initiator with a fresh credential. Service cleanup remains available after revocation. Trusted legacy import verifies real Projects and Storage facts, serializes immutable organization assignment and receipt replay, and denies uncertain proof without a partial registry write. Generation003, Projects002 and Access006 migrations and physical readiness probes are registered and checked.

Independent QUALITY found two additional defects after initial SPEC approval. Projects release/revision deletion could wait for a row lock then commit without checking the current deleting caller; its final mutation gate now checks the exact locked binding after writes/outbox waits, including idempotent responses. Generation could finish container preparation with only a generate check; it now checks the full saved purpose immediately before Start and rechecks attempt/lease/cancel/deadline fences after that decision. Definite denial cleans the created container and records access_denied. Published private compilation still does not require public source access.

Corrected SPEC freshly passed 9 top-level/28 total existing actual-PG checks, zero skips/failures. Focused QUALITY freshly passed 5 top-level/18 total local checks, zero skips/failures, and independently matched 647 frozen plus 204 corrective source files. The original QUALITY findings/report and meaningful failing regressions remain preserved; corrected reports supersede them only for the exact source above. New negative deletion/start cases use actual isolated PostgreSQL with controlled decisions/fake engine. The separately executed real-provider suite uses Ory/Keto A/B, PostgreSQL, S3, Temporal, Typst and renderer: 3 top-level/7 total tests passed under Linux race with no skips/failures. It covers final-lock role/session revocation, owner import races, private inputs/fonts, restricted diagnostics, historical PDF access and Storage read revocation. Fourteen deployed HTTP denial checks passed; fake-human cases prove denial guards only.

Full corrected Projects Linux race/vet/build passed 48 top-level/93 total tests without named skips; Generation passed 169/237 with one explicitly unrun direct-live-Docker sandbox opt-in. Earlier Access actual-provider race passed 111/179 without skips. Seven-module original-dev race/vet/build and native/schema checks are recorded in the final qualification receipt. Six original lifecycle opt-ins were subsequently exercised: restart assertions6/6, controller-crash2/2 and broker-outage4/4. The original restart wrapper exited1 after its assertions because Windows reserved its old Temporal port; namespace repair exited0 and all APIs recovered. E04 now uses ignored loopback port58843 override for Temporal; its original host test-env still needs that override for further direct host Temporal checks. Core/Contracts first-phase nonverbose individual skip counts remain unknown. No skipped test is relabeled a pass.

The corrected runtime has both Projects roles on immutable image `sha256:1c4600fcbad62cb0e04f76e54794594c04b4370109be43cc092534ccb7642542` and six Generation roles on `sha256:761ed3e1172e2fa9b4869ec33dfba1e9fa4ab54013797bb038996a4079ead253`, with five ready APIs. Frozen dependencies and the existing renderer were retained during this bounded refresh. Storage's final two media headers and Infra's namespace --no-deps fix are test/helper-only over runtime b27328d/4cbeffc. Retained running Access/Storage executables were independently hash-matched to rebuilt binaries; this is distinct from deploying every new image tag. Remote decisions and local commits do not form a distributed atomic transaction.

Evidence under ignored m06/reviews/task05: final-qualification-receipt.json, handoff.md, fix-spec-probes/report.md, quality-probes-code/fixed-report.md, and quality-fix/{handoff.md,corrected-runtime-receipt.json,corrected-source-verification.json,corrected-running-readiness.json}. Task 6 source development overlapped a frozen Task 5 review following the user's speed instruction; its runtime gate cleared only after both corrected reviews approved. Draft publication and formal acceptance remain later gates.

### Task 5 export-byte addendum discovered during Task 6

The Task 6 Git-archive rebuild exposed a Windows export conversion mismatch for Generation migration003. Binary-safe inspection proves that the raw Git blob at061bc61 was already LF509 bytes with SHA2565df87e37b95d693a056b4c7a09d456a189a0f83b595657370a92d87ecf5c7647, matching the qualified runtime and live ledger. With core.autocrlf enabled, its Git archive instead emitted CRLF513 bytes with SHA256e3547f36c8dcd18b6b7f7b81708d1a5e0efdb0b5602cc9546cbcfa94fa0fe2ef. Earlier source/export receipts did not establish raw Git blob equivalence and must not be read as that proof.

Generation7e44f0f790a671fe3eeb661ec706deffa5ac5b94 adds one migration003-only text eol=lf attribute. All three SQL blobs and the historical001/002 archive bytes remain unchanged. Independent fresh binary Git/archive checks confirm the correction. The old archived embed failed the checksum regression; the corrected archive passed compiled embedded checksums, isolated PostgreSQL fresh migration/reapply/readiness, and read-only readiness against the existing E06 ledger:2passes, zero skips/failures. No ledger rewrite, checksum bypass or Go/DDL semantic change was performed. Final Task6 runtime qualification separately rebuilds and runs the coherent source set; preliminary mixed-runtime results are not final qualification.

Evidence: m06/reviews/task06/generation003-byte-repair/receipt.json and task05/quality-probes-code/migration-export-addendum/binary-check.json. Initial wrong-test-database probe and the original export mismatch remain retained. Raw Git bytes, platform-exported bytes, compiled bytes and live ledger checksums are explicitly distinguished.

## Task 6 — LEA-46 reviewed bot identities and credential versions

Task6 is locally qualified after full SPEC and fresh independent QUALITY approval. Exact final production/test pins are Access042e6df545a452edd096395014eb77dd2d0185e1, Contracts29730d95e6ad4fa177ec544fddee43133b8f8226, Infra571a7eb3bc3a0bd033f5a8c71b119b8cabb56b9e, Generation7e44f0f790a671fe3eeb661ec706deffa5ac5b94, Projectsf74cf4416d91b312df86a79fb408dd33d800a77e, Storage8212d8d25c80bb7a4bd6899e5dcbefc364132168 and Platform7da6189af12cfd92e7556826420e6e831ad58ffb, with unchanged Core3d2c5a9dd142dc05d4ba5c349bdd096a946b2a80 and Integrationsa4414f5fe3f12815d305d14edba5396a97e5738b. Later root Infra documents and Task7 source are separate from these immutable pins.

Bots have a server-derived stable principal and exact immutable home project, with no implicit role or key. Actual graph relations and a nonnil credential scope ceiling are independently required. Issue/rotate/revoke serialize receipts and expected versions with current gates after waits and before commit. Keys contain32 random bytes; only SHA256 and public metadata persist. Lost-disclosure retries return metadata without reissuing the secret. A new key cannot replace the original authority of saved work. Human-only bootstrap, domain-aware eligibility and separate service cleanup remain enforced. Migration007/readiness/upgrade/concurrent rerun are registered.

SPEC found database uncertainty being classified as denial in new bot lookups; Access042 preserves503 for SQL/pool/timeout failures while actual absence denies. The actual bot chain then exposed Storage current-check typed denial being reported503; Storage821 returns privacy404 for known denial/unauthenticated errors and retains503 for uncertainty. Generation's archive-only migration-byte correction is documented above. Original failing probes, intermediate mixed runtime, reviewer setup failures and helper errors remain retained and excluded from final passes.

Final coherent actual Linux race passed bot lifecycle/render1 top-level/1 total, Storage4/24 and durable-secret inspection1/1:6 top-level/26 total passes, zero skips/failures. It covers actual PostgreSQL/Ory A/B rotation/revocation without event delivery, real PDF/historical TTL, metadata receipts, original job authority, concurrent rotations, observed final-publication waits, no invalid candidate visibility and service pin cleanup. Only owned failed fixtures' diagnostic retention was made due. The corrected source PG/Keto race58/79 is separate retained evidence, not a unique combined total.

Fresh independent QUALITY passed native23/36 and actual PostgreSQL7/11, zero skips/failures. SPEC and QUALITY independently verified665 files across nine modules; QUALITY also matched all nine archive hashes. Root separately verified665 export hashes,14 running application image IDs, exact renderer binding and five ready APIs. Six image builds and historical Access/Generation/Projects ledger checks are recorded; volumes and the E04 stand were preserved.

The owned synthetic secret probe inspected100 durable payloads including11 Temporal events and verified the32-byte stored key hash. A bounded scan read14 application containers over that fixture's interval, capped at2000 lines each:311 bytes, one fixture-ID record, zero key candidates. Thirteen apps were quiet; the removed renderer logs were unavailable and are explicitly not counted. This is bounded fixture/source-flow evidence, not whole-database or unrelated-history secrecy. Native opt-in skips and unchanged prior-task limits remain disclosed.

Evidence under m06/reviews/task06: handoff.md, final-qualification-receipt.json, spec-probes/report.md, quality-probes/report.md, runtime-build-final/{source-manifest.json,images.json,runtime-receipt.json}, root-final-export-hashes.json, root-final-running-check.json and runtime-probes/no-secret-*.json. Task7 source overlapped immutable Task6 QUALITY under the user's acceleration instruction; its shared runtime gate cleared only after approval. Draft delivery, hosted CI and formal acceptance remain later gates.

## Task 7 — LEA-47 Hydra login, consent and immutable token authority

Full independent SPEC approved the final Access38679c57c18b0f2154d0479fe1b6804612572f07, Contracts470e88c9cfcb050ee5972162555c68a9fd2fd884 and Infraa364f295cb26d9a98d0d57280e04a023681ce8a4 source set. Other module pins are unchanged from Task6. Fresh independent QUALITY/security also approved this immutable source set after15 top-level/39 total fresh tests with zero skips/failures, including independent binding/final-gate/decrypt/revalidation and cryptographic probes. Task7 is locally complete; Task8 shared-runtime work is now cleared.

Actual pinned Hydra uses a fixed public code/S256 client, exact localhost callback, explicit scope/audience allowlist, opaque tokens and a separate private database/admin endpoint. Access login/consent requires the current exact Kratos human session, one-use persisted CSRF/browser/challenge bindings, strict parent/context/subject checks and typed410-only acceptance recovery. Each unique access token has its own immutable authority, original expiry and AES-GCM ciphertext with associated data; refresh never replaces the original token of saved work. Expired ciphertext and nonce are wiped.

Review and real browser/provider execution corrected native Kratos ory_st_ rejection, consent subject/error media/recovery checks, provider JSON case-fold ambiguity, form referrer/CSP redirect behavior, optional granted scope in the callback, and Hydra's compact32hex consent selector. Append-only migration009 preserves that selector exactly as bounded text for provider revocation;001–008 bytes and ledger checksums remain unchanged. Native fixture preconditions and false-versus-typed403 expectations were corrected separately from product defects. Failed attempts remain retained.

Access affected frozen actual-PG/Keto Linux race passed82 top-level/121 total, and actual native-provider integration passed12/15, each without skips/failures; focused native16/16, vet and build passed. Contracts native and frozen Linux race each passed99/157 with no skips/failures and generated OpenAPI byte equality. Infra affected internal race passed15/30; final SPEC independently repeated15/30 on the final compiled export, no skips/failures. These are separate suites, not combined unique-test totals.

All five actual Chromium phases passed: fresh native/OAuth login and consent, proper scope subset, decline, wrong verifier and missing verifier. Native document POST Origin/referrer, CSRF tamper, cross-browser state, callback replay and retained login/consent form replay were checked. Dual-live same-human session swap additionally used the real HTTP browser-form protocol and is not labeled Chromium. The actual owner lifecycle passed under Linux race1/1 in263 seconds: A/B scope/management denial, real PDF, separately authorized historical download and fixed TTL, Access/worker restarts, original queued-work denial after strict refresh, immutable successor authority, old-token revoke no-op, active-chain revoke/refresh reuse/independent-chain isolation, exact provider selector, short expiry and bound Kratos logout.

Root independently matched702 exported build files,14 running application images, renderer binding and six readiness endpoints after restarts. SPEC separately verified raw Git/export provenance (146 exact,556 line-ending-only, no other changes), all nine Access ledger checksums, migration009 repeated application and an expired short authority's NULL ciphertext/nonce without reading secret values. Named volumes, E04 and the root fixture remained intact. Current Access image is ddce6808…, demo2ca2262b… and owner-router8782c9bc…; complete identities and retained exact Docker input proofs are in the immutable receipts.

The private exact-token absence probe covered two owned jobs,94 related persistence rows, two terminal outbox rows and22 Temporal events (215096 bytes), with no held credential matches. Runtime logs were unavailable in this harness; no global or log-absence claim follows. Known injected-secret scans are separate bounded evidence. Normal immutable demo was restored and private browser sessions closed. Evidence: m06/reviews/task07/infra/{README.md,final-receipt.json,final-artifact-index.json}, runtime-build-operation, spec-probes/{final-spec-review.md,final-qualification-index.json,expired-ciphertext-receipt.json}, quality-probes/{final-quality-review.md,final-quality-receipt.json}, root-operation-export-hashes.json and root-operation-running-check.json. Draft PR/hosted CI and formal acceptance remain later gates.

## Source export and migration byte proof

Final source provenance distinguishes the exact Git commit/tree/raw blob hashes from the platform-exported files actually compiled. Every exported file must record its hash and compare against the raw blob; declared LF/CRLF conversion is recorded explicitly, binary files remain exact, and any other transformation blocks qualification. Reviewers also inspect the actual immutable build inputs. Historical SQL ledger checksums and new migration checksums are separate hard gates; source-line-ending normalization never authorizes rewriting a ledger or changing a previously qualified migration. Existing fingerprint/wire/runtime checks still apply to the actual exported inputs. Source equivalence is never inferred solely from an image tag or a normalized hash.

## Remaining acceptance

Each implementation task requires red/green behavior evidence, independent specification review, then quality review. Real PostgreSQL/Ory/S3/Temporal/NATS scenarios, two-node revocation, full Linux race checks, restarts and hosted CI remain required. Fixture regression passes alone will not qualify E06.


## Task 8 — LEA-48 scoped grants and browser guest sessions

FULL SPEC and independent QUALITY approved Access88b72f81b554a7a33741653f99583c8055924cca, Contractsb91e61b5fbf00c3ce2baba2a769b2feceeea325b and Infrae2e479eb3c413cbd32d5214eb5c560361798e9f8. QUALITY ran24 top-level/36 total fresh tests with zero skips/failures. Root independently verified733 compiled source hashes, exact application images/renderer and readiness both before and after the acceptance restarts.

Actual Chromium acceptance has18 passing assertions covering protected fragment exchange, clearing the fragment, bounded HttpOnly/Strict host-only guest cookies, all three owners, human-cookie preservation, explicit replacement/cleanup, dedicated logout, stale credential denial across Access A/B restarts, CSRF/session binding and ambiguous-cookie rejection. Owner race tests ran4 passes without skips/failures. PostgreSQL/Keto tests separately qualify grant issuer policy suspension/restoration, normal issuer credential changes, rotation/revocation and original-authority publication gates. These are separate suites and are not combined into a unique total.

The final scoped plaintext scan checked seven distinct privately held test values, including five exact guest-session hash matches,23 owned logs and Access metadata. No plaintext candidates were found in that bounded scope. An initial wrapper accidentally exposed a temporary owned grant fragment; it was immediately revoked and both nodes subsequently denied the old cookie and old link exchange. The original partial acceptance red file was overwritten during fixture repair; safe original503 diagnosis/tool output remains, but no intact original artifact/hash is claimed. Final executed driver/scanner hashes and green receipts remain retained. Both owned browser profiles were closed.

Evidence: ignored m06/reviews/task08/access, infra/{README.md,final-receipt.json,final-artifact-index.json}, spec-probes/{report.md,final-spec-receipt.json}, quality-probes/{report.md,final-quality-receipt.json}, root-acceptance-runtime-check.json and root-acceptance-after-restart-check.json. NATS was explicitly absent during Task8 qualification. Task9 source overlap began after SPEC8; shared runtime changes began only after QUALITY8. Finite inactive grant/receipt metadata cleanup is assigned to Task9. Local completion does not imply draft publication, hosted CI or formal Linear Done.
