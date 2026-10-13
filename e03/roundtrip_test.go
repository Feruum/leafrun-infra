// Package e03 exercises Storage through its HTTP SDK and Core through public APIs.
package e03

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	v "github.com/leafrun/leafrun-contracts/storage/v1"
	core "github.com/leafrun/leafrun-document-core"
	"github.com/leafrun/leafrun-infra/internal/fixture"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestRoundTripAndPublication(t *testing.T) {
	if os.Getenv("LEAFRUN_E03_INTEGRATION") != "1" {
		t.Skip("requires explicit dev Compose stand")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client, err := v.NewClient(os.Getenv("LEAFRUN_E03_STORAGE_URL"), os.Getenv("STORAGE_TOKEN"), nil)
	if err != nil {
		t.Fatal(err)
	}
	scope := v.Scope{WorkspaceID: "demo", ProjectID: "roundtrip"}
	state := fixture.State{Scope: scope, Published: map[string]string{}, Pins: map[string]v.OwnerState{}}
	control := func() {
		t.Helper()
		controlCtx, controlCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer controlCancel()
		b, _ := json.Marshal(state)
		r, e := http.NewRequestWithContext(controlCtx, "PUT", os.Getenv("LEAFRUN_E03_OWNER_URL")+"/dev/v1/state", bytes.NewReader(b))
		if e != nil {
			t.Fatal(e)
		}
		r.Header.Set("Authorization", "Bearer "+os.Getenv("FIXTURE_ADMIN_TOKEN"))
		r.Header.Set("Content-Type", "application/json")
		res, e := http.DefaultClient.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		if res.StatusCode != 204 {
			t.Fatalf("fixture update status %d", res.StatusCode)
		}
	}
	control()
	t.Cleanup(func() {
		state.Deny = false
		state.Unavailable = false
		state.Published = map[string]string{}
		state.Pins = map[string]v.OwnerState{}
		control()
	})
	project, external := t.TempDir(), t.TempDir()
	write := func(name string, b []byte) {
		t.Helper()
		if e := os.MkdirAll(filepath.Dir(name), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(name, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	write(filepath.Join(project, "main.typ"), []byte("#set text(font: \"Fira Sans\")\n#let data = json(\"data.json\")\nCaptured #data.label\n#image(\"mark.svg\")"))
	write(filepath.Join(project, "data.json"), []byte(`{"label":"Storage M03"}`))
	write(filepath.Join(project, "mark.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20"><rect width="40" height="20" fill="navy"/></svg>`))
	if err = os.MkdirAll(filepath.Join(project, "empty", "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	font, err := os.ReadFile(os.Getenv("LEAFRUN_E03_FONT_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(external, "Fira.ttf"), font)
	if err = os.Mkdir(filepath.Join(external, "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	captured, err := core.CreateSnapshot(ctx, core.FolderSource{Root: project}, core.SnapshotOptions{FontPaths: []string{external}})
	if err != nil {
		t.Fatal(err)
	}
	defer captured.Close()
	// Capture must survive the complete loss of the original external font folder.
	if err = os.RemoveAll(external); err != nil {
		t.Fatal(err)
	}
	manifest := captured.Manifest()
	wire, _ := json.Marshal(manifest)
	var transport v.SnapshotManifest
	if err = json.Unmarshal(wire, &transport); err != nil {
		t.Fatal(err)
	}
	contentID, err := v.ManifestContentID(transport)
	if err != nil || contentID != manifest.ID {
		t.Fatalf("cross-module content ID: %s %v", contentID, err)
	}
	refs := []v.ResourceRef{}
	upload := func(class v.ObjectClass, data []byte) v.Upload {
		t.Helper()
		h := sha256.Sum256(data)
		u, e := client.CreateUpload(ctx, v.CreateUploadRequest{Scope: scope, Class: class, SizeBytes: int64(len(data)), SHA256: hex.EncodeToString(h[:]), IdempotencyKey: fmt.Sprintf("%d-%s", time.Now().UnixNano(), class)})
		if e != nil {
			t.Fatal(e)
		}
		out, e := client.PutUpload(ctx, u.UploadID, u.WriterToken, bytes.NewReader(data))
		if e != nil {
			t.Fatal(e)
		}
		out.WriterToken = u.WriterToken
		return out
	}
	sendTree := func(tree fs.FS, files []core.SnapshotFile, name string, index int) {
		t.Helper()
		for _, file := range files {
			b, e := fs.ReadFile(tree, file.Path)
			if e != nil {
				t.Fatal(e)
			}
			u := upload(v.ClassResource, b)
			resource, e := client.CommitResource(ctx, u.UploadID, v.CommitResourceRequest{WriterToken: u.WriterToken, IdempotencyKey: u.UploadID})
			if e != nil {
				t.Fatal(e)
			}
			refs = append(refs, v.ResourceRef{Tree: name, FontIndex: index, Path: file.Path, ResourceID: resource.ResourceID})
		}
	}
	projectFS := captured.FS()
	sendTree(projectFS, manifest.Files, "project", 0)
	for i, f := range manifest.Fonts {
		if f.InProject {
			continue
		}
		fontFS, e := captured.FontFS(i)
		if e != nil {
			t.Fatal(e)
		}
		sendTree(fontFS, f.Files, "font", i)
	}
	record, err := client.CreateSnapshot(ctx, v.CreateSnapshotRequest{Scope: scope, Manifest: transport, Resources: refs, IdempotencyKey: fmt.Sprint(time.Now().UnixNano())})
	if err != nil {
		t.Fatal(err)
	}
	if record.SnapshotID == manifest.ID {
		t.Fatal("scoped record ID confused with content ID")
	}
	pin, err := client.AcquirePin(ctx, v.AcquirePinRequest{Scope: scope, ObjectID: record.SnapshotID, HolderID: "demo", HolderDomain: "core", RetentionPolicy: "release", IdempotencyKey: record.SnapshotID})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the owner saving the pending reference before confirming activation.
	state.Pins[pin.PinID] = v.OwnerReferenced
	control()
	active, err := client.ActivatePin(ctx, pin.PinID)
	if err != nil || active.State != "active" {
		t.Fatalf("activation: %+v %v", active, err)
	}
	receipt, err := client.GetPin(ctx, pin.PinID)
	if err != nil || receipt.State != "active" {
		t.Fatalf("pin receipt %v", err)
	}
	restoredProject := fstest.MapFS{}
	fonts := map[int]fs.FS{}
	for _, dir := range manifest.Directories {
		restoredProject[dir] = &fstest.MapFile{Mode: fs.ModeDir | 0700}
	}
	for i, f := range manifest.Fonts {
		if f.InProject {
			continue
		}
		m := fstest.MapFS{}
		for _, dir := range f.Directories {
			m[dir] = &fstest.MapFile{Mode: fs.ModeDir | 0700}
		}
		fonts[i] = m
	}
	for _, ref := range record.Resources {
		res, e := client.ResourceContent(ctx, ref.ResourceID)
		if e != nil {
			t.Fatal(e)
		}
		b, e := io.ReadAll(io.LimitReader(res.Body, 1<<28))
		_ = res.Body.Close()
		if e != nil {
			t.Fatal(e)
		}
		entry := &fstest.MapFile{Data: b, Mode: 0600}
		if ref.Tree == "project" {
			restoredProject[ref.Path] = entry
		} else {
			fonts[ref.FontIndex].(fstest.MapFS)[ref.Path] = entry
		}
	}
	restored, err := core.RestoreSnapshot(ctx, core.SnapshotBundle{Manifest: manifest, ProjectFS: restoredProject, ExternalFontFS: fonts}, core.RestoreSnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if restored.Manifest().ID != manifest.ID {
		t.Fatal("restore changed content identity")
	}
	pdfPath := filepath.Join(t.TempDir(), "render.pdf")
	render, err := core.RenderSnapshot(ctx, restored, core.RenderSnapshotOptions{Output: pdfPath})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(render.RawOutput, "unknown font family") {
		t.Fatal("external font unavailable")
	}
	pdf, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("invalid PDF")
	}
	u := upload(v.ClassResult, pdf)
	input := v.FinalizeArtifactRequest{WriterToken: u.WriterToken, OriginJobID: u.UploadID, SnapshotID: record.SnapshotID, Metadata: v.ArtifactMetadata{Filename: "render.pdf", MediaType: "application/pdf", TargetID: "main", AttemptID: u.UploadID}, IdempotencyKey: u.UploadID}
	artifact, err := client.FinalizeArtifact(ctx, u.UploadID, input)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := client.FinalizeArtifact(ctx, u.UploadID, input)
	if err != nil || replay.ArtifactID != artifact.ArtifactID || !replay.ExpiresAt.Equal(artifact.ExpiresAt) {
		t.Fatalf("finalize receipt changed: %v", err)
	}
	requireBlocked := func() {
		t.Helper()
		response, e := client.ArtifactContent(ctx, artifact.ArtifactID)
		if response != nil {
			_ = response.Body.Close()
		}
		var status *v.HTTPError
		if !errors.As(e, &status) || status.Status < 400 {
			t.Fatalf("bytes escaped checks: %v", e)
		}
	}
	requireBlocked()
	state.Published[artifact.ArtifactID] = artifact.OriginJobID
	control()
	res, err := client.ArtifactContent(ctx, artifact.ArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	downloaded, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil || !bytes.Equal(pdf, downloaded) {
		t.Fatal("downloaded PDF differs")
	}
	state.Deny = true
	control()
	requireBlocked()
	state.Deny = false
	state.Unavailable = true
	control()
	requireBlocked()
	state.Unavailable = false
	control()
	// A failed subsequent render leaves the previous published PDF available.
	if _, err = core.RenderSnapshot(ctx, restored, core.RenderSnapshotOptions{Entrypoint: "missing.typ", Output: pdfPath}); err == nil {
		t.Fatal("missing entrypoint rendered")
	}
	after, _ := os.ReadFile(pdfPath)
	if !bytes.Equal(pdf, after) {
		t.Fatal("failed next render replaced PDF")
	}
	res, err = client.ArtifactContent(ctx, artifact.ArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if name := os.Getenv("LEAFRUN_E03_RECEIPT"); name != "" {
		b, e := json.Marshal(restartReceipt{SnapshotID: record.SnapshotID, ArtifactID: artifact.ArtifactID, JobID: artifact.OriginJobID, PinID: pin.PinID, ContentID: manifest.ID, Hash: artifact.Hash, ExpiresAt: artifact.ExpiresAt})
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(name, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	t.Logf("D04/D10/D15/D18: full captured bundle %s, scoped snapshot %s, pin receipt, frozen fonts, publication/access fail-closed and original finalize receipt verified", manifest.ID, record.SnapshotID)
}

type restartReceipt struct {
	SnapshotID, ArtifactID, JobID, PinID, ContentID, Hash string
	ExpiresAt                                             time.Time
}

func TestRestartPersistence(t *testing.T) {
	name := os.Getenv("LEAFRUN_E03_RESTART_RECEIPT")
	if name == "" {
		t.Skip("requires completed round trip and restarted persistent services")
	}
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var receipt restartReceipt
	if err = json.Unmarshal(b, &receipt); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := v.NewClient(os.Getenv("LEAFRUN_E03_STORAGE_URL"), os.Getenv("STORAGE_TOKEN"), nil)
	if err != nil {
		t.Fatal(err)
	}
	state := fixture.State{Scope: v.Scope{WorkspaceID: "demo", ProjectID: "roundtrip"}, Published: map[string]string{receipt.ArtifactID: receipt.JobID}, Pins: map[string]v.OwnerState{receipt.PinID: v.OwnerReferenced}}
	body, _ := json.Marshal(state)
	r, err := http.NewRequestWithContext(ctx, "PUT", os.Getenv("LEAFRUN_E03_OWNER_URL")+"/dev/v1/state", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer "+os.Getenv("FIXTURE_ADMIN_TOKEN"))
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 204 {
		t.Fatal(res.StatusCode)
	}
	snapshot, err := client.GetSnapshot(ctx, receipt.SnapshotID)
	if err != nil || snapshot.Manifest.ID != receipt.ContentID {
		t.Fatalf("snapshot persistence: %v", err)
	}
	pin, err := client.GetPin(ctx, receipt.PinID)
	if err != nil || pin.State != "active" {
		t.Fatalf("pin persistence: %v", err)
	}
	artifact, err := client.GetArtifact(ctx, receipt.ArtifactID)
	if err != nil || !artifact.ExpiresAt.Equal(receipt.ExpiresAt) {
		t.Fatalf("original expiry persistence: %v", err)
	}
	res, err = client.ArtifactContent(ctx, receipt.ArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	_, err = io.Copy(h, res.Body)
	_ = res.Body.Close()
	if err != nil || hex.EncodeToString(h.Sum(nil)) != receipt.Hash {
		t.Fatalf("exact object version persistence: %v", err)
	}
	t.Log("snapshot, activated pin, original PDF expiry and exact S3 bytes survived service restarts")
}
