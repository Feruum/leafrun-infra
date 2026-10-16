package e06

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	access "github.com/leafrun/leafrun-contracts/access/v1"
	generation "github.com/leafrun/leafrun-contracts/generation/v1"
	identity "github.com/leafrun/leafrun-contracts/identity/v1"
	projects "github.com/leafrun/leafrun-contracts/projects/v1"
	storage "github.com/leafrun/leafrun-contracts/storage/v1"
)

func TestRealFinalPublicationLockWaitRechecksLiveOriginalAuthority(t *testing.T) {
	ownerEnabled(t)
	ctx := context.Background()
	manager, generator, reader := ownerIdentity(t), ownerIdentity(t), ownerIdentity(t)
	managerSession, initialGenerator, readerSession := ownerLogin(t, manager), ownerLogin(t, generator), ownerLogin(t, reader)
	a, b := ownerAccess(t, ownerNodeA), ownerAccess(t, ownerNodeB)
	managerAuth := access.ResolveRequest{Bearer: managerSession.Token}
	org, err := a.CreateOrganization(ctx, managerAuth, access.NameRequest{Name: "Task5 final publication fence"})
	if err != nil {
		t.Fatal("final fence organization failed")
	}
	for _, person := range []ownerHuman{generator, reader} {
		if _, err = b.SetOrganizationMembership(ctx, managerAuth, org.OrganizationID, access.MembershipRequest{HumanID: person.ID, Role: "member"}); err != nil {
			t.Fatal("final fence membership failed")
		}
	}
	projectPool, generationPool := ownerPool(t, "E06_PROJECTS_DATABASE_URL"), ownerPool(t, "E06_GENERATION_DATABASE_URL")
	scope := storage.Scope{WorkspaceID: "final-fence-" + ownerNonce(t), ProjectID: "project"}
	_, revision := ownerSeedRevision(t, projectPool, scope, ownerSuccessSource)
	if _, err = ownerImport(t, ownerNodeA, org.OrganizationID, scope, revision, manager.ID); err != nil {
		t.Fatal("final fence import failed")
	}
	permissionScope := access.PermissionScope{OrganizationID: org.OrganizationID, WorkspaceID: scope.WorkspaceID, ProjectID: scope.ProjectID}
	ownerBind(t, a, managerAuth, permissionScope, generator.ID, "generate", false)
	ownerBind(t, a, managerAuth, permissionScope, reader.ID, "download", false)
	ownerPermission(t, initialGenerator, scope, "generation.create", true)
	release := ownerRelease(t, managerSession.Token, scope, revision)
	publisher, _ := projects.NewClient(ownerProjects, managerSession.Token, ownerHTTP(nil))
	release, err = publisher.SetPublication(ctx, release.ReleaseID, projects.PublicationRequest{Scope: scope, ExpectedVersion: release.PublicationVersion, Published: true})
	if err != nil {
		t.Fatal("final fence publication setup failed")
	}
	managerGeneration, _ := generation.NewClient(ownerGeneration, managerSession.Token, ownerHTTP(nil))
	selection := generation.RenderSelectionRequest{Scope: scope, ReleaseID: release.ReleaseID, Parameters: json.RawMessage(`{}`)}
	for _, mode := range []string{"capability-revoked-through-node-b", "original-session-revoked-new-session-cannot-rebind"} {
		t.Run(mode, func(t *testing.T) {
			session := ownerLogin(t, generator)
			originalIdentity := ownerSessionIdentity(t, session)
			ownerPermission(t, session, scope, "generation.publish", true)
			client, _ := generation.NewClient(ownerGeneration, session.Token, ownerHTTP(nil))
			key := "terminal-fence-" + ownerNonce(t)
			suffix := ownerNonce(t)
			function := "e06_terminal_fence_" + suffix
			trigger := "e06_terminal_trigger_" + suffix
			lockID := int64(time.Now().UnixNano()&0x3fffffff) + 1
			quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
			functionSQL := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $e06$ BEGIN IF NEW.idempotency_key=%s AND NEW.workspace_id=%s AND NEW.project_id=%s AND NEW.status='succeeded' AND OLD.status<>'succeeded' THEN PERFORM pg_advisory_xact_lock(%d::bigint); END IF; RETURN NEW; END $e06$`, function, quote(key), quote(scope.WorkspaceID), quote(scope.ProjectID), lockID)
			if _, err = generationPool.Exec(ctx, functionSQL); err != nil {
				t.Fatal("bounded final-lock fixture function failed")
			}
			if _, err = generationPool.Exec(ctx, "CREATE TRIGGER "+trigger+" BEFORE UPDATE ON generation_jobs FOR EACH ROW EXECUTE FUNCTION "+function+"()"); err != nil {
				t.Fatal("bounded final-lock fixture trigger failed")
			}
			defer func() {
				cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if _, e := generationPool.Exec(cleanup, "DROP TRIGGER IF EXISTS "+trigger+" ON generation_jobs"); e != nil {
					t.Error("final-lock fixture trigger cleanup failed")
				}
				if _, e := generationPool.Exec(cleanup, "DROP FUNCTION IF EXISTS "+function+"()"); e != nil {
					t.Error("final-lock fixture function cleanup failed")
				}
			}()
			holder, err := generationPool.Begin(ctx)
			if err != nil {
				t.Fatal("final lock holder failed")
			}
			defer holder.Rollback(context.Background())
			if _, err = holder.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockID); err != nil {
				t.Fatal("final lock holder acquisition failed")
			}
			admission, err := client.CreateRender(ctx, key, selection)
			if err != nil {
				t.Fatal("final-fence render admission failed")
			}
			deadline := time.Now().Add(80 * time.Second)
			for {
				var waiting bool
				if err = generationPool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND NOT granted AND classid=0 AND objid=$1::oid AND objsubid=1)`, lockID).Scan(&waiting); err != nil {
					t.Fatal("actual final publication wait observation failed")
				}
				if waiting {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("actual Temporal publisher did not reach exact final owner wait")
				}
				time.Sleep(10 * time.Millisecond)
			}
			var artifactID string
			if err = generationPool.QueryRow(ctx, `SELECT artifact_id FROM generation_publication_candidates WHERE job_id=$1 ORDER BY created_at DESC LIMIT 1`, admission.Job.JobID).Scan(&artifactID); err != nil || artifactID == "" {
				t.Fatal("finalized candidate binding absent at observed final wait")
			}
			resultURL := ownerStorage + "/v1/artifacts/" + artifactID + "/content"
			status, _ := ownerRequest(t, ownerHTTP(nil), "GET", resultURL, readerSession.Token, "", "", nil)
			if status != 404 {
				t.Fatal("finalized private candidate became publicly readable before terminal commit")
			}
			refreshed := session
			if mode == "capability-revoked-through-node-b" {
				ownerBind(t, b, managerAuth, permissionScope, generator.ID, "generate", true)
				ownerPermission(t, session, scope, "generation.publish", false)
			} else {
				ownerLogout(t, session)
				refreshed = ownerLogin(t, generator)
				freshIdentity := ownerSessionIdentity(t, refreshed)
				if freshIdentity.Authority == originalIdentity.Authority {
					t.Fatal("fresh provider session reused original authority")
				}
				ownerPermission(t, refreshed, scope, "generation.publish", true)
			}
			if err = holder.Commit(ctx); err != nil {
				t.Fatal("final owner wait release failed")
			}
			job := ownerJob(t, managerGeneration, admission.Job.JobID)
			if job.Status != generation.Failed || job.FailureCode != "access_denied" || job.Artifact != nil {
				t.Fatalf("revoked final publication escaped rollback: %s %s", job.Status, job.FailureCode)
			}
			status, _ = ownerRequest(t, ownerHTTP(nil), "GET", resultURL, readerSession.Token, "", "", nil)
			if status != 404 {
				t.Fatal("saved candidate URL became public after revoked final publication")
			}
			var raw []byte
			if err = generationPool.QueryRow(ctx, `SELECT original_identity FROM generation_jobs WHERE job_id=$1`, job.JobID).Scan(&raw); err != nil {
				t.Fatal("original fenced authority unavailable")
			}
			var saved identity.Identity
			if json.Unmarshal(raw, &saved) != nil || saved != originalIdentity {
				t.Fatal("final-boundary work rebound original authority")
			}
			if mode == "capability-revoked-through-node-b" {
				ownerBind(t, a, managerAuth, permissionScope, generator.ID, "generate", false)
				ownerPermission(t, session, scope, "generation.create", true)
			} else {
				freshClient, _ := generation.NewClient(ownerGeneration, refreshed.Token, ownerHTTP(nil))
				replay, err := freshClient.CreateRender(ctx, key, selection)
				if err != nil || replay.Job.JobID != job.JobID || replay.Job.Status != generation.Failed {
					t.Fatal("new session rescued original revoked work or lost its authorized receipt")
				}
				ownerRestricted(t, replay.Job)
			}
			t.Log("actual PG-observed final wait, live Ory/Keto A/B revocation, real finalized Storage candidate and Temporal terminal rollback passed")
		})
	}
}
