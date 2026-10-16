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

type browserFixture struct {
	Human              ownerHuman    `json:"human"`
	Scope              storage.Scope `json:"scope"`
	ReleaseID          string        `json:"releaseId"`
	PublicationVersion int64         `json:"publicationVersion"`
}

// This opt-in setup creates a separate identity for an actual browser login.
// The private fixture contains no provider cookie or native session token.
func TestRealBrowserFixture(t *testing.T) {
	action := os.Getenv("E06_BROWSER_ACTION")
	if action == "" {
		t.Skip("separate browser fixture setup not requested")
	}
	ownerEnabled(t)
	const path = "/evidence/browser-fixture.json"
	if action == "cleanup" {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal("browser cleanup fixture unavailable")
		}
		var f browserFixture
		if json.Unmarshal(data, &f) != nil || f.Human.ID == "" {
			t.Fatal("browser cleanup fixture invalid")
		}
		status, _ := ownerRequest(t, ownerHTTP(nil), "DELETE", os.Getenv("E06_KRATOS_ADMIN_URL")+"/admin/identities/"+f.Human.ID, "", "", "", nil)
		if status != 204 {
			t.Fatal("browser identity cleanup failed")
		}
		if os.Remove(path) != nil {
			t.Fatal("browser credential fixture removal failed")
		}
		return
	}
	h := ownerHuman{Email: "browser-task5-" + ownerNonce(t) + "@e06.example.test", Password: ownerNonce(t) + "aA!"}
	status, body := ownerRequest(t, ownerHTTP(nil), "POST", os.Getenv("E06_KRATOS_ADMIN_URL")+"/admin/identities", "", "", "application/json", map[string]any{"schema_id": "default", "state": "active", "traits": map[string]string{"email": h.Email}, "credentials": map[string]any{"password": map[string]any{"config": map[string]string{"password": h.Password}}}})
	var created struct {
		ID string `json:"id"`
	}
	if status != 201 || json.Unmarshal(body, &created) != nil || created.ID == "" {
		t.Fatal("browser identity creation failed")
	}
	h.ID = created.ID
	prepared := false
	t.Cleanup(func() {
		if !prepared {
			ownerRequest(t, ownerHTTP(nil), "DELETE", os.Getenv("E06_KRATOS_ADMIN_URL")+"/admin/identities/"+h.ID, "", "", "", nil)
		}
	})
	session := ownerLogin(t, h)
	a := ownerAccess(t, ownerNodeA)
	org, err := a.CreateOrganization(context.Background(), access.ResolveRequest{Bearer: session.Token}, access.NameRequest{Name: "Task5 actual browser"})
	if err != nil {
		t.Fatal("browser organization setup failed")
	}
	scope := storage.Scope{WorkspaceID: "browser-" + ownerNonce(t), ProjectID: "project"}
	_, revision := ownerSeedRevision(t, ownerPool(t, "E06_PROJECTS_DATABASE_URL"), scope, ownerSuccessSource)
	if _, err = ownerImport(t, ownerNodeA, org.OrganizationID, scope, revision, h.ID); err != nil {
		t.Fatal("browser verified scope setup failed")
	}
	ownerPermission(t, session, scope, "project.publish", true)
	release := ownerRelease(t, session.Token, scope, revision)
	c, _ := projects.NewClient(ownerProjects, session.Token, ownerHTTP(nil))
	release, err = c.SetPublication(context.Background(), release.ReleaseID, projects.PublicationRequest{Scope: scope, ExpectedVersion: release.PublicationVersion, Published: true})
	if err != nil {
		t.Fatal("browser publication setup failed")
	}
	data, _ := json.Marshal(browserFixture{h, scope, release.ReleaseID, release.PublicationVersion})
	if os.WriteFile(path, data, 0600) != nil {
		t.Fatal("private browser fixture write failed")
	}
	ownerLogout(t, session)
	prepared = true
	t.Log("separate browser identity and exact published binding prepared; native setup session revoked")
}
