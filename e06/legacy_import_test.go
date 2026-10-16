package e06

import (
	"context"
	"encoding/json"
	access "github.com/leafrun/leafrun-contracts/access/v1"
	proof "github.com/leafrun/leafrun-contracts/ownerproof/v1"
	storage "github.com/leafrun/leafrun-contracts/storage/v1"
	"os"
	"sync"
	"testing"
	"time"
)

func TestRealOwnerVerifiedLegacyImportAndCrossOrganizationRace(t *testing.T) {
	ownerEnabled(t)
	ctx := context.Background()
	ownerA, ownerB := ownerIdentity(t), ownerIdentity(t)
	sessionA, sessionB := ownerLogin(t, ownerA), ownerLogin(t, ownerB)
	a, b := ownerAccess(t, ownerNodeA), ownerAccess(t, ownerNodeB)
	orgA, err := a.CreateOrganization(ctx, access.ResolveRequest{Bearer: sessionA.Token}, access.NameRequest{Name: "Verified legacy A"})
	if err != nil {
		t.Fatal("organization A failed")
	}
	orgB, err := b.CreateOrganization(ctx, access.ResolveRequest{Bearer: sessionB.Token}, access.NameRequest{Name: "Verified legacy B"})
	if err != nil {
		t.Fatal("organization B failed")
	}
	projectsPool, storagePool, accessPool := ownerPool(t, "E06_PROJECTS_DATABASE_URL"), ownerPool(t, "E06_STORAGE_DATABASE_URL"), ownerPool(t, "E06_ACCESS_DATABASE_URL")
	scope := storage.Scope{WorkspaceID: "legacy workspace-é-" + ownerNonce(t), ProjectID: "repeated-project"}
	snapshot, revision := ownerSeedRevision(t, projectsPool, scope, "#set text(font: \"Fira Sans\")\nVerified legacy domain")
	input := proof.ImportRequest{OrganizationID: orgA.OrganizationID, Scope: scope, Revision: revision, Kind: "personal", OwnerID: ownerA.ID, IdempotencyKey: "verified-import-" + ownerNonce(t)}
	status, _ := ownerRequest(t, ownerHTTP(nil), "POST", ownerNodeA+"/internal/v1/scopes/import", sessionA.Token, "", "application/json", input)
	if status != 401 {
		t.Fatal("public organization owner claimed domain scope")
	}
	wrong := input
	wrong.Scope.WorkspaceID = "foreign-" + ownerNonce(t)
	status, _ = ownerRequest(t, ownerHTTP(nil), "POST", ownerNodeA+"/internal/v1/scopes/import", os.Getenv("E06_IMPORT_OPERATOR_TOKEN"), "", "application/json", wrong)
	if status != 404 {
		t.Fatalf("foreign domain witness status %d", status)
	}
	// An observed Storage catalog wait must become dependency uncertainty and
	// leave no workspace/project claim or policy epoch mutation in Access.
	lock, err := storagePool.Begin(ctx)
	if err != nil {
		t.Fatal("catalog lock failed")
	}
	defer lock.Rollback(ctx)
	if _, err = lock.Exec(ctx, `LOCK TABLE storage_snapshots IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal("catalog lock acquisition failed")
	}
	done := make(chan int, 1)
	go func() {
		status, _ := ownerRequest(t, ownerHTTP(nil), "POST", ownerNodeA+"/internal/v1/scopes/import", os.Getenv("E06_IMPORT_OPERATOR_TOKEN"), "", "application/json", input)
		done <- status
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		if err = storagePool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%JOIN storage_snapshots%')`).Scan(&waiting); err != nil {
			t.Fatal("owner wait observation failed")
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("real Storage verifier did not reach observed catalog wait")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status = <-done; status != 503 {
		t.Fatalf("blocked real owner evidence status %d", status)
	}
	if err = lock.Commit(ctx); err != nil {
		t.Fatal("catalog unlock failed")
	}
	var claims int
	if err = accessPool.QueryRow(ctx, `SELECT count(*) FROM access_workspaces WHERE workspace_id=$1`, scope.WorkspaceID).Scan(&claims); err != nil || claims != 0 {
		t.Fatal("uncertain owner evidence mutated registry")
	}
	status, body := ownerRequest(t, ownerHTTP(nil), "POST", ownerNodeA+"/internal/v1/scopes/import", os.Getenv("E06_IMPORT_OPERATOR_TOKEN"), "", "application/json", input)
	var imported proof.ImportResult
	if status != 200 || json.Unmarshal(body, &imported) != nil || imported.Binding.Scope != scope || imported.Binding.SnapshotID != snapshot.SnapshotID || imported.Binding.ContentID != snapshot.Manifest.ID {
		t.Fatalf("real owner verified import status %d", status)
	}
	var before, after int64
	if err = accessPool.QueryRow(ctx, `SELECT desired_epoch FROM access_policy_epochs WHERE organization_id=$1`, orgA.OrganizationID).Scan(&before); err != nil {
		t.Fatal("policy intent unavailable")
	}
	status, _ = ownerRequest(t, ownerHTTP(nil), "POST", ownerNodeB+"/internal/v1/scopes/import", os.Getenv("E06_IMPORT_OPERATOR_TOKEN"), "", "application/json", input)
	if status != 200 {
		t.Fatalf("cross-node exact import replay %d", status)
	}
	if err = accessPool.QueryRow(ctx, `SELECT desired_epoch FROM access_policy_epochs WHERE organization_id=$1`, orgA.OrganizationID).Scan(&after); err != nil || after != before {
		t.Fatal("import replay changed policy epoch")
	}
	changed := input
	changed.OrganizationID = orgB.OrganizationID
	changed.OwnerID = ownerB.ID
	status, _ = ownerRequest(t, ownerHTTP(nil), "POST", ownerNodeB+"/internal/v1/scopes/import", os.Getenv("E06_IMPORT_OPERATOR_TOKEN"), "", "application/json", changed)
	if status != 409 {
		t.Fatalf("changed immutable import status %d", status)
	}
	// Distinct projects in one previously absent workspace still have one tenant.
	workspace := "race-workspace-" + ownerNonce(t)
	raceScopes := []storage.Scope{{WorkspaceID: workspace, ProjectID: "a"}, {WorkspaceID: workspace, ProjectID: "b"}}
	revisions := make([]string, 2)
	for i, scope := range raceScopes {
		_, revisions[i] = ownerSeedRevision(t, projectsPool, scope, "Imported race evidence")
	}
	statuses := make([]int, 2)
	var wg sync.WaitGroup
	for i := range raceScopes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			org, owner, node := orgA.OrganizationID, ownerA.ID, ownerNodeA
			if i == 1 {
				org, owner, node = orgB.OrganizationID, ownerB.ID, ownerNodeB
			}
			in := proof.ImportRequest{OrganizationID: org, Scope: raceScopes[i], Revision: revisions[i], Kind: "personal", OwnerID: owner, IdempotencyKey: "race-" + ownerNonce(t)}
			statuses[i], _ = ownerRequest(t, ownerHTTP(nil), "POST", node+"/internal/v1/scopes/import", os.Getenv("E06_IMPORT_OPERATOR_TOKEN"), "", "application/json", in)
		}(i)
	}
	wg.Wait()
	if !((statuses[0] == 200 && statuses[1] == 409) || (statuses[0] == 409 && statuses[1] == 200)) {
		t.Fatalf("cross-node import race outcomes %v", statuses)
	}
	t.Log("actual Projects+Storage evidence, observed verifier lock uncertainty, public claim denial, exact A/B replay and same-workspace cross-org race passed")
}
