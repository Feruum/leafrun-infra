# E06 identity, authorization and audit design

Date: 2026-10-15. Delivery scope: Linear LEA-41, LEA-42–LEA-49.
Source: https://linear.app/bulnik/issue/LEA-41/e06-polzovateli-organizacii-roli-i-boty

## Outcome and boundaries

An authenticated human, bot or permitted guest can select a published release, submit parameters, render and download a PDF with current permissions. Minimal Kratos login and Hydra login/consent pages are included. Access owns hierarchy, membership, roles, bot credentials and public grants. Audit owns its history projection. Every owner enforces authorization.

Existing E04/E05 draft changes are the qualified starting point; their acceptance and merge remain separate. E06 does not implement Collaboration/WebSocket rooms, delivery callbacks or a full editor. Future WS/delivery consumers receive the same authority contract; the implemented HTTP/background boundaries must be tested now.

Eight repositories change: contracts, platform, access, audit, projects, generation, storage and infra. Core is an unchanged dependency, giving nine checkouts. Workspaces are isolated under .cache/leafrun-work/m06 on codex/e06-identity-access. Existing m04/m05 and their stand remain usable.

## Alternatives considered

1. Recommended: live Access authority checks backed by owned PostgreSQL, real Ory adapters, version-fenced Keto policy snapshots, and separate durable authority references. It preserves E05 content fingerprints and proves revocation without event delivery.
2. Locally verified bearer/JWT plus asynchronous invalidation is faster at the request boundary, but cannot meet current revocation after a lost event.
3. Membership and role checks implemented only in domain databases duplicate policy and omit required Keto integration.

## Identity and current authority

Keep stable Principal {subject, domain}. Move its definition to contracts/identity/v1 and use storagev1.Principal as a Go alias so wire JSON and existing comparisons remain unchanged.

Add a separate comparable AuthorityRef {kind, id, version} and Identity {principal, authority}. Kinds are kratos_session, bot_key, guest_session, oauth_token and service; dev exists only in explicit regression fixtures. The reference contains no credential, scope list, email or display name. Access resolves the reference to the actor and current scope ceiling. The server supplies identity; public input cannot select actor/authority.

ResolveResponse has optional expiresAt: it is mandatory and finite for Kratos/guest/OAuth sessions; bot keys without configured expiry can omit it. Any supplied expiry must be nonzero and future. This metadata never licenses cached authorization. The real public authenticator rejects service/dev identities independently of the internal service allowlist.

POST /internal/v1/auth/resolve accepts one bearer token or a narrowly forwarded Kratos session cookie behind a fixed per-service credential. Reject ambiguous credentials, duplicate headers and overlong input. The request is never logged or persisted. GET /v1/session returns verified identity and current session metadata. POST /internal/v1/checks/access uses a new accessv1.CheckRequest with principal, optional authority, PermissionScope and action. PermissionScope supports organization-only, organization/workspace and organization/workspace/project resources. Existing storagev1.Scope remains unchanged; legacy domain checks with workspace/project IDs are converted and Access derives their organization from its exact registry binding. Every supplied hierarchy component must match; audit queries provide complete scope. Non-service principals require a bound current AuthorityRef in access mode. A trusted service principal must match an explicit internal service/action allowlist. Fixed service credentials cannot authenticate a public user.

Use 401 for invalid/expired authentication, 403 for denied management actions, 404 where existing resource privacy requires hiding existence, 409 for changed idempotency input or stale version, 410 for expired artifacts only after successful authorization, and 503 for uncertain dependencies. No fallback allow, no cross-node positive authorization cache, no bearer token in URLs.

Authority must be passed separately from normalized content/execution data. Generation adds authority JSON columns for admissions/jobs; Projects adds initiating identity/authority to release pin intents. Existing fingerprints, stable initiator IDs and receipt namespaces remain byte-for-byte compatible. Old rows with no authority execute only under explicit dev regression mode; production fails closed. A new credential for the same actor may read an existing receipt after a fresh rights check; it does not revive work bound to a revoked original credential.

Use request context helpers for live handlers and explicitly restore identity from durable rows for worker checks. Never let a context be the only durable copy.

## Kratos and minimal login (LEA-42)

Use real pinned oryd/kratos v26.2.0. Kratos owns its database and schema. Access calls public /sessions/whoami for incoming credentials, then retains the nonsecret session ID and human identity ID. Background checks call GET /admin/sessions/{id}?expand=identity and validate active, expiry, identity match and enabled identity. An absent expanded identity denies access. Tokens, identity traits, passwords and cookies do not enter project data, workflow payloads, event data or logs.

The minimal server-rendered login page renders only allowlisted Kratos form nodes with html/template escaping. Preserve browser CSRF handling, session cookies and bounded return URL allowlist. Do not relay arbitrary URLs or expose Ory admin endpoints. Native/API login is exercised through the real Ory self-service API. Session resolution on two Access nodes reads the same Kratos state. Insecure cookies and HTTP issuer URLs are allowed only in the loopback E06 dev profile; production config requires TLS.

## Hierarchy and membership (LEA-43)

Owned tables: identities, organizations, organization_memberships, workspaces, teams, team_memberships, projects, project_roles, invitations and authorization_authorities. Every child has an organization foreign key and scope-qualified unique keys. Workspace IDs are globally unique and each workspace/project pair maps to exactly one organization under a database unique constraint. Public creation allocates server-generated IDs; a public caller cannot claim existing domain scope IDs. Importing an existing E05 scope requires an explicitly trusted owner/bootstrap API with verified binding. Personal projects have a human owner, team projects have a team in the same organization. An organizational workspace is metadata, not a temporary filesystem generation.

Create organizations with the authenticated human as owner. Enforce at least one organization owner under a row lock. Organization owners alone manage organization membership/invitations, teams and workspace-wide role bindings. A project owner manages roles/bots/grants only for that project and cannot assign organization/workspace ownership or manage another project through that role. Invitations are one-use random secrets, hash-only at rest, target an existing verified human identity and have a finite expiry; issuance returns the invitation once for manual distribution. This scope does not send email. Acceptance checks exact target identity, tenant, expiry and consumed/revoked state atomically.

Expose create/read/list and required membership/invitation management routes in access-v1 OpenAPI. Lists are scoped and bounded, ordered with opaque stable cursors. Guessed IDs and same-named IDs in another tenant never authorize access.

## Keto roles and policy synchronization (LEA-44)

Roles are reader, commenter, editor and owner. Reader: project metadata, source/resource read, job status and download. Commenter inherits reader plus comment capability (contract for later Collaboration). Editor inherits commenter plus project.write and generation create/execute/publish/cancel. Owner inherits editor plus project.publish, membership/role/bot/grant management and audit.read.

Capability relations independently grant generate, download, source.read and resource.read. Thus a generate-only participant does not receive the reader role. Credential scopes restrict the capability set; they never grant a relation by themselves. Explicit generate includes safe job status and execution/publication for that initiator; download still needs a distinct capability.

Keto evaluates the relations and inheritance. Access owns a monotonically increasing organization policy epoch. Each relation-changing transaction increments the desired epoch and records a durable reconciliation intent. Reconciliation writes an immutable complete graph under epoch-qualified Keto object IDs, verifies the source epoch is still current, then marks that epoch ready. Decisions require desired_epoch == ready_epoch, use that exact epoch in Keto, and re-read current epoch/authority after the remote check. A stale graph never grants access after membership removal. Observable dependency uncertainty or pending current policy yields 503. Every false/error result denies; the pinned Keto REST API can mask depth exhaustion and some internal database errors as allowed:false, so Access cannot universally distinguish those cases from ordinary denial. Obsolete graph cleanup is bounded and follows reconciliation; it cannot remove the current graph. This costs full-graph rebuilds in the MVP and favors correctness over large-tenant throughput.

Use organization/workspace/team/project relations and subject sets for inheritance, with strict tenant-qualified IDs. No cross-database SQL. Direct DB inspection belongs only in integration harnesses.

## Domain integration and separate permissions (LEA-45)

Generation and Projects replace static public credential maps with a context-aware authenticator. Storage keeps internal fixed service authentication and uses Access only for public artifact metadata/content. All constructors require an explicit auth mode; dev static maps are accepted only under a dev fixture flag. Production startup rejects missing Access config, service/public credential collisions and implicit fixture fallback.

The storage owner router sends Access checks to real Access while preserving Generation publishability and Projects/Generation pin ownership routes. Readiness checks each required dependency with bounded timeout.

Live checks are required at admission/replay, every public job read/cancel, source/parameter/schema exposure, trusted Projects resolve, input resource reads, worker start, deferred source pin/workflow activation, release pin activation, and before terminal/publication commit. Persist initiating authority for deferred activation. Denied activation does not publish a release or job; cleanup and pin release remain service duties after user revocation.

Operation-authority clarification (2026-10-15): original authority belongs to the operation it initiated. Release creation and its deferred pin acquire/activation/rotated intents preserve the creator's identity. A new synchronous publish/withdraw operation on an already-ready release uses the current publishing caller's authority, checked after locks against the exact release/version and confirmed usable bindings. The old creator's logout does not prohibit a different currently authorized owner from managing that ready release. A new request cannot rebind or activate original pending work whose creator authority was revoked. If publication itself is deferred or replayed, its intent preserves that publication operation's original caller. Test both independent management of a ready release and denial of pending-work revival/current-publisher revocation during a lock wait.

Generate-only job responses expose safe state/error codes and sanitized summaries. Parameters, raw compiler messages, hints, trace, source paths/content and input resource details require source/resource read. Form/schema endpoints apply their independent published-form policy and cannot expose source snippets through errors. Downloads require download/artifact.read, current credential/grant state, Generation publishability and fixed original artifact expiry. Storage proxies a concrete object version through a bounded read lease.

Revocation semantics: new requests and each authorization/side-effect boundary observe current authority. A download already opened may finish only within its existing read lease; new reads and renewals are denied after revocation/expiry. The implementation must not claim distributed atomicity between an Access decision and another owner's PostgreSQL commit. Recheck immediately before the side-effect commit; tests pause before that final check. No existing stream is a credential for another request. WS termination is a future Collaboration implementation requirement.

## Bot identity and key lifecycle (LEA-46)

Bots are separate principals with owned bot IDs and tenant membership/role bindings. Mint 256-bit random opaque keys, expose the secret once, store only a cryptographic hash and nonsecret key ID/version. Enforce exact bounded scope allowlist and optional finite credential expiry. Issue, rotate and revoke atomically; rotation invalidates the previous version immediately with no grace. Keep a stable bot principal across rotation. Server credentials determine actor; caller-supplied botId/author is rejected or ignored as untrusted according to the endpoint schema.

Every resolve and permission check reads current authority/key version. Test node A issuance/admission, node B rotation/revocation, node A HTTP and worker rejection with NATS unavailable. No human OAuth token authenticates a bot.

## Hydra and login/consent (LEA-47)

Use real pinned oryd/hydra v26.2.0 and a separate database/credentials. Configure authorization-code grants with enforced S256 PKCE, exact callback allowlist, no implicit/password flow and no bot impersonation by client credentials. Login accepts only a live verified Kratos human and server-derived subject. Login/consent GET renders bounded requested scopes; POST requires CSRF/state binding and explicit consent. Bind challenge, subject, client, exact scope set, audience and return path; prevent replay and session swapping. Callback state and verifier checks belong to the actual OAuth client as well as Leafrun's login/consent state; E06 provides a minimal registered demonstration client and tests it rather than claiming to control every external client.

Consent sets an Access-minted consent_binding_id and verified kratos_session_id in session.access_token metadata, not caller fields. This consent binding is shared by refresh-issued tokens and is distinct from a token-specific AuthorityRef. Hydra issues opaque access tokens. On each resolve/check, Access verifies introspection active, access-token use, subject, exact registered client/audience, bounded scopes, expiry/not-before and trusted consent/session bindings, plus current Kratos session.

Hydra opaque token introspection requires the token. For durable background revalidation Access alone retains it encrypted with AES-256-GCM in its owned secret column; the deployment injects a 32-byte key/key ID outside tracked source and project files. Each distinct token hash maps to its own immutable token-specific authority row with its ciphertext, provider expiry, consent binding and actor/session/client/audience/scope bindings. Bind encryption associated data to that authority and its actor/client/session. Refresh creates a new token authority and cannot overwrite or extend the original authority saved by a job. Jobs contain AuthorityRef only. Decrypt/introspection failure denies access. Wipe expired ciphertext by bounded cleanup. Tests revoke/expire an original token after refresh and verify its queued job remains denied while the new token follows its own state. This is explicitly encrypted credential persistence, not a claim that token hashes can be introspected. No token values enter audit/error/debug output.

## Public grants and magic-link (LEA-48)

A grant is bounded to one tenant/workspace/project, allowed role (reader/commenter/editor) and/or explicit capabilities, expiry, issuer and monotonically increasing version. Management requires current owner permission and cannot delegate more rights than the issuer. It does not delegate owner/admin/bot/grant management. Grant state is authoritative in Access DB and must remain compatible with current issuer/resource policy.

Task8 lifecycle clarification: the issuing credential is immutable issuance/audit provenance after grant commit; ordinary issuer logout, token expiry or key rotation does not revoke the separate guest authority. Each use still requires the stable issuer principal enabled, eligible in the exact tenant and currently an owner able to delegate those rights, plus the immutable issued role/capability ceiling. Later wider issuer credentials never widen that ceiling. Loss of issuer eligibility suspends new use while it is absent; restoring eligibility may allow the still-current unexpired grant version. This follows current-policy evaluation and adds no irreversible role-change cascade. Explicit grant revoke/rotation permanently invalidates its old version and guest sessions, regardless of later issuer restoration. Original issuer-submitted work remains bound to its original credential; guest-submitted work keeps its exact guest authority/grant version. Dependency uncertainty denies without silently revoking or restoring a version.

Create and rotate return a 256-bit secret once. Store hash only. A fragment link can carry the secret to the small exchange page, which posts it in the body; no query-string secret or request logging. POST /v1/grants/exchange validates secret and current version then mints a separate random guest-session credential. Guest sessions are bounded by min(grant expiry, configured session lifetime) and reference grant ID/version. They do not create a Kratos account.

Rotation/revoke commits a new version/status; every authentication/check reads it. Existing guest sessions cannot migrate silently to a new version. Test expiry boundary, repeated exchange, changed rights, revoke/rotate, service restart, cross-node requests and background publication with event transport unavailable.

Task8 browser integration uses two configured allowlisted cookie names: existing Kratos and a separate Access guest cookie. Every public owner uses the same bounded credential-selection rules. Bearer plus a selected cookie, both selected cookies, duplicates or malformed selected candidates deny without fallback. Internal resolve gains an explicit bounded selected-cookie-kind discriminator; its legacy absence means Kratos, and guest resolution never tries a failed Kratos credential. Public callers still supply no actor/authority. Exchange requires a finite one-use same-origin browser/CSRF challenge and explicit activation; it refuses a browser presenting a human cookie and never overwrites/revokes that human session. Existing guest replacement is deliberate and session-bound. A dedicated Access guest-logout action revokes only that guest session and clears only its cookie; a documented narrow cleanup exception may select the guest cookie when both are present, without authorizing domain operations. Cookie scope must reach all configured public owners, with HttpOnly/SameSite and Secure outside explicit loopback dev. Actual browser exchange/cookie/all-owner/coexistence/logout tests are required.

## Audit, event durability and authorized history (LEA-49)

Reuse per-owner Platform outbox/inbox. Extend events with schemaVersion=2 hierarchical optional organization/workspace/project identifiers; v1 validation and byte fixtures remain unchanged. V2 project implies workspace and organization, workspace implies organization. Identity-only events can have no tenant and are not readable from a tenant history endpoint. Do not put synthetic project IDs in organization events.

Create a strict versioned audit action payload: stable actor, authority kind/ID/version, operationId, action, resource kind/ID, outcome, optional revision. It contains IDs and enums only; no source, parameter values, identity traits, email, display names, passwords, cookies, tokens, key/grant secret or arbitrary error strings. Producers are allowlisted Access, Projects, Generation and Storage. Public callers cannot submit Audit records.

Producers record owner actions transactionally with their state/receipt. Storage records download.authorized before opening bytes, not a false download.completed. Generation records admission and terminal transitions; Projects records publish/retire; Access records membership/roles/credentials/grants/consent changes. Event ID is idempotent and a conflicting ID/payload is rejected.

Use a dedicated bounded LimitsPolicy audit stream with 30-day retention, DiscardNew, separate publisher/consumer ACLs and durable Audit consumer provisioned before producers. The exact subjects are leafrun.audit.access.action.v1, leafrun.audit.projects.action.v1, leafrun.audit.generation.action.v1 and leafrun.audit.storage.action.v1. Stream LEAFRUN_AUDIT captures leafrun.audit.*.action.v1; each publisher may send only its own subject, and payload producer must match the subject. Existing leafrun.generation.> and leafrun.projects.> streams do not overlap these subjects. Existing E04 InterestPolicy regression stream remains compatible; Audit does not depend on its dev consumer. Audit preserves distinct reordered actions once and commits history effect with inbox before ACK. Invalid wire records are rejected into bounded metadata-only quarantine (hash, subject, reason code); do not persist untrusted secret-bearing raw messages.

Audit API authenticates through Access and rechecks audit.read on exact organization/workspace/project scope on every page. Cursor cannot alter scope or bypass revocation. Internal replay/resolve routes require an explicit operator principal/action and record their own audit fact. Cleanup defaults to 30 days for audit/history bookkeeping, preserving unresolved quarantine and pending replay; finite retention is configurable and documented.

## Infrastructure and qualification

E06 has its own Compose project leafrun-e06 and separate volumes. Keep m05 stand leafrun-e04 untouched. All host ports loopback; Ory admin/Keto are network-internal. Defaults: PostgreSQL 58534, S3 58543, Temporal 57343, NATS 54322/58322, Generation 58190, Storage 58191, Projects 58196, Access nodes 58197/58198, Audit 58199, Kratos public 54433, Hydra public 54444. Startup verifies availability and permits explicit env overrides.

Access, Audit, Kratos, Hydra and Keto each have separate database/role/password and migrations. Provisioning validates existing ownership/privileges; never reset persistent local volumes. Secret generation preserves existing keys and uses ignored runtime files with restricted filesystem permissions. Images and source dependencies are pinned by digest/commit. Ory bootstrap is idempotent and does not expose admin endpoints or secrets.

Run meaningful unit/contract tests, real PostgreSQL/Ory/S3/Temporal/NATS scenarios, vet, build and Linux race checks. Include two-node current rights, dependency outage, revocation before terminal checks, source-sensitive diagnostics, snapshot release retention and fixed PDF TTL. Repeat restarts and lost ACK/replay. E03/E04/E05 fixture profiles continue as regression tests and do not satisfy E06 real-auth acceptance.

Publish linked draft PRs with LEA mapping, exact remote trees and verification evidence. Attach every PR. No merge, release or formal epic closure in this delivery.

## Verified Ory API and immutable image baseline

Official source verification performed 2026-10-15; latest public OSS release is v26.2.0, without assuming an enterprise support commitment.

| Image | Manifest digest |
| --- | --- |
| oryd/kratos:v26.2.0 | sha256:2a13bb8d362c7a7ae33bd7c0f5168aee46921f15c916a06346db91c06dc76643 |
| oryd/keto:v26.2.0 | sha256:bfdb8b9e283aedbd9e1d005c44eb1bbefd2096b2cb5b5aa0b7360b6e39c178d2 |
| oryd/hydra:v26.2.0 | sha256:ff67c7fb5f95074fa53374d41151713554960504b340cd3f95b09e65deaea2a9 |

Primary schemas: [Kratos](https://github.com/ory/kratos/blob/v26.2.0/spec/swagger.json), [Keto](https://github.com/ory/keto/blob/v26.2.0/spec/swagger.json), [Hydra](https://github.com/ory/hydra/blob/v26.2.0/spec/api.json). Immutable registry manifests are independently checked when pulled.

Kratos browser flow: GET /self-service/login/browser, GET /self-service/login/flows?id=UUID, POST /self-service/login?flow=UUID with method=password, identifier, password and browser CSRF token/cookie. Native GET /self-service/login/api is used only by API clients. Background GET /admin/sessions/{id}?expand=identity must validate the full current binding. [Pinned handler](https://github.com/ory/kratos/blob/v26.2.0/session/handler.go).

Keto check: POST /relation-tuples/check/openapi?max-depth=20 with namespace/object/relation and subject_id; require explicit allowed=true. Write: PUT /admin/relation-tuples or transactional PATCH /admin/relation-tuples with insert/delete relation_tuple entries; subject sets support inherited roles. OPL namespace config uses namespaces.location=file:///etc/keto/permissions.ts. Keto's latest/snaptoken fields are unimplemented in this release, so the application epoch fence is required. [Pinned transaction source](https://github.com/ory/keto/blob/v26.2.0/internal/relationtuple/transact_server.go).

Task4 source preparation clarified replay behavior before implementation: v26.2.0 tuple inserts create fresh physical rows, so duplicate PUT/PATCH is not an upsert. PATCH performs inserts before deletes. Reconciliation must canonicalize exact-object paginated tuple sets, insert only missing facts, verify complete equality and safely resume an uncertain outcome without clearing a potentially ready graph. Residual duplicates after a late provider commit are logically harmless but remain subject to bounded work. Parent object references use subject_set with an explicitly empty relation. These are source-established behaviors; full OPL parsing and real permission/replay acceptance remain Task4 tests. [Pinned persistence](https://github.com/ory/keto/blob/e4393662cd2e744deeb79de77669e07b6ccf51f3/internal/persistence/sql/relationtuples.go), [check engine](https://github.com/ory/keto/blob/e4393662cd2e744deeb79de77669e07b6ccf51f3/internal/check/engine.go).

Hydra admin login/consent requests use /admin/oauth2/auth/requests/{login,consent} and /accept or /reject with the corresponding challenge query parameter. Accepted consent session.access_token metadata appears in introspection ext and propagates through refresh; therefore its consent_binding_id must not be reused as the per-token authority ID. POST /admin/oauth2/introspect takes form token and returns active/sub/client_id/aud/scope/exp/nbf/iss/ext/token_use. Configure strategies.access_token=opaque and oauth2.pkce.enforced=true; the pinned implementation disables plain PKCE. [Pinned PKCE source](https://github.com/ory/hydra/blob/v26.2.0/fositex/config.go).

Individual revocation uses POST /oauth2/revoke with token/client authentication. Consent-chain revocation is implemented in this OSS release at DELETE /admin/oauth2/auth/sessions/consent?consent_request_id=ID; it does not replace individual current-token introspection. [Pinned revocation source](https://github.com/ory/hydra/blob/v26.2.0/persistence/sql/persister_consent.go).

Task7 source preparation clarified provider semantics: strict refresh already invalidates original access/refresh tokens and preserves extra claims in the successor; an active-token RFC7009 revoke can revoke the shared request chain. Token-specific Access authorities remain immutable and distinct regardless of this provider behavior. Hydra also relaxes callback ports for literal loopback IPs, so the separate demo client uses an exact localhost callback and Access validates the trusted challenge's exact redirect_uri; Kratos/Access browser origins stay on127.0.0.1. The provider's own verifier CSRF/replay checks do not replace Access's one-use HTML POST/challenge binding. These are source-established constraints and hypotheses for Task7's actual real-flow tests, not completed Hydra acceptance. [Pinned refresh tests](https://github.com/ory/hydra/blob/v26.2.0/oauth2/e2e_test.go), [revocation](https://github.com/ory/hydra/blob/v26.2.0/fosite/handler/oauth2/revocation.go), [callback matching](https://github.com/ory/hydra/blob/v26.2.0/fosite/authorize_helper.go).
