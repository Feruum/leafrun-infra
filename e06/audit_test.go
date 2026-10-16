package e06

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	access "github.com/leafrun/leafrun-contracts/access/v1"
	audit "github.com/leafrun/leafrun-contracts/audit/v1"
	generation "github.com/leafrun/leafrun-contracts/generation/v1"
	projects "github.com/leafrun/leafrun-contracts/projects/v1"
	storage "github.com/leafrun/leafrun-contracts/storage/v1"
	"github.com/leafrun/leafrun-platform/events"
)

// This test owns fresh identities/scopes only. The separate root browser fixture
// is neither loaded nor changed. Private credentials never enter evidence output.
func TestRealAuditFourOwnerFactsAndCurrentReader(t *testing.T) {
	if os.Getenv("E06_AUDIT_INTEGRATION") != "1" {
		t.Skip("explicit owned E06 Audit runtime disabled")
	}
	ownerEnabled(t)
	if os.Getenv("E06_AUDIT_OPERATOR_TOKEN") == "" {
		t.Fatal("owned Audit operator credential setting missing")
	}
	ctx := context.Background()
	manager, reader, foreign := ownerIdentity(t), ownerIdentity(t), ownerIdentity(t)
	ms, rs, fs := ownerLogin(t, manager), ownerLogin(t, reader), ownerLogin(t, foreign)
	defer ownerLogout(t, ms)
	defer ownerLogout(t, fs)
	readerLive := true
	defer func() {
		if readerLive {
			ownerLogout(t, rs)
		}
	}()
	a := ownerAccess(t, ownerNodeA)
	auth := access.ResolveRequest{Bearer: ms.Token}
	org, err := a.CreateOrganization(ctx, auth, access.NameRequest{Name: "Task9 owned Audit chain"})
	if err != nil {
		t.Fatal("Audit fixture organization failed")
	}
	if _, err = a.SetOrganizationMembership(ctx, auth, org.OrganizationID, access.MembershipRequest{HumanID: reader.ID, Role: "member"}); err != nil {
		t.Fatal("Audit fixture reader membership failed")
	}
	scope := storage.Scope{WorkspaceID: "audit-owned-" + ownerNonce(t), ProjectID: "project"}
	_, revision := ownerSeedRevision(t, ownerPool(t, "E06_PROJECTS_DATABASE_URL"), scope, ownerSuccessSource)
	if _, err = ownerImport(t, ownerNodeA, org.OrganizationID, scope, revision, manager.ID); err != nil {
		t.Fatal("Audit verified scope import failed")
	}
	complete := access.PermissionScope{OrganizationID: org.OrganizationID, WorkspaceID: scope.WorkspaceID, ProjectID: scope.ProjectID}
	ownerBind(t, a, auth, complete, reader.ID, "owner", false)
	ownerPermission(t, ms, scope, "audit.read", true)
	ownerPermission(t, rs, scope, "audit.read", true)
	managerIdentity := ownerSessionIdentity(t, ms)
	outage := startAuditOutage(t, complete)
	keyFacts, keySecrets := ownerAuditKeyActions(t, a, auth, complete)
	consent := ownerAuditConsentFlow(t, manager)
	// New create/retry uses the canonical private key; only one grant fact may exist.
	grant := guestIssue(t, guestBrowserFixture{Browser: browserFixture{Scope: scope}}, ms, time.Now().UTC().Add(time.Hour))
	release := ownerRelease(t, ms.Token, scope, revision)
	pc, err := projects.NewClient(ownerProjects, ms.Token, guestHTTP(nil))
	if err != nil {
		t.Fatal("Audit Projects client failed")
	}
	release, err = pc.SetPublication(ctx, release.ReleaseID, projects.PublicationRequest{Scope: scope, ExpectedVersion: release.PublicationVersion, Published: true})
	if err != nil {
		t.Fatal("Audit release publish failed")
	}
	gc, err := generation.NewClient(ownerGeneration, ms.Token, guestHTTP(nil))
	if err != nil {
		t.Fatal("Audit Generation client failed")
	}
	privateKey := "audit-private-idempotency-" + ownerNonce(t)
	selection := generation.RenderSelectionRequest{Scope: scope, ReleaseID: release.ReleaseID, Parameters: json.RawMessage(`{}`)}
	admission, err := gc.CreateRender(ctx, privateKey, selection)
	if err != nil {
		t.Fatal("Audit actual render admission failed")
	}
	job := ownerJob(t, gc, admission.Job.JobID)
	if job.Status != generation.Succeeded || job.Artifact == nil {
		t.Fatal("Audit actual render did not publish an artifact")
	}
	retry, err := gc.CreateRender(ctx, privateKey, selection)
	if err != nil || retry.Job.JobID != job.JobID {
		t.Fatal("Audit render retry changed the logical job")
	}
	status, pdf := ownerRequest(t, guestHTTP(nil), "GET", ownerStorage+"/v1/artifacts/"+job.Artifact.ArtifactID+"/content", ms.Token, "", "", nil)
	if status != 200 || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("Audit actual authorized download failed")
	}
	expected := map[audit.ActionName]string{
		audit.ActionGrantCreate:         grant.Grant.GrantID,
		audit.ActionReleasePublish:      release.ReleaseID,
		audit.ActionGenerationAdmission: job.JobID,
		audit.ActionGenerationTerminal:  job.JobID,
		audit.ActionDownloadAuthorized:  job.Artifact.ArtifactID,
	}
	for action, resource := range keyFacts {
		expected[action] = resource
	}
	if outage != nil {
		consent.pending(t)
		outage.IdentityEventID = consent.EventID
		outage.committed(t, expected, managerIdentity)
	}
	base := os.Getenv("E06_AUDIT_URL")
	if base == "" {
		t.Fatal("owned Audit HTTP setting missing")
	}
	client, err := audit.NewClient(base, "ory_kratos_session", task08Cookie, guestHTTP(nil))
	if err != nil {
		t.Fatal("Audit public client configuration failed")
	}
	query := audit.HistoryQuery{Scope: complete, Limit: 100}
	deadline := time.Now().Add(30 * time.Second)
	var page audit.HistoryPage
	for {
		page, err = client.Actions(ctx, auth, query)
		if err != nil {
			t.Fatal("actual authorized Audit history failed")
		}
		found := map[audit.ActionName]int{}
		for _, item := range page.Items {
			if resource, ok := expected[item.Action.Action]; ok && item.Action.Resource.ID == resource {
				found[item.Action.Action]++
			}
		}
		all := true
		for action := range expected {
			if found[action] == 0 {
				all = false
			}
			if found[action] > 1 {
				t.Fatalf("logical action duplicated: %s", action)
			}
		}
		if all {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("actual four-owner Audit facts did not converge")
		}
		time.Sleep(100 * time.Millisecond)
	}
	secrets := []string{ms.Token, rs.Token, fs.Token, manager.Password, reader.Password, foreign.Password, manager.Email, grant.Secret, privateKey, ownerPrivateSentinel}
	secrets = append(secrets, keySecrets...)
	secrets = append(secrets, consent.secrets...)
	for _, item := range page.Items {
		encoded, _ := json.Marshal(item)
		for _, secret := range secrets {
			if secret != "" && bytes.Contains(encoded, []byte(secret)) {
				t.Fatal("private input entered public Audit history")
			}
		}
		if item.Scope != complete || !events.ValidAuditID(item.EventID) || !events.ValidAuditID(item.Action.OperationID) {
			t.Fatal("Audit fact scope/server IDs invalid")
		}
		if resource, ok := expected[item.Action.Action]; ok && item.Action.Resource.ID == resource {
			if item.Action.Actor != managerIdentity.Principal || item.Action.Authority != managerIdentity.Authority {
				t.Fatal("owner fact lost original initiating identity/version")
			}
			for name, revision := range map[audit.ActionName]string{audit.ActionKeyCreate: "1", audit.ActionKeyRotate: "2", audit.ActionKeyRevoke: "3"} {
				if item.Action.Action == name && item.Action.Revision != revision {
					t.Fatal("actual key lifecycle fact lost exact resource version")
				}
			}
		}
	}
	ownerAuditMembershipFact(t, org.OrganizationID, reader.ID, managerIdentity, secrets)
	consent.applied(t, secrets)
	t.Run("ActualCapturedOwnerRedeliveryAndLostAcknowledgement", func(t *testing.T) {
		ownerAuditTransport(t, page.Items, expected, complete)
	})
	// A second real registry project gives an authorized sideways control without
	// creating another compilation or touching the separate browser fixture.
	other, err := a.CreateProject(ctx, auth, scope.WorkspaceID, access.ProjectRequest{Name: "Audit scope marker", Kind: "personal", OwnerID: manager.ID})
	if err != nil {
		t.Fatal("second owned Audit scope creation failed")
	}
	otherScope := access.PermissionScope{OrganizationID: org.OrganizationID, WorkspaceID: scope.WorkspaceID, ProjectID: other.ProjectID}
	otherContentScope := storage.Scope{WorkspaceID: scope.WorkspaceID, ProjectID: other.ProjectID}
	ownerPermission(t, ms, otherContentScope, "audit.read", true)
	ownerPermission(t, rs, otherContentScope, "audit.read", false)
	deadline = time.Now().Add(20 * time.Second)
	for {
		marker, err := client.Actions(ctx, auth, audit.HistoryQuery{Scope: otherScope, Limit: 100})
		if err != nil {
			t.Fatal("authorized second Audit scope history failed")
		}
		found := false
		for _, item := range marker.Items {
			if item.Scope != otherScope {
				t.Fatal("second Audit scope included sideways history")
			}
			if item.Action.Action == audit.ActionProjectCreate && item.Action.Resource.ID == other.ProjectID {
				found = true
			}
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second owned Audit scope marker did not converge")
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Opaque cursors bind the authorized reader, filters, and exact scope. They
	// never substitute for a fresh current permission check on the next page.
	query.Limit = 1
	first, err := client.Actions(ctx, auth, query)
	if err != nil || first.NextCursor == "" {
		t.Fatal("owned Audit keyset first page unavailable")
	}
	query.Cursor = first.NextCursor
	next, err := client.Actions(ctx, auth, query)
	if err != nil || len(next.Items) != 1 || next.Items[0].EventID == first.Items[0].EventID {
		t.Fatal("Audit keyset repeated or lost next fact")
	}
	if _, err = client.Actions(ctx, access.ResolveRequest{Bearer: rs.Token}, query); !errors.Is(err, access.ErrInvalid) {
		t.Fatal("authorized different reader reused opaque cursor")
	}
	if _, err = client.Actions(ctx, access.ResolveRequest{Bearer: fs.Token}, query); !errors.Is(err, access.ErrDenied) {
		t.Fatal("foreign reader reused Audit history cursor")
	}
	changed := query
	changed.Scope = otherScope
	if _, err = client.Actions(ctx, auth, changed); !errors.Is(err, access.ErrInvalid) {
		t.Fatal("cursor crossed between two currently authorized exact scopes")
	}
	if _, err = client.Actions(ctx, access.ResolveRequest{Bearer: rs.Token}, changed); !errors.Is(err, access.ErrDenied) {
		t.Fatal("project-scoped reader expanded sideways with a cursor")
	}
	changed = query
	changed.ResourceID = first.Items[0].Action.Resource.ID
	if _, err = client.Actions(ctx, auth, changed); !errors.Is(err, access.ErrInvalid) {
		t.Fatal("changed Audit filters reused cursor")
	}
	changed = query
	changed.Cursor = strings.Repeat("A", len(query.Cursor))
	if _, err = client.Actions(ctx, auth, changed); !errors.Is(err, access.ErrInvalid) {
		t.Fatal("tampered opaque Audit cursor accepted")
	}
	readerFirst, err := client.Actions(ctx, access.ResolveRequest{Bearer: rs.Token}, audit.HistoryQuery{Scope: complete, Limit: 1})
	if err != nil || readerFirst.NextCursor == "" {
		t.Fatal("authorized reader cursor unavailable")
	}
	ownerBind(t, a, auth, complete, reader.ID, "owner", true)
	ownerPermission(t, rs, scope, "audit.read", false)
	if _, err = client.Actions(ctx, access.ResolveRequest{Bearer: rs.Token}, audit.HistoryQuery{Scope: complete, Limit: 1, Cursor: readerFirst.NextCursor}); !errors.Is(err, access.ErrDenied) {
		t.Fatal("cursor bypassed current reader permission revocation")
	}
	ownerLogout(t, rs)
	readerLive = false
	if _, err = client.Actions(ctx, access.ResolveRequest{Bearer: rs.Token}, query); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("revoked provider session read Audit history")
	}
	if _, err = client.Actions(ctx, access.ResolveRequest{Bearer: os.Getenv("E06_AUDIT_OPERATOR_TOKEN")}, query); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("internal operator credential authenticated as public reader")
	}
	t.Log("Actual Ory/Hydra/Access/Projects/Generation/Storage chain: membership/key/grant/consent/release/job/download facts, logical retries, original identity, private-input exclusion, opaque cursor and current reader denial passed")
}
