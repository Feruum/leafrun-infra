package e04

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	generationv1 "github.com/leafrun/leafrun-contracts/generation/v1"
	projectsv1 "github.com/leafrun/leafrun-contracts/projects/v1"
	storagev1 "github.com/leafrun/leafrun-contracts/storage/v1"
	core "github.com/leafrun/leafrun-document-core"
	"github.com/leafrun/leafrun-infra/internal/fixture"
	"go.temporal.io/sdk/client"
)

const e05Manifest = `{"formatVersion":1,"name":"E05 templates","engine":{"typstVersion":"0.15.1"},"templates":[{"id":"invoice","entrypoint":"templates/main.typ","inputSchema":"schemas/input.json"}],"targets":[{"id":"invoice","kind":"document","template":"invoice","output":{"format":"pdf","path":"output/result.pdf"}},{"id":"bundle","kind":"bundle","inputSchema":"schemas/input.json","steps":[{"id":"invoice","template":"invoice","inputPointer":""}],"order":["invoice"],"output":{"format":"zip","path":"output/bundle.zip"}}],"profiles":{"draft":{"inputs":{"quality":"draft"}},"production":{"inputs":{"quality":"production"}},"archive":{"inputs":{"quality":"archive"},"pdfStandard":"1.7"}},"defaultProfile":"production"}`

const e05Schema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"amount":{"type":"number","default":1.0},"customer":{"type":"object","default":{},"properties":{"name":{"type":"string","default":"E05 default"}}},"logo":{"type":"string","x-leafrun-file":true}},"additionalProperties":false}`

func e05Stand(t *testing.T) (*stand, *projectsv1.Client) {
	t.Helper()
	if os.Getenv("LEAFRUN_E05_INTEGRATION") != "1" {
		t.Skip("explicit internal E05 stand is disabled")
	}
	s := liveStand(t)
	projects, err := projectsv1.NewClient(os.Getenv("LEAFRUN_E05_PROJECTS_URL"), os.Getenv("GENERATION_USER_TOKEN"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return s, projects
}

func (s *stand) e05Resource(t *testing.T, data []byte) storagev1.Resource {
	t.Helper()
	hash := sha256.Sum256(data)
	upload, err := s.storage.CreateUpload(s.ctx, storagev1.CreateUploadRequest{Scope: s.scope, Class: storagev1.ClassResource,
		SizeBytes: int64(len(data)), SHA256: hex.EncodeToString(hash[:]), IdempotencyKey: fmt.Sprintf("e05-upload-%d", time.Now().UnixNano())})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.storage.PutUpload(s.ctx, upload.UploadID, upload.WriterToken, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	resource, err := s.storage.CommitResource(s.ctx, upload.UploadID, storagev1.CommitResourceRequest{WriterToken: upload.WriterToken, IdempotencyKey: upload.UploadID})
	if err != nil {
		t.Fatal(err)
	}
	return resource
}

func (s *stand) e05Snapshot(t *testing.T, typst string) storagev1.Snapshot {
	t.Helper()
	root, fonts := t.TempDir(), t.TempDir()
	for name, data := range map[string][]byte{
		"leafrun.json": []byte(e05Manifest), "schemas/input.json": []byte(e05Schema), "templates/main.typ": []byte(typst),
		"private.json": []byte(`{"sourceOnly":"must not be exposed in a form"}`),
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "empty", "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	font, err := os.ReadFile(os.Getenv("LEAFRUN_E03_FONT_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(fonts, "Fira.ttf"), font, 0600); err != nil {
		t.Fatal(err)
	}
	captured, err := core.CreateSnapshot(s.ctx, core.FolderSource{Root: root}, core.SnapshotOptions{FontPaths: []string{fonts}})
	if err != nil {
		t.Fatal(err)
	}
	defer captured.Close()
	if err = os.RemoveAll(fonts); err != nil {
		t.Fatal(err)
	}
	manifest := captured.Manifest()
	wireBytes, _ := json.Marshal(manifest)
	var wire storagev1.SnapshotManifest
	if err = json.Unmarshal(wireBytes, &wire); err != nil {
		t.Fatal(err)
	}
	refs := []storagev1.ResourceRef{}
	send := func(tree fs.FS, files []core.SnapshotFile, kind string, index int) {
		for _, file := range files {
			data, readErr := fs.ReadFile(tree, file.Path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			resource := s.e05Resource(t, data)
			refs = append(refs, storagev1.ResourceRef{Tree: kind, FontIndex: index, Path: file.Path, ResourceID: resource.ResourceID})
		}
	}
	send(captured.FS(), manifest.Files, "project", 0)
	for index, font := range manifest.Fonts {
		if !font.InProject {
			tree, fontErr := captured.FontFS(index)
			if fontErr != nil {
				t.Fatal(fontErr)
			}
			send(tree, font.Files, "font", index)
		}
	}
	snapshot, err := s.storage.CreateSnapshot(s.ctx, storagev1.CreateSnapshotRequest{Scope: s.scope, Manifest: wire, Resources: refs, IdempotencyKey: fmt.Sprintf("e05-snapshot-%d", time.Now().UnixNano())})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func (s *stand) e05Release(t *testing.T, projects *projectsv1.Client, snapshot storagev1.Snapshot) (projectsv1.Revision, projectsv1.Release) {
	t.Helper()
	key := fmt.Sprintf("e05-%d", time.Now().UnixNano())
	revision, err := projects.RegisterRevision(s.ctx, projectsv1.RegisterRevisionRequest{Scope: s.scope, Revision: key, SnapshotID: snapshot.SnapshotID, IdempotencyKey: key})
	if err != nil {
		t.Fatal(err)
	}
	release, err := projects.CreateRelease(s.ctx, projectsv1.CreateReleaseRequest{Scope: s.scope, Revision: revision.Revision, TargetID: "invoice", Version: key, IdempotencyKey: key})
	if err != nil {
		t.Fatal(err)
	}
	for release.State != "ready" {
		select {
		case <-s.ctx.Done():
			t.Fatalf("release did not activate: state=%s", release.State)
		case <-time.After(100 * time.Millisecond):
		}
		release, err = projects.GetRelease(s.ctx, release.ReleaseID)
		if err != nil {
			t.Fatal(err)
		}
	}
	return revision, release
}

func TestE05ReleasePublicationWithoutRenderAndD16(t *testing.T) {
	s, projects := e05Stand(t)
	// Technical validity is enough to publish; this source intentionally fails
	// if any implementation invokes Typst as a publication prerequisite.
	snapshot := s.e05Snapshot(t, "#this_identifier_is_deliberately_undefined")
	revision, release := s.e05Release(t, projects, snapshot)
	published, err := projects.SetPublication(s.ctx, release.ReleaseID, projectsv1.PublicationRequest{Scope: s.scope, ExpectedVersion: release.PublicationVersion, Published: true})
	if err != nil || !published.Published || published.DescriptorDigest != release.DescriptorDigest {
		t.Fatalf("technical-only release publication: %+v, %v", published, err)
	}
	if err = projects.DeleteRevision(s.ctx, revision.Revision, projectsv1.DeleteRevisionRequest{Scope: s.scope}); err != nil {
		t.Fatal(err)
	}
	s.ageE05Snapshot(t, snapshot.SnapshotID)
	s.sweepE05(t)
	if got, getErr := s.storage.GetSnapshot(s.ctx, snapshot.SnapshotID); getErr != nil || got.Manifest.ID != snapshot.Manifest.ID {
		t.Fatalf("published release lost its snapshot after working deletion: %v", getErr)
	}
	if err = projects.DeleteRelease(s.ctx, release.ReleaseID, projectsv1.DeleteReleaseRequest{Scope: s.scope, ExpectedVersion: published.PublicationVersion}); err == nil {
		t.Fatal("published release could be deleted")
	}
	withdrawn, err := projects.SetPublication(s.ctx, release.ReleaseID, projectsv1.PublicationRequest{Scope: s.scope, ExpectedVersion: published.PublicationVersion, Published: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = projects.DeleteRelease(s.ctx, release.ReleaseID, projectsv1.DeleteReleaseRequest{Scope: s.scope, ExpectedVersion: withdrawn.PublicationVersion}); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 20; attempt++ {
		s.sweepE05(t)
		_, err = s.storage.GetSnapshot(s.ctx, snapshot.SnapshotID)
		var gone *storagev1.HTTPError
		if errors.As(err, &gone) && (gone.Status == 404 || gone.Status == 410 || gone.Status == 409 && gone.Code == "object_unavailable") && s.e05SnapshotDeleted(t, snapshot.SnapshotID) {
			t.Log("D16: activated release pin survived working deletion; final unpublished release deletion made its snapshot collectible")
			return
		}
		select {
		case <-s.ctx.Done():
			t.Fatal("D16 cleanup did not complete")
		case <-time.After(250 * time.Millisecond):
		}
	}
	t.Fatal("D16 final released snapshot remained readable after reconciliation and real Temporal GC")
}

func (s *stand) e05SnapshotDeleted(t *testing.T, snapshotID string) bool {
	t.Helper()
	pool, err := pgxpool.New(s.ctx, os.Getenv("LEAFRUN_E05_STORAGE_CATALOG_URL"))
	if err != nil {
		t.Fatal("owned Storage test catalog is unavailable")
	}
	defer pool.Close()
	var deleted bool
	if err = pool.QueryRow(s.ctx, `SELECT state='deleted' FROM storage_objects WHERE id=$1 AND workspace=$2 AND project=$3 AND class='snapshot'`, snapshotID, s.scope.WorkspaceID, s.scope.ProjectID).Scan(&deleted); err != nil {
		t.Fatal("selected snapshot lifecycle could not be verified")
	}
	return deleted
}

func (s *stand) ageE05Snapshot(t *testing.T, snapshotID string) {
	t.Helper()
	// Test-only clock setup on the owned local catalog, limited to this ID. The
	// deployed Storage API keeps the production 24-hour orphan grace unchanged.
	pool, err := pgxpool.New(s.ctx, os.Getenv("LEAFRUN_E05_STORAGE_CATALOG_URL"))
	if err != nil {
		t.Fatal("owned Storage test catalog is unavailable")
	}
	defer pool.Close()
	tag, err := pool.Exec(s.ctx, `UPDATE storage_objects SET created_at=clock_timestamp()-interval '25 hours' WHERE id=$1 AND workspace=$2 AND project=$3 AND class='snapshot'`, snapshotID, s.scope.WorkspaceID, s.scope.ProjectID)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatal("could not prepare the selected owned snapshot's orphan-grace boundary")
	}
}

func (s *stand) sweepE05(t *testing.T) {
	t.Helper()
	connection, err := client.Dial(client.Options{HostPort: os.Getenv("LEAFRUN_STORAGE_TEMPORAL_ADDRESS"), Namespace: "default"})
	if err != nil {
		t.Fatal("internal Temporal is unavailable")
	}
	defer connection.Close()
	run, err := connection.ExecuteWorkflow(s.ctx, client.StartWorkflowOptions{ID: fmt.Sprintf("e05-gc-%d", time.Now().UnixNano()), TaskQueue: "leafrun-storage-gc", WorkflowExecutionTimeout: time.Minute}, "GCWorkflow")
	if err != nil || run.Get(s.ctx, nil) != nil {
		t.Fatal("real Storage Temporal sweep failed")
	}
}

func TestE05ReleaseSelectionDefaultsFilesProfilesAndWithdrawal(t *testing.T) {
	s, projects := e05Stand(t)
	snapshot := s.e05Snapshot(t, "#set text(font: \"Fira Sans\")\n#let p = json(sys.inputs.at(\"leafrun-data\"))\n#assert(p.customer.name == \"E05 default\")\n#assert(type(p.amount) == float)\n#assert(sys.inputs.at(\"quality\") == \"archive\")\nE05 #p.customer.name\n#image(p.logo)")
	revision, release := s.e05Release(t, projects, snapshot)
	release, err := projects.SetPublication(s.ctx, release.ReleaseID, projectsv1.PublicationRequest{Scope: s.scope, ExpectedVersion: release.PublicationVersion, Published: true})
	if err != nil {
		t.Fatal(err)
	}
	file := s.e05Resource(t, []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="30" height="20"><rect width="30" height="20" fill="navy"/></svg>`))
	s.e05PermitGenerator(t, file.ResourceID)
	generator, _ := generationv1.NewClient(os.Getenv("LEAFRUN_E04_GENERATION_URL"), os.Getenv("GENERATION_ONLY_USER_TOKEN"), nil)
	generatorProjects, _ := projectsv1.NewClient(os.Getenv("LEAFRUN_E05_PROJECTS_URL"), os.Getenv("GENERATION_ONLY_USER_TOKEN"), nil)
	generatorStorage, _ := storagev1.NewClient(os.Getenv("LEAFRUN_E04_STORAGE_URL"), os.Getenv("GENERATION_ONLY_USER_TOKEN"), nil)
	if _, err = generatorProjects.GetRevision(s.ctx, s.scope, revision.Revision); err == nil {
		t.Fatal("generation-only principal read protected revision descriptors")
	}
	if _, err = generatorStorage.GetSnapshot(s.ctx, snapshot.SnapshotID); err == nil {
		t.Fatal("generation-only principal read a protected source snapshot")
	}
	if response, err := generatorStorage.ResourceContent(s.ctx, snapshot.Resources[0].ResourceID); err == nil {
		_ = response.Body.Close()
		t.Fatal("generation-only principal read protected source resource bytes")
	}
	form, err := generatorProjects.Form(s.ctx, release.ReleaseID)
	if err != nil || len(form.SchemaResources) != 1 || form.DefaultProfile != "production" || form.Profiles["archive"].Inputs["quality"] != "archive" {
		t.Fatalf("generation-only versioned form: %+v %v", form, err)
	}
	if _, leaked := form.SchemaResources["private.json"]; leaked {
		t.Fatal("form exposed source JSON outside its schema closure")
	}
	selection := generationv1.RenderSelectionRequest{Scope: s.scope, ReleaseID: release.ReleaseID, Parameters: json.RawMessage(`{}`),
		FileRefs: map[string]generationv1.FileReference{"/logo": {ResourceID: file.ResourceID, Filename: "logo.svg"}}, Profile: "archive"}
	key := fmt.Sprintf("e05-render-%d", time.Now().UnixNano())
	admission, err := generator.CreateRender(s.ctx, key, selection)
	if err != nil {
		t.Fatal(err)
	}
	// The privileged test user inspects the frozen derived snapshot. The caller
	// itself is still denied source-read and gets a redacted job representation.
	job := s.wait(t, admission.Job.JobID)
	if job.Status != generationv1.Succeeded || job.Artifact == nil {
		t.Fatalf("release-selected render failed: %+v", job)
	}
	callerJob, err := generator.GetJob(s.ctx, job.JobID)
	if err != nil || callerJob.RawDiagnostics != "" || len(callerJob.Parameters) != 0 || len(callerJob.Diagnostics) != 0 {
		t.Fatalf("generation-only source redaction: %+v %v", callerJob, err)
	}
	derived, err := s.storage.GetSnapshot(s.ctx, job.DerivedSnapshotID)
	if err != nil {
		t.Fatal(err)
	}
	var prepared []byte
	for _, ref := range derived.Resources {
		if ref.Tree == "project" && ref.Path == "input.json" {
			response, readErr := s.storage.ResourceContent(s.ctx, ref.ResourceID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			prepared, readErr = io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
		}
	}
	want := fmt.Sprintf(`{"amount":1.0,"customer":{"name":"E05 default"},"logo":"/.leafrun-input/%s.svg"}`, file.SHA256)
	if string(prepared) != want {
		t.Fatalf("frozen Core preparation changed: %s", prepared)
	}
	artifact, err := s.storage.GetArtifact(s.ctx, job.Artifact.ArtifactID)
	if err != nil || artifact.ExpiresAt.Sub(artifact.CreatedAt) != 24*time.Hour {
		t.Fatalf("original fixed PDF lifetime: %+v %v", artifact, err)
	}
	response, err := generatorStorage.ArtifactContent(s.ctx, artifact.ArtifactID)
	if err != nil {
		t.Fatalf("generation-only caller could not download its published PDF: %v", err)
	}
	pdf, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	hash := sha256.Sum256(pdf)
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) || hex.EncodeToString(hash[:]) != artifact.Hash {
		t.Fatalf("generation-only download did not match the exact PDF: %v", err)
	}
	withdrawn, err := projects.SetPublication(s.ctx, release.ReleaseID, projectsv1.PublicationRequest{Scope: s.scope, ExpectedVersion: release.PublicationVersion, Published: false})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = generator.CreateRender(s.ctx, key+"-fresh", selection); err == nil {
		t.Fatal("withdrawn release admitted a new render")
	}
	historical, err := generatorStorage.GetArtifact(s.ctx, artifact.ArtifactID)
	if err != nil || !historical.ExpiresAt.Equal(artifact.ExpiresAt) {
		t.Fatalf("withdrawal changed an existing successful PDF: %+v %v", historical, err)
	}
	replay, err := generator.CreateRender(s.ctx, key, selection)
	if err != nil || replay.Job.JobID != job.JobID {
		t.Fatalf("authorized original selection replay after withdrawal: %+v %v", replay, err)
	}
	changed := selection
	changed.Parameters = json.RawMessage(`{"amount":2.0}`)
	if _, err = generator.CreateRender(s.ctx, key, changed); err == nil {
		t.Fatal("changed original selection reused its receipt")
	}
	republished, err := projects.SetPublication(s.ctx, release.ReleaseID, projectsv1.PublicationRequest{Scope: s.scope, ExpectedVersion: withdrawn.PublicationVersion, Published: true})
	if err != nil || republished.DescriptorDigest != release.DescriptorDigest || republished.SnapshotID != release.SnapshotID {
		t.Fatalf("old release republish changed its frozen identity: %+v %v", republished, err)
	}
	currentArtifact, err := s.storage.GetArtifact(s.ctx, artifact.ArtifactID)
	if err != nil || !currentArtifact.ExpiresAt.Equal(artifact.ExpiresAt) {
		t.Fatal("withdrawal/republish/replay extended the original PDF expiry")
	}
	if path := os.Getenv("LEAFRUN_E05_RECEIPT_FILE"); path != "" {
		writeE05Receipt(t, path, e05RestartReceipt{Scope: s.scope, ReleaseID: release.ReleaseID, SnapshotID: snapshot.SnapshotID,
			ContentID: snapshot.Manifest.ID, DescriptorDigest: release.DescriptorDigest, JobID: job.JobID,
			DerivedContentID: derived.Manifest.ID, ArtifactID: artifact.ArtifactID, SHA256: artifact.Hash, ExpiresAt: artifact.ExpiresAt,
			IdempotencyKey: key, Selection: selection, ResourceID: file.ResourceID})
	}
	t.Log("release selection rendered shared defaults, nested root-relative file input, frozen custom profile and captured external font; withdrawal/replay/republish preserved identity and original PDF expiry")
}

func (s *stand) e05PermitGenerator(t *testing.T, resourceID string) {
	t.Helper()
	s.e05Access(t, fixture.State{Scope: s.scope, Published: map[string]string{}, Pins: map[string]storagev1.OwnerState{}, AccessRules: []fixture.AccessRule{{
		Principal:  storagev1.Principal{Subject: "generator", Domain: "user"},
		Operations: map[string]bool{"generation.create": true, "generation.execute": true, "generation.publish": true, "generation.read": true, "artifact.read": true, "resource.read": true},
		ObjectIDs:  map[string][]string{"resource.read": {resourceID}},
	}}})
}

func TestE05WorkingSelectionRejectsInvalidInputsBeforeAdmission(t *testing.T) {
	s, projects := e05Stand(t)
	snapshot := s.e05Snapshot(t, "#let p = json(sys.inputs.at(\"leafrun-data\"))\n#assert(p.amount == 2.0)\n#assert(p.customer.name == \"E05 default\")\n#assert(sys.inputs.at(\"quality\") == \"draft\")\nWorking selection")
	key := fmt.Sprintf("e05-working-%d", time.Now().UnixNano())
	if _, err := projects.RegisterRevision(s.ctx, projectsv1.RegisterRevisionRequest{Scope: s.scope, Revision: key, SnapshotID: snapshot.SnapshotID, IdempotencyKey: key}); err != nil {
		t.Fatal(err)
	}
	foreign := *s
	foreign.scope.ProjectID += "-foreign"
	foreign.access(t, false, false)
	foreignFile := foreign.e05Resource(t, []byte("foreign input is not authorized in the selected project"))
	s.access(t, false, false)
	assertNoGenerationPins := func() {
		t.Helper()
		pool, err := pgxpool.New(s.ctx, os.Getenv("LEAFRUN_E05_STORAGE_CATALOG_URL"))
		if err != nil {
			t.Fatal("open owned Storage pin verification catalog")
		}
		defer pool.Close()
		var pins int
		err = pool.QueryRow(s.ctx, "SELECT count(*) FROM storage_pins WHERE object_id=ANY($1) AND holder_id='leafrun-generation' AND holder_domain='service'",
			[]string{snapshot.SnapshotID, foreignFile.ResourceID, "resource_missing_e05"}).Scan(&pins)
		if err != nil || pins != 0 {
			t.Fatal("rejected selection created a Generation-held Storage pin")
		}
	}
	assertNoGenerationPins()
	cases := []struct {
		name      string
		selection generationv1.RenderSelectionRequest
		status    int
		code      string
	}{
		{"bundle", generationv1.RenderSelectionRequest{Scope: s.scope, SnapshotID: snapshot.SnapshotID, TargetID: "bundle", Parameters: json.RawMessage("{}")}, 422, "unsupported_target"},
		{"schema", generationv1.RenderSelectionRequest{Scope: s.scope, SnapshotID: snapshot.SnapshotID, TargetID: "invoice", Parameters: json.RawMessage(`{"amount":"private-invalid-value"}`)}, 422, "invalid_selection"},
		{"missing-file", generationv1.RenderSelectionRequest{Scope: s.scope, SnapshotID: snapshot.SnapshotID, TargetID: "invoice", Parameters: json.RawMessage("{}"),
			FileRefs: map[string]generationv1.FileReference{"/logo": {ResourceID: "resource_missing_e05", Filename: "logo.svg"}}}, 404, "not_found"},
		{"foreign-file", generationv1.RenderSelectionRequest{Scope: s.scope, SnapshotID: snapshot.SnapshotID, TargetID: "invoice", Parameters: json.RawMessage("{}"),
			FileRefs: map[string]generationv1.FileReference{"/logo": {ResourceID: foreignFile.ResourceID, Filename: "logo.svg"}}}, 404, "not_found"},
	}
	for _, test := range cases {
		_, err := s.jobs.CreateRender(s.ctx, key+"-"+test.name, test.selection)
		var failure *generationv1.HTTPError
		if !errors.As(err, &failure) || failure.Status != test.status || failure.Code != test.code {
			t.Fatalf("%s returned unexpected bounded selection error: %v", test.name, err)
		}
		if test.name == "schema" && (len(failure.Fields) == 0 || failure.Fields[0].Pointer != "/amount" ||
			bytes.Contains([]byte(failure.Message), []byte("private-invalid-value"))) {
			t.Fatal("schema failure lost its field pointer or exposed its value")
		}
		assertNoGenerationPins()
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("LEAFRUN_E05_GENERATION_CATALOG_URL"))
	if err != nil || cfg.ConnConfig.Database != "generation" || cfg.ConnConfig.User != "generation" {
		t.Fatal("requires owned Generation catalog for pre-admission verification")
	}
	pool, err := pgxpool.NewWithConfig(s.ctx, cfg)
	if err != nil {
		t.Fatal("open owned Generation catalog")
	}
	defer pool.Close()
	var admitted int
	if err = pool.QueryRow(s.ctx, "SELECT count(*) FROM generation_jobs WHERE source_snapshot_id=$1", snapshot.SnapshotID).Scan(&admitted); err != nil || admitted != 0 {
		t.Fatal("unsupported or invalid selection created a durable job before validation")
	}
	for _, table := range []string{"generation_reservations", "generation_pin_acquires", "generation_workflow_starts"} {
		query := "SELECT count(*) FROM " + table + " r JOIN generation_jobs j ON j.job_id=r.job_id WHERE j.source_snapshot_id=$1"
		if err = pool.QueryRow(s.ctx, query, snapshot.SnapshotID).Scan(&admitted); err != nil || admitted != 0 {
			t.Fatalf("rejected selection created durable %s state", table)
		}
	}
	admission, err := s.jobs.CreateRender(s.ctx, key, generationv1.RenderSelectionRequest{Scope: s.scope, SnapshotID: snapshot.SnapshotID, TargetID: "invoice",
		Parameters: json.RawMessage(`{"amount":2.0}`), Profile: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	if job := s.wait(t, admission.Job.JobID); job.Status != generationv1.Succeeded || job.Artifact == nil {
		t.Fatalf("authorized working selection did not render: status=%s phase=%s", job.Status, job.Phase)
	}
	t.Log("bundle/schema/file failures left no admission; authorized working selection rendered its selected profile and missing-only defaults")
}

func (s *stand) e05Access(t *testing.T, state fixture.State) {
	t.Helper()
	data, _ := json.Marshal(state)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, os.Getenv("LEAFRUN_E04_OWNER_URL")+"/dev/v1/state", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+os.Getenv("FIXTURE_ADMIN_TOKEN"))
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 204 {
		t.Fatalf("explicit E05 access fixture returned %d", response.StatusCode)
	}
}
