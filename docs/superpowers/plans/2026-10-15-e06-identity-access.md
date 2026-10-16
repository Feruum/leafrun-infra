# E06 Identity, Access and Audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use subagent-driven-development to implement this plan task-by-task. Each task has independent specification review followed by code quality review; fix and repeat before qualification. Under the user's subsequent request to accelerate development, next-task source implementation may overlap the preceding task's QUALITY review on an immutable source copy. Shared runtime changes wait for the prior review gate, and every correction must be integrated before the next qualification.

**Goal:** Complete LEA-42–LEA-49 so a current authenticated human/bot/guest obtains an authorized PDF from a published template and can inspect authorized audit history.

**Architecture:** Access owns live identity/authority and tenant policy; Ory Kratos, Keto and Hydra each own independent storage. Stable content identity is separate from revocable authority. Domain owners recheck current Access decisions; Audit builds a durable IDs-only projection from committed action events.

**Tech Stack:** Go 1.27, PostgreSQL 17, Ory v26.2.0 (digest pinned at provisioning), SeaweedFS S3, Temporal, NATS JetStream, HTTP/OpenAPI, server-rendered minimal authentication pages.

---

## Sources and delivery rules

Design: ../specs/2026-10-15-e06-identity-access-design.md.
Linear: LEA-41 parent; LEA-42 → LEA-43 → LEA-44 → LEA-45, LEA-46, LEA-47 → LEA-48 → LEA-49.
Implementation ownership follows the order below with one planned-feature lead at a time. To accelerate work as subsequently requested, the lead may coordinate component workers with explicit disjoint repository/file ownership and one shared-runtime coordinator. A fresh reviewer may examine a frozen preceding-task source copy while the implementer starts the next task's source work; this overlap follows the user's later speed instruction and does not remove review or runtime gates.

All paths below are relative to .cache/leafrun-work/m06 unless explicitly stated. Use gpt-6.1-sol/high/fork_turns=none for every implementer and reviewer. One planned-feature lead at a time, with explicit component ownership and instruction not to revert others. Component workers agree shared contracts before integration and qualify one coherent frozen result. A bounded correction from the preceding review may run concurrently in disjoint files, using frozen prior-task dependencies for qualification and deployment. Explorers may independently inspect disjoint topics. A fresh spec reviewer independently reads actual code; a fresh quality reviewer follows only after spec approval.

Each task:
- [ ] Capture starting clean commit/tree and relevant baseline checks.
- [ ] Add behavior tests that fail for the stated reason; preserve red output.
- [ ] Implement only assigned scope.
- [ ] Run focused green tests, then package tests/vet/build appropriate to changes.
- [ ] Commit coherent change(s).
- [ ] Pass independent spec review; fix findings and repeat.
- [ ] Pass independent quality/security review; fix findings and repeat.
- [ ] Record completion, actual commits, test commands/results and review receipts before next task.

The user has authorized plan then implementation and selected the subagent workflow. Routine implementation choices do not need a new permission request. Merge/release/formal Done remain separate acceptance.

## Common interfaces and invariants

Task 1 defines the transport, reused without growing Principal:

```go
package identityv1

type Principal struct {
    Subject string `json:"subject"`
    Domain string `json:"domain"`
}
type AuthorityRef struct {
    Kind string `json:"kind"`
    ID string `json:"id"`
    Version uint64 `json:"version"`
}
type Identity struct {
    Principal Principal `json:"principal"`
    Authority AuthorityRef `json:"authority"`
}
```

storagev1.Principal becomes an alias to identityv1.Principal. AccessCheck gains optional *identityv1.AuthorityRef; legacy nil is valid wire input but cannot authorize a human/bot/guest in production. Auth clients send service bearer plus one resolve input; server-derived Identity goes into context and separate durable columns.

```go
type Authenticator interface {
    Authenticate(context.Context, *http.Request) (identityv1.Identity, error)
}
type Checker interface {
    Check(context.Context, accessv1.CheckRequest) (bool, error)
}
```

The new accessv1.PermissionScope has organizationId/workspaceId/projectId, all individually optional with hierarchical validation. Organization-only and organization/workspace checks are supported. Project domain adapters convert storagev1.AccessCheck while retaining its existing two-field Scope; Access resolves omitted organization ID only from an exact stored workspace/project binding and rejects inconsistent supplied hierarchy. Audit supplies complete scope. Do not add organizationId to stable storage/project/generation content Scope.

Do not introduce another owner of domain policy in Platform. Platform may own the typed HTTP auth client and context helpers. Every Leafrun protocol JSON is bounded, strict single-value JSON rejecting duplicate/unknown keys and invalid UTF-8 via jsonvalue/v1. Ory adapters allow documented provider fields, reject duplicate/trailing JSON and validate all mandatory authority bindings explicitly; they do not confuse ignored provider metadata with permission.

## Task 1 — Shared authority contract and hierarchical audit envelopes

Linear mapping: foundation for LEA-42–LEA-49.

Files:
- Create contracts/identity/v1/{types.go,validate.go,validate_test.go}.
- Create contracts/access/v1/{types.go,scope.go,scope_test.go,client.go,client_test.go,openapi.go,openapi_test.go}.
- Create contracts/cmd/access-openapi/main.go and contracts/openapi/access-v1.json.
- Modify contracts/storage/v1/types.go (Principal alias; optional authority on AccessCheck), and relevant owner schemas.
- Modify platform/events/envelope.go and tests for schema v2 with OrganizationID.
- Create platform/authn/{context.go,client.go,client_test.go}.
- Create contracts/audit/v1/{action.go,action_test.go} for strict IDs/enums payload and subject allowlist.

Initial Access DTOs:

```go
type ResolveRequest struct {
    Bearer string `json:"bearer,omitempty"`
    SessionCookie string `json:"sessionCookie,omitempty"`
}
type ResolveResponse struct {
    Identity identityv1.Identity `json:"identity"`
    ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}
```

Reject both/none/oversized credentials; SessionCookie is only the selected allowlisted cookie's raw value. Do not format input in errors. Client caps response before strict decoding, has bounded context timeout, refuses redirects and treats dependency uncertainty distinctly from invalid authentication. ExpiresAt is required, nonzero and future for Kratos/guest/OAuth authority; a bot/service/dev authority may omit expiry, but any supplied expiry must be future. The real public authenticator rejects service/dev identities even if returned by a misconfigured upstream; explicit static dev adapters are separate. Authorization check caller credential and input actor are separate.

Red/green cases: stable Principal JSON/initiator fixtures unchanged; authority validation invalid kind/ID/zero/overflow version; resolve ambiguity; 401 vs 503; timeout/cancel/redirect; duplicate/trailing/oversized response; organization-only/workspace-only/project permission scopes and inconsistent hierarchy denial; conversion preserves stable two-field content Scope; v1 envelopes still require project/workspace; v2 identity/org/workspace/project hierarchy; no arbitrary audit data or secret-shaped fields. Tests assert existing E05 fingerprints do not change.

Run (from each changed repo with GOWORK=<absolute m06/e06.work>): go test ./...; go vet ./...; go build ./....
Add new modules to e06.work only once their go.mod exists. Generated OpenAPI must byte-match generator output.

Task 1 status (2026-10-15): complete at Contracts 90358b854499f3650a46a01d78c1df6a1baba3bf and Platform 64b6be6970d7660e50696433feb60bb6d503e103. Independent spec and quality reviews approved; native tests/vet/build, consumer compatibility and Linux race passed. PostgreSQL/Ory integration is reserved for subsequent tasks. Evidence: ../../e06-verification.md and ignored m06/reviews/task01 receipts.

## Task 2 — LEA-42: runnable Access, Kratos and current sessions

Files:
- Create access/go.mod, go.sum, .gitignore, .gitattributes.
- Extend contracts/access/v1 with the public /v1/session client/route, explicit public credential transport and generated OpenAPI tests.
- Extract Platform authn credential parsing into a reusable technical helper for Access; preserve all strict ambiguity/service separation behavior.
- Create access/cmd/leafrun-access/main.go.
- Create access/internal/config/{config.go,config_test.go}.
- Create access/internal/store/{store.go,migrate.go,authorities.go,authorities_test.go}.
- Create access/internal/store/migrations/001_access.sql (embedded from the owning store package).
- Create access/internal/ory/{http.go,kratos.go,kratos_test.go}.
- Create access/internal/service/{runtime.go,http.go,session.go,login.go,login_test.go,session_integration_test.go}.
- Create infra/Dockerfile.access, infra/ory/kratos.yaml, infra/ory/identity.schema.json.
- Create infra/compose.e06.yaml, infra/scripts/{e06-init-env.ps1,e06-database.ps1,e06-start.ps1,e06-test-env.ps1}.
- Modify infra/scripts/contexts.ps1 and add e06.work Access entry.

Access process modes: migrate/api/reconcile/event-relay (last two may start only when implemented). HTTP health is process liveness; readiness validates owned DB and required Ory dependency. Configuration rejects incomplete/insecure non-dev deployments. No constructor network I/O.

Store authority binding is unique on kind/provider ID/version and bound to exact human identity. Incoming whoami must be active, unexpired, supported identity state. Revalidate by admin session ID for durable checks without raw token storage. Login page only renders allowlisted trusted Ory nodes, uses escaped template, CSRF and return URL allowlist.

Provision dedicated E06 PostgreSQL/Access/Kratos only at this step; add other services as their tasks land. All host ports loopback, admin internal, private runtime files, no reset of m05. Validate existing DB owner/roles, idempotent bootstrap and independent credentials.

Behavior scenarios: real API login; real browser CSRF login; same session resolved by Access A/B; logout/inactivation/expiry denied on both; malicious return URL and forged identity rejected; Kratos outage returns 503; no password/token in persisted authority/audit/project source/logs. Include real PG concurrency/idempotent migration and repeated provision tests. Run native tests/vet/build and real Ory integration; record actual image digests.

Task 2 status (2026-10-15): complete at Access 2121785d06c265683a36a550a51af7fccb417731, Contracts 3bc4bd4d381047b733ab150a75960901332d8887, Platform 1cbd6f2907df2c8be3a7524b756207612205b2c1 and Infra 780254d35c44d43eea6a1a21bde0870d965fa786. Fresh independent specification and quality reviews approved. Actual Chromium A/B login/CSRF/logout, real PG/Kratos expiry/revocation/restart/outage, native checks and Linux race passed. Evidence: ../../e06-verification.md and ignored m06/reviews/task02 receipts. Task 3 follows; no draft publication, merge or formal acceptance is claimed at this checkpoint.

## Task 3 — LEA-43: tenant hierarchy, membership and invitations

Files:
- Create contracts/access/v1/{hierarchy.go,hierarchy_test.go}; extend OpenAPI/client/generator.
- Create access/internal/store/migrations/002_hierarchy.sql.
- Create access/internal/store/{hierarchy.go,membership.go,invitations.go,hierarchy_integration_test.go}.
- Create access/internal/service/{hierarchy.go,membership.go,invitations.go,hierarchy_test.go}.
- Extend access README with API examples and organization workspace distinction.

Tables use organization-qualified foreign keys and unique constraints; workspace IDs are globally unique and each workspace/project pair has exactly one organization under a DB unique constraint. Public creation allocates IDs server-side, never claims arbitrary existing domain IDs. Existing E05 scope import requires an explicitly trusted owner/bootstrap operation with verified binding. Initial organization creator becomes owner. At least one owner is maintained under transaction lock. Organization owners alone manage org membership/invitations/teams/workspace roles; project owners manage only their project's roles/bots/grants. Personal project owner and team-project team must belong to that organization. Invitations bind an existing human identity, contain hash-only secret, one-use state and expiry; response exposes secret once and no mail is sent.

Implementation dependency clarification (2026-10-15): Task 3 creates the server-allocated hierarchy/registry and proves public claims are denied. The actual existing-domain import operation and real owner verification land in Task 5, where Projects/Storage owner interfaces are in scope. Do not expose an unverified bootstrap before that integration. Domain workspace/project IDs remain exact bounded text compatible with E05; new IDs may be generated UUID strings. Qualify project lookups by workspace/scope rather than assuming globally unique legacy project IDs.

Route families: /v1/organizations; /v1/organizations/{id}/workspaces, /teams, /memberships, /invitations; /v1/workspaces/{id}/projects; /v1/teams/{id}/memberships; /v1/invitations/accept. Scope and actor always come from verified session/current ownership. Bounded paginated list APIs never scan another tenant for caller-provided IDs.

Behavior table: personal and team project create/read; same IDs cannot cross tenants; duplicate registry scope or public claim of existing domain IDs denied; invitation accepted only by exact target once; expiry/revoke/wrong target denied; concurrent last-owner removal cannot leave zero owners; forged project/team/workspace combinations fail atomically; a project-A owner cannot alter org membership/workspace ownership/project-B grants while org owner can manage the corresponding scoped operations; restart preserves membership. Real PostgreSQL tests and full Access/Contracts checks.

Task3 status (2026-10-15): complete at Access756e1d0fe0c99ca709f5edbed28adea4bdedeb53 and Contractsee26c1149710ae55c7b96fa9822bc13f5449a9ed. Fresh independent specification and quality reviews approved. Real PostgreSQL, actual Kratos-backed A/B hierarchy/lock-wait revocation, both-process restart persistence, native checks/OpenAPI byte-match and Linux race passed. Evidence: ../../e06-verification.md and ignored m06/reviews/task03. The actual verified legacy-owner import stays in Task5. Task4 follows; no draft publication, merge or formal acceptance is claimed.

## Task 4 — LEA-44: Keto roles and epoch-fenced reconciliation

Files:
- Create contracts/access/v1/{permissions.go,roles.go,roles_test.go}; extend OpenAPI/client.
- Create access/internal/store/migrations/{003_policy_epochs.sql,004_policy_tenant_guard.sql,005_policy_reconcile_schedule.sql}.
- Create access/internal/ory/{keto.go,keto_test.go}.
- Create access/internal/policy/{model.go,graph.go,reconcile.go,decision.go,policy_integration_test.go}.
- Create access/internal/service/{roles.go,checks.go,checks_test.go}.
- Create infra/ory/keto.yaml, namespaces.ts; extend compose.e06.yaml/provisioner/config.
- Wire all hierarchy/membership writes to policy-epoch transactions.

Implement only four role names and separately grantable capabilities. Credential scopes form a ceiling. Organization membership, workspace/project roles and team subject sets inherit through actual Keto. Desired epoch increments with relation changes and durable intent. Complete immutable graph uses epoch-qualified IDs; ready epoch advances only if still current. Decision checks current authority and ready/current equality before Keto and again after. Pending policy/Keto error never allows; obsolete generation cannot become ready.

Pinned-provider preparation (2026-10-15): tuple insert replay is not a physical upsert, PATCH inserts precede deletes, parent references require an empty-relation subject_set, and list pagination must follow next_page_token until empty. Build a canonical complete tuple set, serialize builders, read exact manifest objects before retrying uncertain writes, insert missing facts and verify complete equality before ready. Keep exact-object cleanup bounded and protect current/ready/active-builder graphs. No provider exactly-once claim is made. Observable dependency uncertainty is503; some pinned Keto internal failures/depth exhaustion surface as allowed:false and must remain denied. Source-backed preparation is in ignored m06/reviews/keto-opl; actual parser/matrix/replay validation remains this task.

Tests: full role/action matrix and source/generate/download separation; inherited team/workspace roles; two independent API nodes; stale Keto allow after revocation; obsolete reconcile wins ordering; process crash during graph write then replay; cross-tenant subject set; dependency timeout; unknown operation denied; service/action allowlist. Use actual Keto+PG, tests should fail against stale positive policy. Do not proceed until spec and quality reviews approve revocation semantics.

Task 4 status (2026-10-15): local qualification complete at Access `8bb43787b1a326b00ff1067efdadb3ab7b4c19d7`, Contracts `cd1cca2361157cabf854c5e1fed69efe336ac5d8`, Platform `a67b50f39d7b42145bc489bf0e77b6037b4c921a` and Infra runtime `ffc082d74072f9d17a26b4ce1883daf81335a5f7`. Fresh independent specification and quality re-reviews approved after correcting cross-tenant reconciliation starvation. Migration 005 and one durable SKIP LOCKED claim per fresh bounded pass preserve graph/epoch safety and finite-queue progress across failures, locks and restarts. Native owned PostgreSQL, actual PG/Keto/Kratos/A-B Linux race, literal crash/replay and process fairness passed. Evidence: ../../e06-verification.md and ignored m06/reviews/task04 receipts. Task 5 domain-owner enforcement follows; no formal Linear Done, publication, merge or release is claimed here.

## Task 5 — LEA-45: owner enforcement and durable authority integration

Files:
- Modify generation/internal/service/{principal.go,http.go,types.go,runtime.go,dependencies.go,admission.go,render_selection.go}.
- Create generation/internal/store/migrations/003_e06_authority.sql; modify internal/store/{types.go,store.go,recovery.go,admission.go,execution_input.go,terminal.go}.
- Modify generation/internal/controller/{runtime.go,publication.go,input_resources.go}.
- Modify generation/cmd/leafrun-generation/config.go.
- Modify projects/internal/service/{http.go,types.go,access.go,resolve.go,reconciler.go}.
- Create projects/internal/store/migrations/002_e06_authority.sql; modify internal/store/{store.go,releases.go,reconcile.go,publication.go}.
- Modify projects/internal/config/config.go and cmd/leafrun-projects/main.go.
- Modify storage/internal/storage/{http.go,config.go,owner.go,snapshots.go,reads.go}.
- Modify infra/internal/ownerrouter/server.go; extend E06 router/config and service auth mode.
- Add the explicit trusted Access existing-scope bootstrap operation and Projects/Storage owner verification contract/adapter, with focused real import tests.
- Add focused tests beside each touched package and infra/e06/permissions_test.go.

Explicit access/dev authentication mode; no public static maps in access mode. Internal service credentials stay separate. Context-aware authenticator distinguishes invalid login from unavailable Access. Owner checks attach verified AuthorityRef, including trusted Projects resolves. New authority columns stay outside input fingerprints and receipt identity; backfill-free old rows are production-denied and dev-regression-compatible.

Implement the deferred existing-domain import behind an explicit trusted owner/operator bootstrap credential, separate from public user APIs. Verify the exact existing workspace/project pair against its real domain owner's stored facts before assigning it to a real organization/workspace/project record. Preserve original domain Scope strings and unique immutable tenant binding; a public organization owner cannot claim guessed legacy IDs. Absent owner verification or dependency uncertainty fails closed. Test successful verified import, missing/foreign scope, public claim denial, duplicate/concurrent different-organization claim and provider outage without a registry mutation. Import also records the policy epoch/reconciliation intent; never introduce a temporary allow fixture.

Persist initiator authority in jobs and releases; revalidate before source pin/workflow intent activation, release pin activation, worker execution/input reads, Storage finalize/publication boundary and terminal commit. Cleanup remains service-authorized. Replay checks the new current request authority before returning receipt but never replaces original job authority. A historical success already committed while authorized stays publishable until its normal lifecycle expiry; revoking its initiator does not retroactively revoke every other authorized reader. Every new caller still needs its own current download permission, and queued/uncommitted work keeps the original authority binding.

Add a lightweight durable initiator/authority loader for pending admissions. GetExecutionInput currently rejects inactive input pins, so it cannot be used to authorize the very recovery that activates them. EnsureSourcePin and release reconciliation must restore the saved original authority, never the new request/service context. A denied activation needs a durable failure/cleanup state instead of retaining pin_pending indefinitely.

For Projects SetPublication, a new synchronous operation on an already-ready release checks the current publishing caller after locks against its exact scope/version and confirmed usable pins. It does not require a past creator's live credential for every later management operation. Preserve the creator's identity on original deferred pin work and every retry/rotation; another publish request must never activate or rebind that pending work. Any deferred publication operation instead saves its own original publishing caller. Test creator logout after readiness with another current owner allowed, pending creator revocation with no revival, and current publisher revocation during a proven row-lock wait with rollback.

Register every new migration explicitly: Generation Migrate lists only 001/002 today and Ready probes old columns; Projects Migrate lists 001 only and schema version has CHECK(version=1). Update migration registration/readiness probes and version constraint/idempotent rerun behavior. Place mutating final gates after owner row-lock waits and fencing checks, immediately before receipt/state/outbox commit (for example bounded callbacks on TerminalSuccess, MarkPinActive and SetPublication with bindings from the locked row). A bool checked before a blocking store call is insufficient. Cleanup callbacks must not require current initiating user authority.

Generate-only tests embed secrets in compiler messages/hints/trace/source paths/parameters. Safe status must contain none without source rights. Separate resource-read and download tests; current published release checks remain. Metadata/content require current download right plus publishability/TTL; already-open read has bounded lease, no new request bypass.

Run all changed owner unit/integration packages and original E04/E05 regression suites. Freeze fingerprint fixtures before/after; test revoked authority while paused just before final check, while waiting for an owner row lock, and with event broker unavailable. Test pending-pin authority loading, same-actor new-key replay preserving old authority, migration/readiness rejection before upgrade and revoked-authority cleanup still completing. Report the precise authorization boundary without claiming distributed transactions.

Task 5 status (2026-10-15): local qualification complete after corrected SPEC and QUALITY approval, with Generation `061bc61ee34402e6e7e87b02c86fefd9161503db` and Projects `f74cf4416d91b312df86a79fb408dd33d800a77e` against the frozen Task 5 dependency set. Current-caller deletion and full saved-purpose worker-Start gates correct both independent review findings. Fresh focused reviews, frozen Linux race/vet/build, exact source/image verification and corrected actual owner runtime passed with explicitly recorded skips and environmental limits. See ../../e06-verification.md and ignored reviews/task05 receipts. Task 6 bot source was developed concurrently on separate ownership; shared runtime deployment clears only now after both reviews. Formal Linear Done and publication remain later gates.

## Task 6 — LEA-46: bot principals and versioned keys

Files:
- Create contracts/access/v1/{bots.go,bots_test.go}; extend client/OpenAPI.
- Create a new append-only Access bots migration using the next unused version at execution (004/005 are already qualified policy migrations).
- Create access/internal/store/{bots.go,credentials.go,bots_integration_test.go}.
- Create access/internal/service/{bots.go,bots_test.go}.
- Extend authority resolution/check and policy graph for bot subjects.
- Create infra/e06/bots_test.go.

Routes: POST /v1/workspaces/{workspaceId}/projects/{projectId}/bots; POST /v1/bots/{botId}/keys; POST /v1/bots/{botId}/keys/{keyId}/rotate; DELETE /v1/bots/{botId}/keys/{keyId}. Require current owner management authority and exact tenant/workspace/project. Project routes include workspace because legacy project IDs can repeat. Key issue exposes random secret once; rotation/revoke are idempotent under operation receipt and changed input conflicts. Store SHA-256 of high-entropy random secret, version and expiry, never original secret.

One-time secret versus lost response: a committed retry/read returns the original public key/version/expiry receipt after fresh management authorization, without redisclosing the secret or rotating again. The typed result explicitly distinguishes first disclosure from a metadata-only receipt. If the first secret response was lost, the owner can perform a new rotation with a new operation key; do not retain plaintext/encrypted secret in a receipt or invent a new secret under the old key. Concurrent identical operations produce one state change. Tests cover loss/retry, current-rights denial and changed-input conflict.

Cases: valid bot renders/releases according to assigned role/scopes; insufficient scoped action denied despite owner role; body author/botId spoof rejected; another node observes rotate/revoke without events; concurrent rotation has one current winner; key expiry; old queued job cannot publish; same stable actor receipt under new valid key does not duplicate work. Inspect persisted DB rows and audit payload shapes for absence of secrets.

Task6 status (2026-10-15): locally qualified after full SPEC and fresh independent QUALITY approval on the coherent final source set in ../../e06-verification.md. Actual bot/render/cleanup, Storage and durable-secret Linux race passed6 top-level/26 total without skips/failures; fresh QUALITY native23/36 and actualPG7/11 passed without skips/failures. All665 source hashes,14 running images, renderer binding and five APIs were verified. Source fixes and bounded log/removed-renderer limits remain explicit. Task7 source overlapped immutable QUALITY; its shared-runtime gate is now cleared. Draft PR, hosted CI and formal Done remain later gates.

## Task 7 — LEA-47: Hydra authorization code login/consent

Files:
- Create contracts/access/v1/oauth.go and tests; extend OpenAPI.
- Create a new append-only Access OAuth migration using the next unused version after the qualified bots/import migrations.
- Create access/internal/ory/{hydra.go,hydra_test.go}.
- Create access/internal/service/{oauth.go,consent.go,oauth_test.go}.
- Create access/internal/secrets/{cipher.go,cipher_test.go}.
- Extend authorities for encrypted opaque token references/current introspection.
- Create infra/ory/hydra.yaml; extend E06 bootstrap/config/secret keys.
- Create infra/e06/oauth_test.go and a minimal registered OAuth demo-client/browser-flow fixture with its own callback/state/verifier validation.

Configure v26.2.0 public client authorization_code+refresh_token, exact loopback callback, explicit audience/scope allowlist, enforced S256 PKCE. Only live Kratos human subject is accepted by login/consent. Persist challenge/state/CSRF binding and mark consumed; validate subject/client/scope/audience at each transition. Hydra-generated redirect is validated against fixed provider origin, not arbitrary user URL.

Pinned-provider preparation (2026-10-15): use an exact localhost demo callback rather than a literal127.0.0.1 callback, because Hydra intentionally relaxes literal loopback-IP callback ports. Keep existing Kratos/Access browser origins on127.0.0.1; the separate loopback demo client owns its own state cookie. Verify the trusted challenge's exact redirect_uri locally too. Hydra admin acceptance can repeat, so Access must independently consume its own CSRF/challenge binding. Registered consent scopes/audiences require explicit subset checks in Access; do not assume trusted admin acceptance enforces them. Refresh does not reliably downscope from a requested narrower scope. Source-backed protocol and placeholder requests are in ignored m06/reviews/hydra-flow-map; actual provider/browser acceptance remains this task.

Token ext includes Access-created consent_binding_id and kratos_session_id, which may propagate across refresh. They do not select token authority ID. Access verifies every introspection field and live Kratos session. For each unique accepted token hash, mint a separate immutable authority row with its own encrypted ciphertext, expiry and actor/session/client/consent/scope/audience bindings. Durable checks decrypt that exact row with associated-data binding using keyId and injected key, introspect the same token and compare exact binding. Refresh creates another authority, never overwrites or extends the old one. Expired ciphertext is removed; no jobs/workflows carry token. Wrong key, malformed ciphertext, Hydra/Kratos outage fail closed.

Real-flow tests: correct state+PKCE success, wrong/missing verifier failure, alternate callback port failure, swapped/replayed state/challenge/consent rejected, invalid requested scopes rejected, human app cannot choose bot identity, logout/revoked token/expired token denies queued publication and download. Refresh maintains verified consent binding and scopes while minting a distinct token authority; pinned strict refresh already invalidates the original access/refresh tokens. Prove the original queued authority and persisted expiry remain unchanged and denied while its successor follows its own current state. Separately test original-token expiry and provider revocation: revocation of a discoverable active token can revoke its entire request chain; revoking an already-absent original access token after refresh is expected to be a no-op and must be real-tested, not attributed as the cause of its earlier denial. Revoke one consent chain while an independent same-human/client chain remains active. Test minimal browser login/consent with actual provider, not only mock HTTP.

Task7 status (2026-10-16): full SPEC approved Access38679c5/Contracts470e88c/Infraa364f29 after all five actual Chromium phases and the actual owner lifecycle under Linux race. Native/OAuth forms, CSRF/state/replay, callback scope, immutable original/successor token authority, PDF/download boundaries, refresh/revoke/independent chain, restarts, expiry and ciphertext wiping are qualified. Append009 preserves the exact compact Hydra consent selector without changing001–008. Final702 source hashes, nine Access ledger checksums,14 application images/renderer/six readiness endpoints were independently verified. See ../../e06-verification.md and ignored reviews/task07 final receipts. Fresh independent QUALITY/security also approved with15 top-level/39 total fresh tests, zero skips/failures and no findings. Task7 is locally complete; Task8 shared runtime is now cleared after its source overlap. Draft PR/hosted CI and formal Done remain later gates.

## Task 8 — LEA-48: scoped grants and guest sessions

Files:
- Create contracts/access/v1/{grants.go,grants_test.go}; extend client/OpenAPI.
- Create a new append-only Access grants migration using the next unused version after the qualified OAuth migrations.
- Create access/internal/store/{grants.go,guests.go,grants_integration_test.go}.
- Create access/internal/service/{grants.go,guests.go,magic_link.go,grants_test.go}.
- Extend authority/policy checks and cleanup.
- Create infra/e06/grants_test.go.

Routes: POST /v1/workspaces/{workspaceId}/projects/{projectId}/grants; POST /v1/grants/{grantId}/rotate; DELETE /v1/grants/{grantId}; POST /v1/grants/exchange. Project routes include workspace because legacy project IDs can repeat. Version-checked current owner management, finite expiry, bounded delegated capabilities, no owner/admin delegation. Random link secret hash-only; guest credential separate hash-only; grant ID/version permanently binds guest session. Any idempotent link-issuance/rotation receipt follows Task6's one-time disclosure/lost-response rule; it never persists a recoverable link secret.

Small fragment exchange page posts secret in body and clears fragment, sets bounded guest session cookie. No secret query string, referrer or logging. Session lifetime is min(grant expiry, configured maximum). Repeated link exchange is allowed while current; guest cannot silently upgrade across rotate.

Lifecycle clarification: committed issuance credential is provenance, while current stable issuer enabled/tenant/owner policy and immutable issued ceiling constrain every guest use. Normal issuer logout/key rotation does not revoke the independent grant. Eligibility loss suspends use; restoration may resume a still-current unexpired version. Explicit grant revoke/rotation never revives old guest versions. Original issuer jobs still require their saved original credential. Test these cases separately, including current-policy uncertainty and no expiry/ceiling extension.

Cookie/CSRF integration: extend Contracts resolve with a bounded selected-cookie-kind discriminator (legacy absence means Kratos) and Platform credential parsing/config across Access, Generation, Projects and Storage for exactly the human and guest names. Reject bearer+cookie, both cookies, duplicates/malformed candidates without fallback. Never overwrite the provider cookie. Protected explicit exchange refuses human-cookie coexistence, preserves it and deliberately handles existing guest replacement with browser/session-bound one-use state. Access guest logout revokes only that session; a narrow explicit cleanup exception can remove guest when both cookies exist without choosing an identity for domain actions. Prove actual fragment->protected POST->cookie->all-owner requests, ambiguity/coexistence and retained-old-cookie denial on A/B. A bearer-only test cannot satisfy guest browser acceptance. Read-only clarification review is in ignored m06/reviews/guest-lifecycle-review; root selected current-policy suspension rather than adding irreversible role-change fencing not required by the sources.

Tests: node A exchange then node B rotate/revoke while NATS down; A/B request/replay/source/download denied; queued/current job stopped at worker/publication boundary; revoked issuer policy invalidates old delegation; exact expiry boundary; guest cannot widen roles/resources/scopes; cross-project guessed artifact denied; restart preserves revocation; TTL never extended by grant/pin/read renew. Existing bounded read is documented separately.

## Task 9 — LEA-49: strict audit action producers and owned history

Task8 status (2026-10-16): FULL SPEC and independent QUALITY approved Access88b72f8/Contractsb91e61b/Infrae2e479e. QUALITY ran24 top-level/36 total fresh tests with zero skips/failures. Final runtime evidence covers733 source files, exact application images,18 actual Chromium guest-flow assertions and current/revoked credentials across all three domain owners. Scoped plaintext checks cover seven distinct privately held test values,23 owned logs and Access metadata; this is bounded evidence. The initial test-grant fragment exposure was contained by revocation and verified denial on both Access nodes. The original partial acceptance red file was overwritten during fixture repair; retained safe diagnosis is not represented as an intact original artifact. See ../../e06-verification.md and ignored reviews/task08 final receipts.

Task9 protocol clarifications: finite cleanup covers inactive grant metadata and grant receipts, while bot create/issue deduplication remains tied to the existing stable bot lifecycle. New grant creation requires a canonical UUIDv7 operation key with an immutable30-day protocol window; current authentication precedes age checks and expired operations return410. Existing arbitrary keys may only replay a matching retained, unexpired receipt; a missing legacy key cannot create a grant. Rotation/revocation retain their version fences. Metadata retention is configurable but cannot shorten that operation window, and cleanup protects live bindings, required receipts, pending outbox and unresolved work. Increasing a configurable retention value cannot extend the immutable operation-key window.

Consent facts are authored at the Access-owned successful FinishOAuthAccept activation transaction. External Hydra revocation is checked as current provider state and does not fabricate an Access-owned revocation action. Pre-upgrade accepting OAuth rows lacking a verified original authority and audit IDs remain retained and fail closed; recovery requires a fresh verified flow or deliberate operator resolution, without synthesizing an initiator. New flows persist the exact original authority and action IDs before provider work.

Legacy Generation jobs without audit context cannot publish a new result. Bounded cancellation/expiry cleanup uses the existing Access CheckScope contract with the explicitly allowlisted generation service actor leafrun-generation and generation.cleanup action. It records a separate truthful service authority and canonical registry scope, preserving the original job initiator rather than inventing or replacing it. Ordinary human/bot/grant callers cannot select this service-only capability.

Files:
- Create audit/go.mod, go.sum, .gitignore, .gitattributes.
- Create audit/cmd/leafrun-audit/main.go; internal/config/config.go.
- Create audit/internal/store/migrations/001_audit.sql.
- Create audit/internal/store/{store.go,migrate.go,history.go,quarantine.go,history_integration_test.go}.
- Create audit/internal/consumer/{consumer.go,handler.go,consumer_test.go}.
- Create audit/internal/service/{http.go,history.go,history_test.go}.
- Extend contracts/audit/v1 with history DTOs/client/OpenAPI; create cmd/audit-openapi and openapi/audit-v1.json.
- Add audit-action production to Access owner transactions, Projects releases, Generation admissions/terminal commits and Storage download receipts.
- Add platform reusable outbox/inbox migrations to each changed owner's owned database when absent.
- Create infra/Dockerfile.audit, internal/e06events/{stream.go,acl_integration_test.go}, E06 service/stream/bootstrap config.
- Create infra/e06/audit_test.go.

Use dedicated bounded LimitsPolicy stream LEAFRUN_AUDIT (30-day default, DiscardNew) capturing leafrun.audit.*.action.v1, and preprovision Audit durable before producers. Exact producer subjects are leafrun.audit.access.action.v1, leafrun.audit.projects.action.v1, leafrun.audit.generation.action.v1 and leafrun.audit.storage.action.v1, with per-producer publish ACLs and payload/subject matching. Existing generation/projects streams coexist without subject overlap. Transactional per-owner action IDs and strict payload/producer allowlist. Preserve reordered distinct action records, reject same ID/different payload. Validate the strict audit payload and subject binding before Platform Consumer.Process, which persists the envelope before calling its handler. Invalid payload quarantine stores only hashes, fixed reason code and an exact allowlisted subject or subject hash; an arbitrary raw subject may itself contain a secret. Authorization decisions and error strings cannot leak secrets.

Read API: GET /v1/audit/actions with exact tenant scope, bounded limit and opaque cursor; authenticate/current audit.read each page. Operator replay/resolve either remains internal with explicit service+operator allowlist or exposes an authenticated audited operation; no open debug endpoint. Cleanup preserves unresolved quarantine/replay and respects configured finite history retention.

Tests: membership/key/grant/consent/Release/job/download authorized facts with true actor/version/operation/resource, duplicate/lost ACK, NATS outage then recovery, consumer restart/quarantine/replay, malicious payload has no raw persistence, foreign/revoked audit reader denied, cursor scope tampering denied. Real provisioning proves all generation/projects/audit streams coexist, history survives without the dev consumer, and one producer cannot publish another producer's subject or mismatched payload. Production cannot admit action without committed outbox.

## Task 10 — Full E06 qualification and regression

Files:
- Create infra/e06/{mvp_test.go,revocation_test.go,isolation_test.go,restarts_test.go}.
- Create infra/scripts/{e06-verify.ps1,e06-verify.sh}.
- Extend infra/.github/workflows/e03-verify.yml or create e06-verify.yml with explicit pinned checkout inputs.
- Extend infra/{ci/dependencies.json,scripts/ci-check.ps1,scripts/linux-checks.ps1,scripts/linux-checks.sh}.
- Add access/audit to workspaces/build contexts/check lists and record E06 evidence.
- Create infra/docs/e06-verification.md.

Required chain: actual local Git origin → Core capture → Projects revision/release/published form → user and bot parameters → Generation durable job/Temporal worker → versioned S3 PDF → authenticated Storage download. Run real Ory, PostgreSQL, NATS, Temporal, SeaweedFS. Include Access A/B and domain API replicas or two independently configured runtimes sharing owner DB.

Revocation tests pause at defined last authorization boundaries, revoke on another node with NATS disabled, then resume; no new job/result publication/download. Check user logout, bot key rotation/revoke, guest grant rotation/revoke, OAuth token revoke, membership removal and dependency uncertainty. Cross-tenant source/resources/releases/job/artifact/audit IDs never cross scope. Fixed 24-hour artifact TTL and published snapshot retention remain unchanged.

Restart services/provider/worker without volume reset; exercise finalize/terminal ambiguity, durable outbox/inbox retries and policy reconcile. Run complete E03/E04/E05 fixtures separately using explicit dev auth. Native unit/contract tests, go vet, go build; Linux go test -race for every Go repo and three-OS compile/test where existing CI supports it. Evidence includes exact head/tree, flags, pass counts and skipped tests; mocks alone cannot satisfy Ory acceptance.

## Task 11 — Linked draft PR delivery and Linear evidence

Files: infra/docs/e06-verification.md, ci/dependencies.json, publication receipts under ignored m06/publication.
- [ ] Run final independent cross-repository review and fix all correctness/security findings with re-review.
- [ ] Publish eight linked draft PRs (access, audit, contracts, platform, projects, generation, storage, infra) with LEA-42–LEA-49 coverage and dependent E04/E05 PR links.
- [ ] Compare exact local and remote trees and ensure tracked sources clean before source-pin/CI qualification.
- [ ] Attach every created PR through Codex artifact tool.
- [ ] Run hosted E06/regression CI, inspect results/artifact exact pins, fix failures and rerun affected checks.
- [ ] Append concrete code/PR/test evidence to Linear without formal Done, merge or release.
- [ ] Give user direct PR links, completed task coverage, actual remaining acceptance/dependencies and material limitations.

## Plan review checklist

- [ ] All eight Linear acceptance criteria map to tasks and test evidence.
- [ ] Stable content fingerprints/initiator namespaces remain compatible.
- [ ] Revocation proof uses current state without delivery/cache assumption.
- [ ] Ory versions, exact APIs, database ownership, endpoint exposure and secrets are explicit.
- [ ] Real-auth E06 scenarios are distinct from fixture regression passes.
- [ ] Every task has red/green tests, independent spec/quality gates and commit evidence.


