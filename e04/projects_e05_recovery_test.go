package e04

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	generationv1 "github.com/leafrun/leafrun-contracts/generation/v1"
	projectsv1 "github.com/leafrun/leafrun-contracts/projects/v1"
	storagev1 "github.com/leafrun/leafrun-contracts/storage/v1"
)

// Only public IDs and test fixture inputs are persisted across stand restarts.
type e05RestartReceipt struct {
	Scope            storagev1.Scope                     `json:"scope"`
	ReleaseID        string                              `json:"releaseId"`
	SnapshotID       string                              `json:"snapshotId"`
	ContentID        string                              `json:"contentId"`
	DescriptorDigest string                              `json:"descriptorDigest"`
	JobID            string                              `json:"jobId"`
	DerivedContentID string                              `json:"derivedContentId"`
	ArtifactID       string                              `json:"artifactId"`
	SHA256           string                              `json:"sha256"`
	ExpiresAt        time.Time                           `json:"expiresAt"`
	IdempotencyKey   string                              `json:"idempotencyKey"`
	Selection        generationv1.RenderSelectionRequest `json:"selection"`
	ResourceID       string                              `json:"resourceId"`
}

func writeE05Receipt(t *testing.T, path string, receipt any) {
	t.Helper()
	data, err := json.Marshal(receipt)
	if err != nil || len(data) > 2<<20 {
		t.Fatal("encode bounded E05 fixture receipt")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal("prepare E05 receipt directory")
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal("write E05 fixture receipt")
	}
}

func readE05Receipt(t *testing.T, path string, receipt any) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal("open E05 fixture receipt")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	if err != nil || len(data) > 2<<20 || json.Unmarshal(data, receipt) != nil {
		t.Fatal("read bounded E05 fixture receipt")
	}
}

func TestE05RestartPersistence(t *testing.T) {
	path := os.Getenv("LEAFRUN_E05_RESTART_RECEIPT")
	if path == "" {
		t.Skip("explicit E05 restart receipt is disabled")
	}
	s, projects := e05Stand(t)
	var receipt e05RestartReceipt
	readE05Receipt(t, path, &receipt)
	if receipt.Scope != s.scope || receipt.Selection.ReleaseID != receipt.ReleaseID || receipt.Selection.Scope != s.scope {
		t.Fatal("restart receipt scope or release binding changed")
	}
	s.e05PermitGenerator(t, receipt.ResourceID)
	release, err := projects.GetRelease(s.ctx, receipt.ReleaseID)
	if err != nil || !release.Published || release.State != "ready" || release.SnapshotID != receipt.SnapshotID ||
		release.ContentID != receipt.ContentID || release.DescriptorDigest != receipt.DescriptorDigest {
		t.Fatal("release pin, publication or frozen identity changed after restart")
	}
	callerProjects, err := projectsv1.NewClient(os.Getenv("LEAFRUN_E05_PROJECTS_URL"), os.Getenv("GENERATION_ONLY_USER_TOKEN"), nil)
	if err != nil {
		t.Fatal(err)
	}
	form, err := callerProjects.Form(s.ctx, receipt.ReleaseID)
	if err != nil || form.DescriptorDigest != receipt.DescriptorDigest || form.Profiles["archive"].Inputs["quality"] != "archive" {
		t.Fatal("frozen form/profile changed after restart")
	}
	generator, err := generationv1.NewClient(os.Getenv("LEAFRUN_E04_GENERATION_URL"), os.Getenv("GENERATION_ONLY_USER_TOKEN"), nil)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := generator.CreateRender(s.ctx, receipt.IdempotencyKey, receipt.Selection)
	if err != nil || replay.Job.JobID != receipt.JobID || replay.Job.Status != generationv1.Succeeded || replay.Job.Artifact == nil {
		t.Fatal("original render selection did not recover the same successful job")
	}
	job, err := s.jobs.GetJob(s.ctx, receipt.JobID)
	if err != nil {
		t.Fatal(err)
	}
	derived, err := s.storage.GetSnapshot(s.ctx, job.DerivedSnapshotID)
	if err != nil || derived.Manifest.ID != receipt.DerivedContentID {
		t.Fatal("prepared input snapshot changed after restart")
	}
	artifact := verifyBrokerOutageArtifact(t, s, job, receipt.ArtifactID, receipt.SHA256, receipt.ExpiresAt)
	callerStorage, err := storagev1.NewClient(os.Getenv("LEAFRUN_E04_STORAGE_URL"), os.Getenv("GENERATION_ONLY_USER_TOKEN"), nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := callerStorage.ArtifactContent(s.ctx, artifact.ArtifactID)
	if err != nil {
		t.Fatal("generation-only artifact grant did not survive restart")
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	_ = response.Body.Close()
	hash := sha256.Sum256(data)
	if readErr != nil || !bytes.HasPrefix(data, []byte("%PDF-")) || hex.EncodeToString(hash[:]) != receipt.SHA256 {
		t.Fatal("generation-only PDF bytes changed after restart")
	}
	t.Log("Projects release, prepared snapshot, original admission receipt and exact PDF/expiry survived restart")
}
