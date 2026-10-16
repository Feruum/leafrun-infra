package e06

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	access "github.com/leafrun/leafrun-contracts/access/v1"
	generation "github.com/leafrun/leafrun-contracts/generation/v1"
	identity "github.com/leafrun/leafrun-contracts/identity/v1"
	projects "github.com/leafrun/leafrun-contracts/projects/v1"
	storage "github.com/leafrun/leafrun-contracts/storage/v1"
)

const ownerSuccessSource = "#set text(font: \"Fira Sans\")\n#let p = json(sys.inputs.at(\"leafrun-data\"))\n#read(\"../private.txt\")\n#json(\"../private.json\").message\n#image(\"../private.svg\")\n#image(\"../private.png\")\n#if \"logo\" in p { image(p.logo) }"

func TestRealPublishedGeneratePrivateInputsDiagnosticsAndHistoricalDownload(t *testing.T) {
	ownerEnabled(t)
	ctx := context.Background()
	creator, manager, generator, reader := ownerIdentity(t), ownerIdentity(t), ownerIdentity(t), ownerIdentity(t)
	creatorSession, managerSession, generatorSession, readerSession := ownerLogin(t, creator), ownerLogin(t, manager), ownerLogin(t, generator), ownerLogin(t, reader)
	a, b := ownerAccess(t, ownerNodeA), ownerAccess(t, ownerNodeB)
	creatorAuth, managerAuth := access.ResolveRequest{Bearer: creatorSession.Token}, access.ResolveRequest{Bearer: managerSession.Token}
	org, err := a.CreateOrganization(ctx, creatorAuth, access.NameRequest{Name: "Task5 private compilation"})
	if err != nil {
		t.Fatal("owner organization create failed")
	}
	for _, member := range []struct{ id, role string }{{manager.ID, "owner"}, {generator.ID, "member"}, {reader.ID, "member"}} {
		if _, err = b.SetOrganizationMembership(ctx, creatorAuth, org.OrganizationID, access.MembershipRequest{HumanID: member.id, Role: member.role}); err != nil {
			t.Fatal("same-organization test membership failed")
		}
	}
	projectPool, generationPool, storagePool := ownerPool(t, "E06_PROJECTS_DATABASE_URL"), ownerPool(t, "E06_GENERATION_DATABASE_URL"), ownerPool(t, "E06_STORAGE_DATABASE_URL")
	scope := storage.Scope{WorkspaceID: "legacy-render-" + ownerNonce(t), ProjectID: "project"}
	snapshot, revision := ownerSeedRevision(t, projectPool, scope, ownerSuccessSource)
	if _, err = ownerImport(t, ownerNodeA, org.OrganizationID, scope, revision, creator.ID); err != nil {
		t.Fatal("verified render scope import failed")
	}
	permissionScope := access.PermissionScope{OrganizationID: org.OrganizationID, WorkspaceID: scope.WorkspaceID, ProjectID: scope.ProjectID}
	ownerBind(t, b, managerAuth, permissionScope, generator.ID, "generate", false)
	ownerBind(t, a, managerAuth, permissionScope, reader.ID, "download", false)
	ownerPermission(t, generatorSession, scope, "generation.create", true)
	ownerPermission(t, generatorSession, scope, "source.read", false)
	ownerPermission(t, generatorSession, scope, "resource.read", false)
	ownerPermission(t, generatorSession, scope, "artifact.read", false)
	ownerPermission(t, readerSession, scope, "artifact.read", true)
	creatorIdentity := ownerSessionIdentity(t, creatorSession)
	release := ownerRelease(t, creatorSession.Token, scope, revision)
	// Ready release management is a new synchronous operation. The old creator's
	// logged-out credential remains attached only to the original deferred work.
	ownerLogout(t, creatorSession)
	managerProjects, _ := projects.NewClient(ownerProjects, managerSession.Token, ownerHTTP(nil))
	release, err = managerProjects.SetPublication(ctx, release.ReleaseID, projects.PublicationRequest{Scope: scope, ExpectedVersion: release.PublicationVersion, Published: true})
	if err != nil || !release.Published {
		t.Fatal("current authorized manager could not publish ready release after creator logout")
	}
	var releaseAuthority, pinAuthority []byte
	if err = projectPool.QueryRow(ctx, `SELECT r.original_identity,a.original_identity FROM projects_releases r JOIN projects_release_pin_acquires a ON a.release_id=r.release_id WHERE r.release_id=$1 ORDER BY a.acquire_number DESC LIMIT 1`, release.ReleaseID).Scan(&releaseAuthority, &pinAuthority); err != nil {
		t.Fatal("original release provenance unavailable")
	}
	var savedCreator, savedPin identity.Identity
	if json.Unmarshal(releaseAuthority, &savedCreator) != nil || json.Unmarshal(pinAuthority, &savedPin) != nil || savedCreator != creatorIdentity || savedPin != creatorIdentity {
		t.Fatal("current publication manager rebound original deferred authority")
	}
	generatorProjects, _ := projects.NewClient(ownerProjects, generatorSession.Token, ownerHTTP(nil))
	if _, err = generatorProjects.GetRevision(ctx, scope, revision); err == nil {
		t.Fatal("generate-only user read source-facing revision")
	}
	form, err := generatorProjects.Form(ctx, release.ReleaseID)
	if err != nil || len(form.SchemaResources) != 1 {
		t.Fatal("generate-only published form failed")
	}
	if _, leaked := form.SchemaResources["private.txt"]; leaked {
		t.Fatal("form exposed captured source outside schema closure")
	}
	generatorClient, _ := generation.NewClient(ownerGeneration, generatorSession.Token, ownerHTTP(nil))
	ownerClient, _ := generation.NewClient(ownerGeneration, managerSession.Token, ownerHTTP(nil))
	selection := generation.RenderSelectionRequest{Scope: scope, ReleaseID: release.ReleaseID, Parameters: json.RawMessage(`{}`)}
	working := generation.RenderSelectionRequest{Scope: scope, SnapshotID: snapshot.SnapshotID, TargetID: "document", Parameters: json.RawMessage(`{}`)}
	if _, err = generatorClient.CreateRender(ctx, "working-denied-"+ownerNonce(t), working); err == nil {
		t.Fatal("generate-only working admission skipped source.read")
	}
	invalid := selection
	invalid.Parameters = json.RawMessage(`{"unexpected-source-key":true}`)
	if _, err = generatorClient.CreateRender(ctx, "invalid-"+ownerNonce(t), invalid); err == nil {
		t.Fatal("invalid parameter admission succeeded")
	} else {
		var remote *generation.HTTPError
		if !errors.As(err, &remote) || len(remote.Fields) != 0 || strings.Contains(remote.Message, "schemas/input.json") {
			t.Fatal("generate-only validation exposed source-sensitive details")
		}
	}
	key := "private-render-" + ownerNonce(t)
	admission, err := generatorClient.CreateRender(ctx, key, selection)
	if err != nil {
		t.Fatal("generate-only published admission failed")
	}
	ownerRestricted(t, admission.Job)
	job := ownerJob(t, generatorClient, admission.Job.JobID)
	ownerRestricted(t, job)
	if job.Status != generation.Succeeded || job.Artifact == nil {
		t.Fatalf("private captured source/font compilation failed: status=%s code=%s phase=%s", job.Status, job.FailureCode, job.Phase)
	}
	resultURL := ownerStorage + "/v1/artifacts/" + job.Artifact.ArtifactID + "/content"
	status, _ := ownerRequest(t, ownerHTTP(nil), "GET", resultURL, generatorSession.Token, "", "", nil)
	if status != 404 {
		t.Fatal("generate-only caller acquired download")
	}
	beforeExpiry := job.Artifact.ExpiresAt
	status, pdf := ownerRequest(t, ownerHTTP(nil), "GET", resultURL, readerSession.Token, "", "", nil)
	if status != 200 || !strings.HasPrefix(string(pdf), "%PDF-") {
		t.Fatal("separate current authorized reader could not download private compiled result")
	}
	originalIdentity := ownerSessionIdentity(t, generatorSession)
	ownerLogout(t, generatorSession)
	status, _ = ownerRequest(t, ownerHTTP(nil), "GET", resultURL, readerSession.Token, "", "", nil)
	if status != 200 {
		t.Fatal("successful historical artifact incorrectly required original live credential")
	}
	refreshed := ownerLogin(t, generator)
	refreshedClient, _ := generation.NewClient(ownerGeneration, refreshed.Token, ownerHTTP(nil))
	replay, err := refreshedClient.CreateRender(ctx, key, selection)
	if err != nil || replay.Job.JobID != job.JobID || replay.Job.Artifact == nil || !replay.Job.Artifact.ExpiresAt.Equal(beforeExpiry) {
		t.Fatal("authorized new-session receipt replay changed result identity or original TTL")
	}
	ownerRestricted(t, replay.Job)
	var original []byte
	if err = generationPool.QueryRow(ctx, `SELECT original_identity FROM generation_jobs WHERE job_id=$1`, job.JobID).Scan(&original); err != nil {
		t.Fatal("original generation provenance unavailable")
	}
	var saved identity.Identity
	if json.Unmarshal(original, &saved) != nil || saved != originalIdentity {
		t.Fatal("new session rebound admitted original authority")
	}
	// Independently selected files require resource.read even though private
	// captured project/font bytes above compiled without it.
	fixedStorage, _ := storage.NewClient(ownerStorage, os.Getenv("E06_GENERATION_STORAGE_TOKEN"), ownerHTTP(nil))
	selected := ownerResource(t, fixedStorage, scope, []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect width="8" height="8" fill="red"/></svg>`))
	withFile := selection
	withFile.FileRefs = map[string]generation.FileReference{"/logo": {ResourceID: selected.ResourceID, Filename: "logo.svg"}}
	if _, err = refreshedClient.CreateRender(ctx, "file-denied-"+ownerNonce(t), withFile); err == nil {
		t.Fatal("selected input admission skipped independent resource.read")
	}
	ownerBind(t, b, managerAuth, permissionScope, generator.ID, "resource.read", false)
	ownerPermission(t, refreshed, scope, "resource.read", true)
	selectedAdmission, err := refreshedClient.CreateRender(ctx, "file-allowed-"+ownerNonce(t), withFile)
	if err != nil {
		t.Fatal("authorized selected-resource admission failed")
	}
	selectedJob := ownerJob(t, refreshedClient, selectedAdmission.Job.JobID)
	if selectedJob.Status != generation.Succeeded {
		t.Fatalf("selected resource compile failed: %s %s", selectedJob.Status, selectedJob.FailureCode)
	}
	ownerBind(t, a, managerAuth, permissionScope, generator.ID, "source.read", false)
	ownerPermission(t, refreshed, scope, "source.read", true)
	visible, err := refreshedClient.GetJob(ctx, selectedJob.JobID)
	if err != nil || !strings.Contains(string(visible.Parameters), "/.leafrun-input/") {
		t.Fatal("authorized source/resource reader lost prepared selected-file binding")
	}
	ownerBind(t, b, managerAuth, permissionScope, generator.ID, "resource.read", true)
	ownerPermission(t, refreshed, scope, "resource.read", false)
	hidden, err := refreshedClient.GetJob(ctx, selectedJob.JobID)
	if err != nil {
		t.Fatal("selected-resource historical status failed")
	}
	ownerRestricted(t, hidden)
	// Real native compiler failures preserve authorized diagnostics while the
	// restricted reader receives no messages, hints, trace, paths or entrypoint.
	ownerBind(t, b, managerAuth, permissionScope, generator.ID, "source.read", true)
	ownerPermission(t, refreshed, scope, "source.read", false)
	failureSnapshot, failureRevision := ownerSeedRevision(t, projectPool, scope, "#assert(false, message: read(\"../private.txt\"))")
	_ = failureSnapshot
	failureRelease := ownerRelease(t, managerSession.Token, scope, failureRevision)
	failureRelease, err = managerProjects.SetPublication(ctx, failureRelease.ReleaseID, projects.PublicationRequest{Scope: scope, ExpectedVersion: failureRelease.PublicationVersion, Published: true})
	if err != nil {
		t.Fatal("technical-only failing release publication failed")
	}
	failureSelection := selection
	failureSelection.ReleaseID = failureRelease.ReleaseID
	failedAdmission, err := refreshedClient.CreateRender(ctx, "native-failure-"+ownerNonce(t), failureSelection)
	if err != nil {
		t.Fatal("failing published document admission failed")
	}
	failed := ownerJob(t, refreshedClient, failedAdmission.Job.JobID)
	if failed.Status != generation.Failed {
		t.Fatalf("native assert did not fail: %s", failed.Status)
	}
	ownerRestricted(t, failed)
	authorized, err := ownerClient.GetJob(ctx, failed.JobID)
	if err != nil || !strings.Contains(authorized.RawDiagnostics, ownerPrivateSentinel) {
		t.Fatal("authorized reader lost native source-sensitive diagnostics")
	}
	_, err = refreshedClient.CancelJob(ctx, failed.JobID)
	var cancelDenied *generation.HTTPError
	if !errors.As(err, &cancelDenied) || cancelDenied.Status != 403 || len(cancelDenied.Fields) != 0 {
		t.Fatal("generate-only cancellation did not preserve the approved edit permission boundary")
	}
	if _, err = ownerClient.CancelJob(ctx, failed.JobID); err != nil {
		t.Fatal("authorized typed bodyless cancellation compatibility failed")
	}
	for _, suffix := range []string{"", "/content"} {
		t.Run("storage-final-live-revocation"+strings.ReplaceAll(suffix, "/", "-"), func(t *testing.T) {
			lock, err := storagePool.Begin(ctx)
			if err != nil {
				t.Fatal("storage final lock fixture failed")
			}
			defer lock.Rollback(context.Background())
			if _, err = lock.Exec(ctx, `SELECT pg_advisory_xact_lock(710032)`); err != nil {
				t.Fatal("storage final lock fixture acquisition failed")
			}
			type result struct {
				status int
				err    error
			}
			done := make(chan result, 1)
			go func() {
				req, _ := http.NewRequest("GET", ownerStorage+"/v1/artifacts/"+job.Artifact.ArtifactID+suffix, nil)
				req.Header.Set("Authorization", "Bearer "+readerSession.Token)
				resp, e := ownerHTTP(nil).Do(req)
				if e != nil {
					done <- result{err: e}
					return
				}
				resp.Body.Close()
				done <- result{status: resp.StatusCode}
			}()
			deadline := time.Now().Add(4 * time.Second)
			for {
				var waiting bool
				if err = storagePool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%pg_advisory_xact_lock(710032)%')`).Scan(&waiting); err != nil {
					t.Fatal("storage final wait observation failed")
				}
				if waiting {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("storage reader did not reach catalog wait")
				}
				time.Sleep(10 * time.Millisecond)
			}
			select {
			case <-done:
				t.Fatal("storage read returned while catalog fixture lock held")
			default:
			}
			ownerBind(t, b, managerAuth, permissionScope, reader.ID, "download", true)
			ownerPermission(t, readerSession, scope, "artifact.read", false)
			if err = lock.Commit(ctx); err != nil {
				t.Fatal("storage final wait release failed")
			}
			select {
			case out := <-done:
				if out.err != nil || out.status != 404 {
					t.Fatal("storage final read committed after live download revocation")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("storage final read did not return")
			}
			var leases int
			if err = storagePool.QueryRow(ctx, `SELECT count(*) FROM storage_read_leases WHERE object_id=$1`, job.Artifact.ArtifactID).Scan(&leases); err != nil || leases != 0 {
				t.Fatal("revoked storage final read retained a lease")
			}
			ownerBind(t, a, managerAuth, permissionScope, reader.ID, "download", false)
			ownerPermission(t, readerSession, scope, "artifact.read", true)
		})
	}
	ownerBind(t, b, managerAuth, permissionScope, reader.ID, "download", true)
	ownerPermission(t, readerSession, scope, "artifact.read", false)
	status, _ = ownerRequest(t, ownerHTTP(nil), "GET", resultURL, readerSession.Token, "", "", nil)
	if status != 404 {
		t.Fatal("saved artifact URL bypassed current download revocation")
	}
	ownerBind(t, a, managerAuth, permissionScope, reader.ID, "download", false)
	ownerPermission(t, readerSession, scope, "artifact.read", true)
	if _, err = storagePool.Exec(ctx, `UPDATE storage_objects SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, job.Artifact.ArtifactID); err != nil {
		t.Fatal("bounded expiry fixture failed")
	}
	status, _ = ownerRequest(t, ownerHTTP(nil), "GET", resultURL, readerSession.Token, "", "", nil)
	if status != 410 {
		t.Fatalf("authorized expired artifact status %d", status)
	}
	var leases int
	if err = storagePool.QueryRow(ctx, `SELECT count(*) FROM storage_read_leases WHERE object_id=$1`, job.Artifact.ArtifactID).Scan(&leases); err != nil || leases != 0 {
		t.Fatal("denied/expired download retained a read lease")
	}
	t.Log("real Temporal/Typst/S3 private captured source+font render, independent selected-resource rights, restricted native failure/replay/cancel, creator logout/current manager publication, historical reader download and saved-URL revocation/expiry passed")
}
