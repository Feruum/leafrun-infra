package e06

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	access "github.com/leafrun/leafrun-contracts/access/v1"
	audit "github.com/leafrun/leafrun-contracts/audit/v1"
	generation "github.com/leafrun/leafrun-contracts/generation/v1"
	projects "github.com/leafrun/leafrun-contracts/projects/v1"
	storage "github.com/leafrun/leafrun-contracts/storage/v1"
)

// A new project uses only public owner APIs. No legacy revision/registry SQL
// seed is used in this MVP: the integration client captures a real Git commit.
func TestRealMVPFromGitHumanAndBotParameters(t *testing.T) {
	ownerEnabled(t)
	ctx := context.Background()
	human := ownerIdentity(t)
	session := ownerLogin(t, human)
	sessionRevoked := false
	defer func() {
		if !sessionRevoked {
			ownerLogout(t, session)
		}
	}()
	a := ownerAccess(t, ownerNodeA)
	auth := access.ResolveRequest{Bearer: session.Token}
	org, err := a.CreateOrganization(ctx, auth, access.NameRequest{Name: "MVP Git origin"})
	if err != nil {
		t.Fatal("MVP organization creation failed")
	}
	workspace, err := a.CreateWorkspace(ctx, auth, org.OrganizationID, access.NameRequest{Name: "MVP workspace"})
	if err != nil {
		t.Fatal("MVP workspace creation failed")
	}
	project, err := a.CreateProject(ctx, auth, workspace.WorkspaceID, access.ProjectRequest{Name: "MVP project", Kind: "personal", OwnerID: human.ID})
	if err != nil {
		t.Fatal("MVP project creation failed")
	}
	scope := storage.Scope{WorkspaceID: workspace.WorkspaceID, ProjectID: project.ProjectID}
	complete := access.PermissionScope{OrganizationID: org.OrganizationID, WorkspaceID: scope.WorkspaceID, ProjectID: scope.ProjectID}
	ownerPermission(t, session, scope, "generation.create", true)
	snapshot, commit := mvpGitSnapshot(t, scope)
	pc, err := projects.NewClient(ownerProjects, session.Token, ownerHTTP(nil))
	if err != nil {
		t.Fatal("MVP Projects client failed")
	}
	revision, err := pc.RegisterRevision(ctx, projects.RegisterRevisionRequest{Scope: scope, Revision: commit, SnapshotID: snapshot.SnapshotID, IdempotencyKey: "mvp-revision-" + ownerNonce(t)})
	if err != nil || revision.ContentID != snapshot.Manifest.ID {
		t.Fatal("MVP public Git revision registration failed")
	}
	release := ownerRelease(t, session.Token, scope, revision.Revision)
	release, err = pc.SetPublication(ctx, release.ReleaseID, projects.PublicationRequest{Scope: scope, ExpectedVersion: release.PublicationVersion, Published: true})
	if err != nil {
		t.Fatal("MVP release publication failed")
	}
	if _, err = pc.Form(ctx, release.ReleaseID); err != nil {
		t.Fatal("MVP published form unavailable")
	}
	bot, err := a.CreateBot(ctx, auth, scope.WorkspaceID, scope.ProjectID, access.CreateBotRequest{OperationKey: "mvp-bot-" + ownerNonce(t), Name: "MVP generator"})
	if err != nil {
		t.Fatal("MVP bot creation failed")
	}
	if _, err = a.Bind(ctx, auth, complete, access.BindingRequest{SubjectKind: "bot", SubjectID: bot.BotID, Relation: "generate"}, false); err != nil {
		t.Fatal("MVP bot binding failed")
	}
	key, err := a.IssueBotKey(ctx, auth, bot.BotID, access.BotKeyRequest{OperationKey: "mvp-key-" + ownerNonce(t), Operations: botGenerateOps})
	if err != nil || key.Secret == "" {
		t.Fatal("MVP bot key issuance failed")
	}
	ownerPermission(t, ownerSession{Token: key.Secret}, scope, "generation.create", true)
	botProjects, err := projects.NewClient(ownerProjects, key.Secret, ownerHTTP(nil))
	if err != nil {
		t.Fatal("MVP bot Projects client failed")
	}
	if _, err = botProjects.Form(ctx, release.ReleaseID); err != nil {
		t.Fatal("MVP generate-only bot form unavailable")
	}
	if _, err = botProjects.GetRevision(ctx, scope, revision.Revision); err == nil {
		t.Fatal("MVP generate-only bot received private source")
	}
	sharedKey := "mvp-actor-namespace-" + ownerNonce(t)
	jobs := make([]generation.Job, 0, 2)
	for _, actor := range []struct{ name, token string }{{"human", session.Token}, {"bot", key.Secret}} {
		gc, e := generation.NewClient(ownerGeneration, actor.token, ownerHTTP(nil))
		if e != nil {
			t.Fatal("MVP Generation client failed")
		}
		parameters, _ := json.Marshal(map[string]string{"message": "from " + actor.name})
		selection := generation.RenderSelectionRequest{Scope: scope, ReleaseID: release.ReleaseID, Parameters: parameters}
		admitted, e := gc.CreateRender(ctx, sharedKey, selection)
		if e != nil {
			t.Fatalf("MVP %s render admission failed", actor.name)
		}
		job := ownerJob(t, gc, admitted.Job.JobID)
		if job.Status != generation.Succeeded || job.Artifact == nil {
			t.Fatalf("MVP %s real Temporal render failed", actor.name)
		}
		retry, e := gc.CreateRender(ctx, sharedKey, selection)
		if e != nil || retry.Job.JobID != job.JobID || retry.Job.Artifact == nil || !retry.Job.Artifact.ExpiresAt.Equal(job.Artifact.ExpiresAt) {
			t.Fatal("MVP retry changed job or original PDF expiry")
		}
		status, pdf := ownerRequest(t, ownerHTTP(nil), "GET", ownerStorage+"/v1/artifacts/"+job.Artifact.ArtifactID+"/content", session.Token, "", "", nil)
		sum := sha256.Sum256(pdf)
		if status != 200 || !bytes.HasPrefix(pdf, []byte("%PDF-")) || hex.EncodeToString(sum[:]) != job.Artifact.SHA256 {
			t.Fatal("MVP authenticated exact PDF download failed")
		}
		status, body := ownerRequest(t, ownerHTTP(nil), "GET", ownerStorage+"/v1/artifacts/"+job.Artifact.ArtifactID, session.Token, "", "", nil)
		var metadata storage.Artifact
		if status != 200 || json.Unmarshal(body, &metadata) != nil || metadata.ExpiresAt.Sub(metadata.CreatedAt) != 24*time.Hour {
			t.Fatal("MVP first-finalize PDF TTL is not fixed at24h")
		}
		var versioned bool
		if ownerPool(t, "E06_STORAGE_DATABASE_URL").QueryRow(ctx, `SELECT version<>'' FROM storage_objects WHERE id=$1 AND workspace=$2 AND project=$3`, job.Artifact.ArtifactID, scope.WorkspaceID, scope.ProjectID).Scan(&versioned) != nil || !versioned {
			t.Fatal("MVP PDF is not bound to a concrete S3 version")
		}
		jobs = append(jobs, job)
	}
	if jobs[0].JobID == jobs[1].JobID {
		t.Fatal("MVP idempotency namespace crossed human and bot actors")
	}
	status, deniedBody := ownerRequest(t, ownerHTTP(nil), "GET", ownerStorage+"/v1/artifacts/"+jobs[1].Artifact.ArtifactID+"/content", key.Secret, "", "", nil)
	if status != 404 || bytes.HasPrefix(deniedBody, []byte("%PDF-")) {
		t.Fatalf("MVP generate-only bot download must conceal the artifact without PDF bytes; status=%d", status)
	}
	ac, err := audit.NewClient("http://audit-api:8199", "ory_kratos_session", task08Cookie, ownerHTTP(nil))
	if err != nil {
		t.Fatal("MVP Audit client failed")
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		page, e := ac.Actions(ctx, auth, audit.HistoryQuery{Scope: complete, Limit: 100})
		if e != nil {
			t.Fatal("MVP authorized Audit history failed")
		}
		seen := map[string]bool{}
		for _, item := range page.Items {
			if item.Action.Action == audit.ActionGenerationTerminal {
				seen[item.Action.Resource.ID] = true
			}
		}
		if seen[jobs[0].JobID] && seen[jobs[1].JobID] {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("MVP original human/bot results did not reach Audit")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if os.Getenv("E06_REPLICA_INTEGRATION") == "1" {
		endpoints := []string{
			ownerGeneration + "/v1/jobs/" + jobs[0].JobID,
			"http://generation-api-replica:8080/v1/jobs/" + jobs[0].JobID,
			ownerProjects + "/v1/releases/" + release.ReleaseID + "/form",
			"http://projects-api-replica:8084/v1/releases/" + release.ReleaseID + "/form",
			ownerStorage + "/v1/artifacts/" + jobs[0].Artifact.ArtifactID + "/content",
			"http://storage-api-replica:8081/v1/artifacts/" + jobs[0].Artifact.ArtifactID + "/content",
		}
		for _, endpoint := range endpoints {
			status, _ := ownerRequest(t, ownerHTTP(nil), "GET", endpoint, session.Token, "", "", nil)
			if status != 200 {
				t.Fatalf("MVP shared-owner replica read failed; status=%d", status)
			}
		}
		ownerLogout(t, session)
		sessionRevoked = true
		for _, endpoint := range endpoints {
			status, body := ownerRequest(t, ownerHTTP(nil), "GET", endpoint, session.Token, "", "", nil)
			if status != 401 || bytes.HasPrefix(body, []byte("%PDF-")) {
				t.Fatalf("MVP revoked session survived on an independent domain replica; status=%d", status)
			}
		}
		t.Log("Both replicas of Generation, Projects and Storage read shared owner state and rejected the exact revoked session; replica authorization used Access B")
	}
	t.Log("Real local Git commit -> Core capture/restore/frozen external fonts -> public Projects revision/release/form -> human+generate-only bot parameters -> Temporal jobs -> concrete versioned S3 PDFs -> authenticated fixed-TTL download -> Audit; actor namespaces remained separate")
}
