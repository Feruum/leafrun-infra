package e06

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	access "github.com/leafrun/leafrun-contracts/access/v1"
	generation "github.com/leafrun/leafrun-contracts/generation/v1"
	projects "github.com/leafrun/leafrun-contracts/projects/v1"
	storage "github.com/leafrun/leafrun-contracts/storage/v1"
)

type guestBrowserFixture struct {
	Browser      browserFixture `json:"browser"`
	GrantID      string         `json:"grantId"`
	GrantVersion int64          `json:"grantVersion"`
	Secret       string         `json:"secret"`
	JobID        string         `json:"jobId"`
	ArtifactID   string         `json:"artifactId"`
}

// This opt-in fixture belongs exclusively to Task8. Link disclosure is retained
// only in the ACL-protected evidence directory; receipts never contain it.
func TestGuestBrowserFixture(t *testing.T) {
	action := os.Getenv("E06_GUEST_BROWSER_ACTION")
	if action == "" {
		t.Skip("separate guest browser fixture not requested")
	}
	ownerEnabled(t)
	const path = "/evidence/guest-browser-fixture.json"
	if action == "cleanup" {
		data, err := os.ReadFile(path)
		var f guestBrowserFixture
		if err != nil || json.Unmarshal(data, &f) != nil || f.Browser.Human.ID == "" {
			t.Fatal("guest fixture unavailable")
		}
		status, _ := ownerRequest(t, ownerHTTP(nil), "DELETE", os.Getenv("E06_KRATOS_ADMIN_URL")+"/admin/identities/"+f.Browser.Human.ID, "", "", "", nil)
		if status != 204 || os.Remove(path) != nil {
			t.Fatal("guest fixture cleanup failed")
		}
		return
	}
	if action != "seed" {
		t.Fatal("invalid guest fixture action")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("existing guest fixture preserved")
	}
	h := ownerHuman{Email: "browser-task8-" + ownerNonce(t) + "@e06.example.test", Password: ownerNonce(t) + "aA!"}
	status, data := ownerRequest(t, ownerHTTP(nil), "POST", os.Getenv("E06_KRATOS_ADMIN_URL")+"/admin/identities", "", "", "application/json", map[string]any{"schema_id": "default", "state": "active", "traits": map[string]string{"email": h.Email}, "credentials": map[string]any{"password": map[string]any{"config": map[string]string{"password": h.Password}}}})
	var created struct {
		ID string `json:"id"`
	}
	if status != 201 || json.Unmarshal(data, &created) != nil || created.ID == "" {
		t.Fatal("guest issuer setup failed")
	}
	h.ID = created.ID
	prepared := false
	t.Cleanup(func() {
		if !prepared {
			ownerRequest(t, ownerHTTP(nil), "DELETE", os.Getenv("E06_KRATOS_ADMIN_URL")+"/admin/identities/"+h.ID, "", "", "", nil)
		}
	})
	manager := ownerLogin(t, h)
	defer ownerLogout(t, manager)
	org, err := ownerAccess(t, ownerNodeA).CreateOrganization(context.Background(), access.ResolveRequest{Bearer: manager.Token}, access.NameRequest{Name: "Task8 actual guest browser"})
	if err != nil {
		t.Fatal("guest organization setup failed")
	}
	scope := storage.Scope{WorkspaceID: "guest-browser-" + ownerNonce(t), ProjectID: "project"}
	_, revision := ownerSeedRevision(t, ownerPool(t, "E06_PROJECTS_DATABASE_URL"), scope, ownerSuccessSource)
	if _, err = ownerImport(t, ownerNodeA, org.OrganizationID, scope, revision, h.ID); err != nil {
		t.Fatal("guest project import failed")
	}
	ownerPermission(t, manager, scope, "project.publish", true)
	release := ownerRelease(t, manager.Token, scope, revision)
	pc, _ := projects.NewClient(ownerProjects, manager.Token, ownerHTTP(nil))
	release, err = pc.SetPublication(context.Background(), release.ReleaseID, projects.PublicationRequest{Scope: scope, ExpectedVersion: release.PublicationVersion, Published: true})
	if err != nil {
		t.Fatal("guest publication setup failed")
	}
	gc, _ := generation.NewClient(ownerGeneration, manager.Token, ownerHTTP(nil))
	admitted, err := gc.CreateRender(context.Background(), "guest-browser-"+ownerNonce(t), generation.RenderSelectionRequest{Scope: scope, ReleaseID: release.ReleaseID, Parameters: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal("guest historical render setup failed")
	}
	job := ownerJob(t, gc, admitted.Job.JobID)
	if job.Status != generation.Succeeded || job.Artifact == nil {
		t.Fatal("guest historical artifact setup failed")
	}
	status, data = ownerRequest(t, ownerHTTP(nil), "POST", ownerNodeA+"/v1/workspaces/"+scope.WorkspaceID+"/projects/"+scope.ProjectID+"/grants", manager.Token, ownerNodeA, "application/json", access.CreateGrantRequest{OperationKey: ownerGrantOperationKey(t), Role: "editor", ExpiresAt: time.Now().UTC().Add(2 * time.Hour)})
	var result struct {
		Grant struct {
			ID      string `json:"grantId"`
			Version int64  `json:"version"`
		} `json:"grant"`
		Secret     string `json:"secret"`
		Disclosure string `json:"disclosure"`
	}
	if status != 200 || json.Unmarshal(data, &result) != nil || result.Grant.ID == "" || result.Secret == "" || result.Disclosure != "first" {
		t.Fatalf("guest first grant disclosure failed: status=%d", status)
	}
	f := guestBrowserFixture{Browser: browserFixture{h, scope, release.ReleaseID, release.PublicationVersion}, GrantID: result.Grant.ID, GrantVersion: result.Grant.Version, Secret: result.Secret, JobID: job.JobID, ArtifactID: job.Artifact.ArtifactID}
	data, _ = json.Marshal(f)
	if os.WriteFile(path, data, 0600) != nil {
		t.Fatal("private guest fixture write failed")
	}
	prepared = true
	t.Log("Separate Task8 issuer, exact grant and historical owner resources prepared; native setup session revoked")
}
