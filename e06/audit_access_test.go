package e06

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	access "github.com/leafrun/leafrun-contracts/access/v1"
	audit "github.com/leafrun/leafrun-contracts/audit/v1"
	identity "github.com/leafrun/leafrun-contracts/identity/v1"
	"github.com/leafrun/leafrun-infra/internal/oauthdemo"
	"github.com/leafrun/leafrun-platform/events"
)

func ownerAuditKeyActions(t *testing.T, client *access.SessionClient, auth access.ResolveRequest, scope access.PermissionScope) (map[audit.ActionName]string, []string) {
	t.Helper()
	ctx := context.Background()
	botRequest := access.CreateBotRequest{OperationKey: "audit-private-bot-" + ownerNonce(t), Name: "Audit key lifecycle"}
	bot, err := client.CreateBot(ctx, auth, scope.WorkspaceID, scope.ProjectID, botRequest)
	if err != nil {
		t.Fatal("actual Audit bot creation failed")
	}
	botRetry, err := client.CreateBot(ctx, auth, scope.WorkspaceID, scope.ProjectID, botRequest)
	if err != nil || botRetry.BotID != bot.BotID {
		t.Fatal("actual Audit bot retry changed resource")
	}
	if _, err = client.Bind(ctx, auth, scope, access.BindingRequest{SubjectKind: "bot", SubjectID: bot.BotID, Relation: "generate"}, false); err != nil {
		t.Fatal("actual Audit bot ceiling binding failed")
	}
	issue := access.BotKeyRequest{OperationKey: "audit-private-key-" + ownerNonce(t), Operations: botGenerateOps}
	issued, err := client.IssueBotKey(ctx, auth, bot.BotID, issue)
	if err != nil || issued.Secret == "" || issued.Key.Version != 1 {
		t.Fatal("actual Audit key creation failed")
	}
	retry, err := client.IssueBotKey(ctx, auth, bot.BotID, issue)
	if err != nil || retry.Secret != "" || retry.Disclosure != "unavailable" || retry.Key.KeyID != issued.Key.KeyID || retry.Key.BotID != bot.BotID || retry.Key.Version != 1 || retry.Key.Status != "active" {
		t.Fatal("actual Audit key creation retry changed receipt")
	}
	rotate := access.RotateBotKeyRequest{OperationKey: "audit-private-rotate-" + ownerNonce(t), Operations: botGenerateOps, ExpectedVersion: 1}
	rotated, err := client.RotateBotKey(ctx, auth, bot.BotID, issued.Key.KeyID, rotate)
	if err != nil || rotated.Secret == "" || rotated.Key.Version != 2 {
		t.Fatal("actual Audit key rotation failed")
	}
	retry, err = client.RotateBotKey(ctx, auth, bot.BotID, issued.Key.KeyID, rotate)
	if err != nil || retry.Secret != "" || retry.Disclosure != "unavailable" || retry.Key.KeyID != rotated.Key.KeyID || retry.Key.Version != 2 {
		t.Fatal("actual Audit key rotation retry changed receipt")
	}
	revoke := access.RevokeBotKeyRequest{OperationKey: "audit-private-revoke-" + ownerNonce(t), ExpectedVersion: 2}
	revoked, err := client.RevokeBotKey(ctx, auth, bot.BotID, issued.Key.KeyID, revoke)
	if err != nil || revoked.Secret != "" || revoked.Key.Version != 3 || revoked.Key.Status != "revoked" {
		t.Fatal("actual Audit key revocation failed")
	}
	retry, err = client.RevokeBotKey(ctx, auth, bot.BotID, issued.Key.KeyID, revoke)
	if err != nil || retry.Secret != "" || retry.Key.KeyID != revoked.Key.KeyID || retry.Key.Version != 3 || retry.Key.Status != "revoked" {
		t.Fatal("actual Audit key revocation retry changed receipt")
	}
	task06BotDeniedSession(t, ownerNodeA, issued.Secret)
	task06BotDeniedSession(t, ownerNodeB, rotated.Secret)
	return map[audit.ActionName]string{audit.ActionBotCreate: bot.BotID, audit.ActionKeyCreate: issued.Key.KeyID, audit.ActionKeyRotate: issued.Key.KeyID, audit.ActionKeyRevoke: issued.Key.KeyID}, []string{botRequest.OperationKey, issue.OperationKey, rotate.OperationKey, revoke.OperationKey, issued.Secret, rotated.Secret}
}

type auditConsentFact struct {
	EventID, OperationID, ConsentID string
	Identity                        identity.Identity
	secrets                         []string
}

type auditAuthorityBinding struct {
	Ref                identity.AuthorityRef
	ActorID, SessionID string
}

func auditConsentAuthorityMatches(saved identity.Identity, actor, session string, registered auditAuthorityBinding) bool {
	return saved.Validate() == nil && registered.Ref.Validate() == nil && registered.Ref.Kind == identity.KindKratosSession && registered.ActorID == actor && registered.SessionID == session && saved.Principal == (identity.Principal{Domain: "human", Subject: actor}) && saved.Authority == registered.Ref
}

// Real fixed Kratos/Hydra/S256 provider/form protocol, driven by Go HTTP. This
// creates no fake provider acceptance and makes no independent Chromium claim.
// The callback listener is local to the checker container, with no host port.
func ownerAuditConsentFlow(t *testing.T, human ownerHuman) auditConsentFact {
	t.Helper()
	grants := make(chan oauthdemo.Grant, 1)
	demo, err := oauthdemo.New(oauthdemo.Config{PublicURL: "http://127.0.0.1:54444", TokenURL: "http://hydra:4444", CallbackURL: "http://localhost:58200/oauth/callback", ClientID: "leafrun-e06-demo", Audience: "leafrun-api", Scopes: []string{"leafrun.generate", "offline_access"}, HTTPClient: ownerHTTP(nil), OnGrant: func(g oauthdemo.Grant) { grants <- g }})
	if err != nil {
		t.Fatal("actual Audit OAuth demo configuration failed")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:58200")
	if err != nil {
		t.Fatal("checker-local Audit OAuth callback listener unavailable")
	}
	server := &http.Server{Handler: demo, ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(listener)
	defer server.Close()
	oauthRunBrowserForm(t, human, "")
	grant := oauthGrant(t, grants)
	intro := oauthIntrospect(t, grant.AccessToken)
	consent, _ := intro.Ext["consent_binding_id"].(string)
	session, _ := intro.Ext["kratos_session_id"].(string)
	if !intro.Active || intro.Use != "access_token" || intro.Subject != human.ID || consent == "" || session == "" {
		t.Fatal("actual provider consent/initiating session binding invalid")
	}
	ctx := context.Background()
	pool := ownerPool(t, "E06_ACCESS_DATABASE_URL")
	var saved []byte
	out := auditConsentFact{ConsentID: consent, secrets: []string{grant.AccessToken, grant.RefreshToken}}
	if pool.QueryRow(ctx, `SELECT f.audit_identity,f.audit_event_id::text,f.audit_operation_id::text FROM access_oauth_flows f JOIN access_oauth_consents c ON c.consent_binding_id=f.consent_binding_id WHERE f.consent_binding_id=$1 AND f.actor_id=$2 AND f.kratos_session_id=$3 AND f.purpose='consent' AND f.state='accepted' AND c.status='active'`, consent, human.ID, session).Scan(&saved, &out.EventID, &out.OperationID) != nil || json.Unmarshal(saved, &out.Identity) != nil {
		t.Fatal("actual owned consent activation/immutable audit context absent")
	}
	// AuthorityRef.ID is an Access-owned UUID, distinct from the Kratos session
	// identifier. Check the exact saved ref/version against the registry mapping
	// for this introspected original provider session and actor, never a latest
	// authority lookup by human identity.
	var registered auditAuthorityBinding
	if out.Identity.Validate() != nil || pool.QueryRow(ctx, `SELECT authority_id::text,kind,version,actor_id::text,provider_session_id::text FROM access_authorities WHERE authority_id=$1 AND version=$2 AND provider_session_id=$3 AND actor_id=$4 AND status='active'`, out.Identity.Authority.ID, out.Identity.Authority.Version, session, human.ID).Scan(&registered.Ref.ID, &registered.Ref.Kind, &registered.Ref.Version, &registered.ActorID, &registered.SessionID) != nil {
		t.Fatal("consent original authority registry binding absent")
	}
	if !auditConsentAuthorityMatches(out.Identity, human.ID, session, registered) || !events.ValidAuditID(out.EventID) || !events.ValidAuditID(out.OperationID) {
		t.Fatal("consent lost provider-verified original browser authority")
	}
	// Delete only this separate test browser session, after the activation commit.
	defer ownerLogout(t, ownerSession{ID: session})
	return out
}

func (fact auditConsentFact) pending(t *testing.T) {
	t.Helper()
	pool := ownerPool(t, "E06_ACCESS_DATABASE_URL")
	var raw []byte
	if pool.QueryRow(context.Background(), `SELECT encoded_envelope FROM platform_event_outbox WHERE event_id=$1 AND sent_at IS NULL`, fact.EventID).Scan(&raw) != nil {
		t.Fatal("actual outage consent activation lacks pending immutable owner fact")
	}
	e, action, err := events.DecodeAuditAction("leafrun.audit.access.action.v1", raw)
	if err != nil || e.OrganizationID != "" || e.WorkspaceID != "" || e.ProjectID != "" || action.Resource.ID != fact.ConsentID || action.Action != audit.ActionConsentGrant || action.OperationID != fact.OperationID || action.Actor != fact.Identity.Principal || action.Authority != fact.Identity.Authority {
		t.Fatal("pending identity-only consent fact provenance invalid")
	}
}

func (fact auditConsentFact) applied(t *testing.T, secrets []string) {
	t.Helper()
	ownerAuditExactStoredFact(t, access.PermissionScope{}, audit.ActionConsentGrant, fact.ConsentID, fact.EventID, fact.OperationID, fact.Identity, secrets)
}

func ownerAuditMembershipFact(t *testing.T, organization, human string, actor identity.Identity, secrets []string) {
	t.Helper()
	key, _ := json.Marshal([]string{"membership", organization, "organization", human})
	pool := ownerPool(t, "E06_ACCESS_DATABASE_URL")
	var resource string
	if pool.QueryRow(context.Background(), `SELECT resource_id::text FROM access_audit_resources WHERE resource_key=$1`, string(key)).Scan(&resource) != nil {
		t.Fatal("actual membership opaque owned resource absent")
	}
	ownerAuditExactStoredFact(t, access.PermissionScope{OrganizationID: organization}, audit.ActionMembershipAdd, resource, "", "", actor, secrets)
}

// Public history deliberately requires a complete project scope. Organization-
// only membership and identity-only consent are checked through the private
// read-only catalog on exact this-run owner resource/EventIDs instead.
func ownerAuditExactStoredFact(t *testing.T, scope access.PermissionScope, name audit.ActionName, resource, eventID, operationID string, actor identity.Identity, secrets []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	pool := ownerPool(t, "E06_AUDIT_DATABASE_URL")
	for {
		rows, err := pool.Query(ctx, `SELECT event_id,action_data FROM audit_actions WHERE organization_id=$1 AND workspace_id=$2 AND project_id=$3 AND producer='access' AND action_name=$4 AND resource_id=$5`, scope.OrganizationID, scope.WorkspaceID, scope.ProjectID, string(name), resource)
		if err != nil {
			t.Fatal("owned exact-scope Audit fact inspection failed")
		}
		count := 0
		for rows.Next() {
			var actualID string
			var raw []byte
			if rows.Scan(&actualID, &raw) != nil {
				rows.Close()
				t.Fatal("owned exact Audit fact scan failed")
			}
			action, err := audit.DecodeAction("leafrun.audit.access.action.v1", raw)
			if err != nil || !events.ValidAuditID(actualID) || !events.ValidAuditID(action.OperationID) || action.Action != name || action.Resource.ID != resource || action.Actor != actor.Principal || action.Authority != actor.Authority || (eventID != "" && actualID != eventID) || (operationID != "" && action.OperationID != operationID) {
				rows.Close()
				t.Fatal("actual exact-scope Access fact lost immutable original provenance")
			}
			for _, secret := range secrets {
				if secret != "" && bytes.Contains(raw, []byte(secret)) {
					rows.Close()
					t.Fatal("private input entered exact-scope Access Audit fact")
				}
			}
			count++
		}
		err = rows.Err()
		rows.Close()
		if err != nil || count > 1 {
			t.Fatal("actual exact-scope Access fact duplicated or unreadable")
		}
		if count == 1 {
			return
		}
		if ctx.Err() != nil {
			t.Fatal("actual exact-scope Access fact did not converge")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
