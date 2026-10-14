package e04

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	generationv1 "github.com/leafrun/leafrun-contracts/generation/v1"
	jsonvalue "github.com/leafrun/leafrun-contracts/jsonvalue/v1"
	storagev1 "github.com/leafrun/leafrun-contracts/storage/v1"
	core "github.com/leafrun/leafrun-document-core"
	"github.com/leafrun/leafrun-infra/internal/fixture"
)

type stand struct {
	ctx     context.Context
	storage *storagev1.Client
	jobs    *generationv1.Client
	scope   storagev1.Scope
}

func liveStand(t *testing.T) *stand {
	t.Helper()
	if os.Getenv("LEAFRUN_E04_INTEGRATION") != "1" {
		t.Skip("explicit internal E04 stand is disabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	storage, err := storagev1.NewClient(os.Getenv("LEAFRUN_E04_STORAGE_URL"), os.Getenv("STORAGE_TOKEN"), &http.Client{Timeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := generationv1.NewClient(os.Getenv("LEAFRUN_E04_GENERATION_URL"), os.Getenv("GENERATION_USER_TOKEN"), &http.Client{Timeout: 20 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	s := &stand{ctx, storage, jobs, storagev1.Scope{WorkspaceID: "demo", ProjectID: "roundtrip"}}
	s.access(t, false, false)
	t.Cleanup(func() { s.access(t, false, false) })
	return s
}
func (s *stand) access(t *testing.T, deny, unavailable bool) {
	t.Helper()
	state := fixture.State{Scope: s.scope, Deny: deny, Unavailable: unavailable, Published: map[string]string{}, Pins: map[string]storagev1.OwnerState{}}
	data, _ := json.Marshal(state)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, os.Getenv("LEAFRUN_E04_OWNER_URL")+"/dev/v1/state", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv("FIXTURE_ADMIN_TOKEN"))
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 204 {
		t.Fatalf("fixture control status %d", response.StatusCode)
	}
}
func (s *stand) snapshot(t *testing.T, source string) storagev1.Snapshot {
	t.Helper()
	project, fonts := t.TempDir(), t.TempDir()
	files := map[string][]byte{
		"main.typ":   []byte(source),
		"input.json": []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["n","f","label"],"properties":{"n":{"type":"integer","minimum":9007199254740993},"f":{"type":"number"},"label":{"type":"string"}},"additionalProperties":false}`),
		"data.json":  []byte(`{"captured":true}`),
		"mark.svg":   []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="12"><rect width="24" height="12" fill="navy"/></svg>`),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(project, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(project, "empty", "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	font, err := os.ReadFile(os.Getenv("LEAFRUN_E03_FONT_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(fonts, "Fira.ttf"), font, 0600); err != nil {
		t.Fatal(err)
	}
	captured, err := core.CreateSnapshot(s.ctx, core.FolderSource{Root: project}, core.SnapshotOptions{FontPaths: []string{fonts}})
	if err != nil {
		t.Fatal(err)
	}
	defer captured.Close()
	if err = os.RemoveAll(fonts); err != nil {
		t.Fatal(err)
	}
	manifest := captured.Manifest()
	data, _ := json.Marshal(manifest)
	var wire storagev1.SnapshotManifest
	if err = json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	refs := []storagev1.ResourceRef{}
	send := func(tree fs.FS, entries []core.SnapshotFile, kind string, index int) {
		for _, file := range entries {
			payload, e := fs.ReadFile(tree, file.Path)
			if e != nil {
				t.Fatal(e)
			}
			hash := sha256.Sum256(payload)
			upload, e := s.storage.CreateUpload(s.ctx, storagev1.CreateUploadRequest{Scope: s.scope, Class: storagev1.ClassResource, SizeBytes: int64(len(payload)), SHA256: hex.EncodeToString(hash[:]), IdempotencyKey: fmt.Sprintf("e04-%d-%s", time.Now().UnixNano(), file.Path)})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = s.storage.PutUpload(s.ctx, upload.UploadID, upload.WriterToken, bytes.NewReader(payload)); e != nil {
				t.Fatal(e)
			}
			resource, e := s.storage.CommitResource(s.ctx, upload.UploadID, storagev1.CommitResourceRequest{WriterToken: upload.WriterToken, IdempotencyKey: upload.UploadID})
			if e != nil {
				t.Fatal(e)
			}
			refs = append(refs, storagev1.ResourceRef{Tree: kind, FontIndex: index, Path: file.Path, ResourceID: resource.ResourceID})
		}
	}
	send(captured.FS(), manifest.Files, "project", 0)
	for index, font := range manifest.Fonts {
		if !font.InProject {
			tree, e := captured.FontFS(index)
			if e != nil {
				t.Fatal(e)
			}
			send(tree, font.Files, "font", index)
		}
	}
	snapshot, err := s.storage.CreateSnapshot(s.ctx, storagev1.CreateSnapshotRequest{Scope: s.scope, Manifest: wire, Resources: refs, IdempotencyKey: fmt.Sprintf("e04-snapshot-%d", time.Now().UnixNano())})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
func (s *stand) wait(t *testing.T, id string) generationv1.Job {
	t.Helper()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		job, err := s.jobs.GetJob(s.ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status.Terminal() {
			return job
		}
		select {
		case <-s.ctx.Done():
			t.Fatalf("job %s did not finish (status=%s phase=%s)", id, job.Status, job.Phase)
		case <-ticker.C:
		}
	}
}

func TestLiveJobParametersPublicationAndAccess(t *testing.T) {
	s := liveStand(t)
	snapshot := s.snapshot(t, "#set text(font: \"Fira Sans\")\n#let p = json(\"input.json\")\n#assert(p.n == 9007199254740993)\n#assert(type(p.f) == float)\nCaptured #p.label\n#image(\"mark.svg\")")
	parameters := json.RawMessage(`{"label":"E04 <exact>","n":9007199254740993,"f":1.0}`)
	request := generationv1.CreateJobRequest{Scope: s.scope, SnapshotID: snapshot.SnapshotID, TargetID: "pdf", Entrypoint: "main.typ", Parameters: parameters, ParametersSchemaPath: "input.json"}
	key := fmt.Sprintf("e04-job-%d", time.Now().UnixNano())
	admission, err := s.jobs.CreateJob(s.ctx, key, request)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.jobs.CreateJob(s.ctx, key, request)
	if err != nil || replay.Job.JobID != admission.Job.JobID {
		t.Fatalf("admission replay: %+v %v", replay, err)
	}
	changed := request
	changed.Parameters = json.RawMessage(`{"label":"changed","n":9007199254740993,"f":1.0}`)
	_, err = s.jobs.CreateJob(s.ctx, key, changed)
	var conflict *generationv1.HTTPError
	if !errors.As(err, &conflict) || conflict.Status != 409 {
		t.Fatalf("changed request reused receipt: %v", err)
	}
	job := s.wait(t, admission.Job.JobID)
	if job.Status != generationv1.Succeeded || job.Artifact == nil || job.SourceContentID != snapshot.Manifest.ID || job.DerivedSnapshotID == "" || job.DerivedContentID == job.SourceContentID {
		t.Fatalf("render result: %+v", job)
	}
	derived, err := s.storage.GetSnapshot(s.ctx, job.DerivedSnapshotID)
	if err != nil {
		t.Fatal(err)
	}
	exact, _ := jsonvalue.NormalizeObject(parameters)
	found := false
	for _, ref := range derived.Resources {
		if ref.Tree == "project" && ref.Path == "input.json" {
			response, e := s.storage.ResourceContent(s.ctx, ref.ResourceID)
			if e != nil {
				t.Fatal(e)
			}
			body, e := io.ReadAll(response.Body)
			response.Body.Close()
			if e != nil || !bytes.Equal(body, exact) {
				t.Fatalf("parameter bytes changed: %q %v", body, e)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("derived input.json was not retained")
	}
	original, err := s.storage.GetSnapshot(s.ctx, snapshot.SnapshotID)
	if err != nil || original.Manifest.ID != snapshot.Manifest.ID {
		t.Fatalf("source mutated: %+v %v", original, err)
	}
	artifact, err := s.storage.GetArtifact(s.ctx, job.Artifact.ArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ExpiresAt.Sub(artifact.CreatedAt) != 24*time.Hour || !artifact.ExpiresAt.Equal(job.Artifact.ExpiresAt) || artifact.SnapshotID != job.DerivedSnapshotID || artifact.Metadata.InputFingerprint != job.InputFingerprint {
		t.Fatalf("artifact binding: %+v", artifact)
	}
	response, err := s.storage.ArtifactContent(s.ctx, artifact.ArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := io.ReadAll(response.Body)
	response.Body.Close()
	hash := sha256.Sum256(pdf)
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) || hex.EncodeToString(hash[:]) != artifact.Hash {
		t.Fatalf("PDF verification: %v", err)
	}
	s.projection(t, job)
	s.access(t, true, false)
	_, err = s.storage.ArtifactContent(s.ctx, artifact.ArtifactID)
	var denied *storagev1.HTTPError
	if !errors.As(err, &denied) || denied.Status != 404 {
		t.Fatalf("revoked access downloaded PDF: %v", err)
	}
	s.access(t, false, true)
	if _, err = s.storage.ArtifactContent(s.ctx, artifact.ArtifactID); err == nil {
		t.Fatal("dependency outage downloaded PDF")
	}
	s.access(t, false, false)
	canceled, err := s.jobs.CancelJob(s.ctx, job.JobID)
	if err != nil || canceled.Status != generationv1.Succeeded {
		t.Fatalf("late cancel changed committed success: %+v %v", canceled, err)
	}
	if evidence := os.Getenv("LEAFRUN_E04_RECEIPT_FILE"); evidence != "" {
		data, _ := json.Marshal(struct {
			JobID, ArtifactID, SHA256 string
			ExpiresAt                 time.Time
		}{job.JobID, artifact.ArtifactID, artifact.Hash, artifact.ExpiresAt})
		if err = os.WriteFile(evidence, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func (s *stand) projection(t *testing.T, job generationv1.Job) {
	t.Helper()
	dsn := os.Getenv("LEAFRUN_E04_CONSUMER_DATABASE_URL")
	if dsn == "" {
		t.Fatal("independent event consumer database is required")
	}
	pool, err := pgxpool.New(s.ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		var version int64
		var status, artifactID string
		err = pool.QueryRow(ctx, `SELECT version,status,artifact_id FROM dev_generation_projection WHERE job_id=$1`, job.JobID).Scan(&version, &status, &artifactID)
		if err == nil && version >= job.Version && status == string(job.Status) && artifactID == job.Artifact.ArtifactID {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("terminal outbox event did not reach independent inbox/effect: %v", err)
		case <-ticker.C:
		}
	}
}

func TestLiveRestartPersistence(t *testing.T) {
	receiptPath := os.Getenv("LEAFRUN_E04_RESTART_RECEIPT")
	if receiptPath == "" {
		t.Skip("restart verification has no saved receipt")
	}
	s := liveStand(t)
	data, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		JobID, ArtifactID, SHA256 string
		ExpiresAt                 time.Time
	}
	if err = json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	job, err := s.jobs.GetJob(s.ctx, receipt.JobID)
	if err != nil || job.Status != generationv1.Succeeded || job.Artifact == nil || job.Artifact.ArtifactID != receipt.ArtifactID || !job.Artifact.ExpiresAt.Equal(receipt.ExpiresAt) {
		t.Fatalf("job receipt changed after restart: %+v %v", job, err)
	}
	artifact, err := s.storage.GetArtifact(s.ctx, receipt.ArtifactID)
	if err != nil || artifact.Hash != receipt.SHA256 || !artifact.ExpiresAt.Equal(receipt.ExpiresAt) {
		t.Fatalf("artifact receipt changed after restart: %+v %v", artifact, err)
	}
	response, err := s.storage.ArtifactContent(s.ctx, receipt.ArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := io.ReadAll(response.Body)
	response.Body.Close()
	h := sha256.Sum256(pdf)
	if err != nil || hex.EncodeToString(h[:]) != receipt.SHA256 {
		t.Fatalf("PDF changed after restart: %v", err)
	}
	s.projection(t, job)
}

func TestLiveSchemaAndCompileFailuresAreAsynchronous(t *testing.T) {
	s := liveStand(t)
	for _, test := range []struct{ name, source, parameters, phase string }{{"schema", "This must never render", `{"n":1,"f":1.0,"label":"bad"}`, "validation"}, {"compile", "#unknown-function()", `{"n":9007199254740993,"f":1.0,"label":"bad"}`, "compile"}} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := s.snapshot(t, test.source)
			admission, err := s.jobs.CreateJob(s.ctx, fmt.Sprintf("e04-%s-%d", test.name, time.Now().UnixNano()), generationv1.CreateJobRequest{Scope: s.scope, SnapshotID: snapshot.SnapshotID, TargetID: "pdf", Entrypoint: "main.typ", Parameters: json.RawMessage(test.parameters), ParametersSchemaPath: "input.json"})
			if err != nil {
				t.Fatal("schema/compile error must be admitted asynchronously:", err)
			}
			job := s.wait(t, admission.Job.JobID)
			if job.Status != generationv1.Failed || job.Phase != test.phase || job.Artifact != nil {
				t.Fatalf("failed job: %+v", job)
			}
		})
	}
}
