package e06

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	access "github.com/leafrun/leafrun-contracts/access/v1"
	projects "github.com/leafrun/leafrun-contracts/projects/v1"
	storage "github.com/leafrun/leafrun-contracts/storage/v1"
)

// A separate Task7 identity and namespace; never reads the retained Task5 fixture.
// The private file contains only fixture credentials and safe resource IDs.
func TestOAuthBrowserFixture(t *testing.T) {
	action := os.Getenv("E06_OAUTH_BROWSER_ACTION")
	if action == "" {
		t.Skip("separate OAuth browser fixture setup not requested")
	}
	ownerEnabled(t)
	const path = "/evidence/oauth-browser-fixture.json"
	if action == "cleanup" {
		data, err := os.ReadFile(path)
		var f browserFixture
		if err != nil || json.Unmarshal(data, &f) != nil || f.Human.ID == "" {
			t.Fatal("OAuth browser fixture unavailable")
		}
		status, _ := ownerRequest(t, ownerHTTP(nil), "DELETE", os.Getenv("E06_KRATOS_ADMIN_URL")+"/admin/identities/"+f.Human.ID, "", "", "", nil)
		if status != 204 || os.Remove(path) != nil {
			t.Fatal("OAuth browser fixture cleanup failed")
		}
		return
	}
	if action != "seed" {
		t.Fatal("OAuth fixture action invalid")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("Existing OAuth fixture preserved; cleanup before reseeding")
	}
	h := ownerHuman{Email: "browser-task7-" + ownerNonce(t) + "@e06.example.test", Password: ownerNonce(t) + "aA!"}
	status, body := ownerRequest(t, ownerHTTP(nil), "POST", os.Getenv("E06_KRATOS_ADMIN_URL")+"/admin/identities", "", "", "application/json", map[string]any{"schema_id": "default", "state": "active", "traits": map[string]string{"email": h.Email}, "credentials": map[string]any{"password": map[string]any{"config": map[string]string{"password": h.Password}}}})
	var out struct {
		ID string `json:"id"`
	}
	if status != 201 || json.Unmarshal(body, &out) != nil || out.ID == "" {
		t.Fatal("OAuth browser identity creation failed")
	}
	h.ID = out.ID
	prepared := false
	t.Cleanup(func() {
		if !prepared {
			ownerRequest(t, ownerHTTP(nil), "DELETE", os.Getenv("E06_KRATOS_ADMIN_URL")+"/admin/identities/"+h.ID, "", "", "", nil)
		}
	})
	session := ownerLogin(t, h)
	a := ownerAccess(t, ownerNodeA)
	org, err := a.CreateOrganization(context.Background(), access.ResolveRequest{Bearer: session.Token}, access.NameRequest{Name: "Task7 actual OAuth browser"})
	if err != nil {
		t.Fatal("OAuth fixture organization setup failed")
	}
	scope := storage.Scope{WorkspaceID: "oauth-browser-" + ownerNonce(t), ProjectID: "project"}
	_, revision := ownerSeedRevision(t, ownerPool(t, "E06_PROJECTS_DATABASE_URL"), scope, ownerSuccessSource)
	if _, err = ownerImport(t, ownerNodeA, org.OrganizationID, scope, revision, h.ID); err != nil {
		t.Fatal("OAuth fixture scope setup failed")
	}
	ownerPermission(t, session, scope, "project.publish", true)
	release := ownerRelease(t, session.Token, scope, revision)
	c, _ := projects.NewClient(ownerProjects, session.Token, ownerHTTP(nil))
	release, err = c.SetPublication(context.Background(), release.ReleaseID, projects.PublicationRequest{Scope: scope, ExpectedVersion: release.PublicationVersion, Published: true})
	if err != nil {
		t.Fatal("OAuth fixture publication failed")
	}
	data, _ := json.Marshal(browserFixture{h, scope, release.ReleaseID, release.PublicationVersion})
	if os.WriteFile(path, data, 0600) != nil {
		t.Fatal("OAuth fixture private write failed")
	}
	ownerLogout(t, session)
	prepared = true
	t.Log("Separate Task7 browser identity, owner relation and published resource prepared; native setup session revoked")
}
