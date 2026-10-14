package e04

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	generationv1 "github.com/leafrun/leafrun-contracts/generation/v1"
	storagev1 "github.com/leafrun/leafrun-contracts/storage/v1"
)

var brokerOutageJobIDPattern = regexp.MustCompile(`^gen_[0-9a-f]{32}$`)

type brokerOutageReceipt struct {
	JobID      string    `json:"jobId"`
	ArtifactID string    `json:"artifactId"`
	SHA256     string    `json:"sha256"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

// TestLiveBrokerOutageRecovery is driven in two phases by broker-outage-check.ps1.
// The submit phase runs with NATS stopped and proves Generation and Storage
// complete independently of event delivery. The verify phase runs after the
// original NATS container is healthy again and proves the same render's event
// reaches the independent consumer.
func TestLiveBrokerOutageRecovery(t *testing.T) {
	receiptPath := os.Getenv("LEAFRUN_E04_BROKER_RECEIPT_FILE")
	stage := os.Getenv("LEAFRUN_E04_BROKER_STAGE")
	if receiptPath == "" || (stage != "submit" && stage != "verify") {
		t.Skip("explicit broker-outage verification stage is disabled")
	}
	s := liveStand(t)

	switch stage {
	case "submit":
		if err := os.Remove(receiptPath); err != nil && !os.IsNotExist(err) {
			t.Fatal("remove stale broker-outage receipt")
		}
		snapshot := s.snapshot(t, "#set page(width: 80mm, height: 35mm, margin: 4mm)\nBroker recovery preserves this accepted PDF.")
		admission, err := s.jobs.CreateJob(s.ctx, "e04-broker-outage-"+time.Now().UTC().Format("20060102T150405.000000000"), generationv1.CreateJobRequest{
			Scope: s.scope, SnapshotID: snapshot.SnapshotID, TargetID: "pdf", Entrypoint: "main.typ",
		})
		if err != nil {
			t.Fatal("Generation did not accept the outage job")
		}
		if !brokerOutageJobIDPattern.MatchString(admission.Job.JobID) {
			t.Fatal("Generation returned an unexpected public job ID")
		}

		job := waitBrokerOutageJob(t, s, admission.Job.JobID, 65*time.Second)
		if job.Status != generationv1.Succeeded || job.Artifact == nil {
			t.Fatalf("accepted job did not succeed while NATS was unavailable: status=%s phase=%s artifact=%t", job.Status, job.Phase, job.Artifact != nil)
		}
		artifact := verifyBrokerOutageArtifact(t, s, job, job.Artifact.ArtifactID, job.Artifact.SHA256, job.Artifact.ExpiresAt)
		assertPendingBrokerOutbox(t, s.ctx, admission.Job.JobID, artifact.ArtifactID)
		writeBrokerOutageReceipt(t, receiptPath, brokerOutageReceipt{
			JobID: admission.Job.JobID, ArtifactID: artifact.ArtifactID,
			SHA256: artifact.Hash, ExpiresAt: artifact.ExpiresAt,
		})
		t.Log("accepted job and Storage artifact succeeded while the terminal event remained durably pending")

	case "verify":
		receipt := readBrokerOutageReceipt(t, receiptPath)
		job, err := s.jobs.GetJob(s.ctx, receipt.JobID)
		if err != nil {
			t.Fatal("read accepted job after NATS recovery")
		}
		if job.Status != generationv1.Succeeded || job.Artifact == nil || job.Artifact.ArtifactID != receipt.ArtifactID || job.Artifact.SHA256 != receipt.SHA256 || !job.Artifact.ExpiresAt.Equal(receipt.ExpiresAt) {
			t.Fatalf("accepted job receipt changed after NATS recovery: status=%s artifact=%t", job.Status, job.Artifact != nil)
		}
		artifact := verifyBrokerOutageArtifact(t, s, job, receipt.ArtifactID, receipt.SHA256, receipt.ExpiresAt)
		assertSingleBrokerOutageAttempt(t, s.ctx, receipt.JobID)
		waitBrokerOutboxSent(t, s.ctx, receipt.JobID, receipt.ArtifactID, 70*time.Second)
		s.projection(t, job)
		t.Logf("NATS recovery delivered the existing terminal event for artifact %s", artifact.ArtifactID)
	}
}

func waitBrokerOutageJob(t *testing.T, s *stand, jobID string, timeout time.Duration) generationv1.Job {
	t.Helper()
	ctx, cancel := context.WithTimeout(s.ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var last generationv1.Job
	for {
		job, err := s.jobs.GetJob(ctx, jobID)
		if err != nil {
			t.Fatal("read accepted broker-outage job")
		}
		last = job
		if job.Status.Terminal() {
			return job
		}
		select {
		case <-ctx.Done():
			t.Fatalf("accepted job did not become terminal while NATS was unavailable: status=%s phase=%s", last.Status, last.Phase)
		case <-ticker.C:
		}
	}
}

func verifyBrokerOutageArtifact(t *testing.T, s *stand, job generationv1.Job, wantID, wantHash string, wantExpiry time.Time) storagev1.Artifact {
	t.Helper()
	if job.Artifact == nil || job.Artifact.ArtifactID != wantID || job.Artifact.SHA256 != wantHash || !job.Artifact.ExpiresAt.Equal(wantExpiry) {
		t.Fatal("Generation artifact identity, hash, or expiry changed")
	}
	artifact, err := s.storage.GetArtifact(s.ctx, wantID)
	if err != nil {
		t.Fatal("read Storage artifact")
	}
	if artifact.ArtifactID != wantID || artifact.OriginJobID != job.JobID || artifact.Hash != wantHash || !artifact.ExpiresAt.Equal(wantExpiry) || artifact.ExpiresAt.Sub(artifact.CreatedAt) != 24*time.Hour {
		t.Fatal("Storage artifact identity, hash, or fixed 24-hour expiry changed")
	}
	response, err := s.storage.ArtifactContent(s.ctx, wantID)
	if err != nil {
		t.Fatal("download accepted PDF from Storage")
	}
	pdf, readErr := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		t.Fatal("read accepted PDF from Storage")
	}
	hash := sha256.Sum256(pdf)
	if len(pdf) < 5 || string(pdf[:5]) != "%PDF-" || hex.EncodeToString(hash[:]) != wantHash {
		t.Fatal("accepted PDF bytes or content hash changed")
	}
	return artifact
}

func brokerGenerationPool(t *testing.T, parent context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("LEAFRUN_E04_BROKER_GENERATION_DATABASE_URL")
	if dsn == "" {
		t.Fatal("internal Generation database access is required for outbox verification")
	}
	pool, err := pgxpool.New(parent, dsn)
	if err != nil {
		t.Fatal("open internal Generation database")
	}
	t.Cleanup(pool.Close)
	return pool
}

type brokerOutboxState struct {
	status, artifactID string
	rows, pending      int64
}

func readBrokerOutboxState(ctx context.Context, pool *pgxpool.Pool, jobID string) (brokerOutboxState, error) {
	var state brokerOutboxState
	err := pool.QueryRow(ctx, `SELECT j.status,j.artifact_id,count(*)::bigint,
		count(*) FILTER (WHERE o.sent_at IS NULL)::bigint
		FROM generation_jobs j
		JOIN generation_terminal_outbox t ON t.job_id=j.job_id
		JOIN platform_event_outbox o ON o.event_id=t.event_id
		WHERE j.job_id=$1 GROUP BY j.status,j.artifact_id`, jobID).Scan(&state.status, &state.artifactID, &state.rows, &state.pending)
	return state, err
}

func assertPendingBrokerOutbox(t *testing.T, parent context.Context, jobID, artifactID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	pool := brokerGenerationPool(t, ctx)
	state, err := readBrokerOutboxState(ctx, pool, jobID)
	if err != nil {
		t.Fatal("read terminal outbox row while NATS was unavailable")
	}
	if state.status != "succeeded" || state.artifactID != artifactID || state.rows != 1 || state.pending != 1 {
		t.Fatalf("terminal outbox was not durably pending: status=%s artifact_match=%t rows=%d pending=%d", state.status, state.artifactID == artifactID, state.rows, state.pending)
	}
}

func assertSingleBrokerOutageAttempt(t *testing.T, parent context.Context, jobID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	pool := brokerGenerationPool(t, ctx)
	var count, maxNumber int
	var allFinished bool
	err := pool.QueryRow(ctx, `SELECT count(*),COALESCE(max(attempt_number),0),
		COALESCE(bool_and(state='finished' AND stop_confirmed),false)
		FROM generation_attempts WHERE job_id=$1`, jobID).Scan(&count, &maxNumber, &allFinished)
	if err != nil {
		t.Fatal("read render attempt count after NATS recovery")
	}
	if count != 1 || maxNumber != 1 || !allFinished {
		t.Fatalf("NATS recovery caused an unexpected render attempt: count=%d max_attempt=%d all_finished=%t", count, maxNumber, allFinished)
	}
}

func waitBrokerOutboxSent(t *testing.T, parent context.Context, jobID, artifactID string, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	pool := brokerGenerationPool(t, ctx)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, err := readBrokerOutboxState(ctx, pool, jobID)
		if err == nil && state.status == "succeeded" && state.artifactID == artifactID && state.rows == 1 && state.pending == 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("terminal outbox event was not acknowledged after NATS recovery: status=%s artifact_match=%t rows=%d pending=%d", state.status, state.artifactID == artifactID, state.rows, state.pending)
		case <-ticker.C:
		}
	}
}

func writeBrokerOutageReceipt(t *testing.T, path string, receipt brokerOutageReceipt) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal("create broker-outage receipt directory")
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal("encode broker-outage receipt")
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal("write broker-outage receipt")
	}
}

func readBrokerOutageReceipt(t *testing.T, path string) brokerOutageReceipt {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal("read broker-outage receipt")
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 4096))
	decoder.DisallowUnknownFields()
	var receipt brokerOutageReceipt
	if err = decoder.Decode(&receipt); err != nil {
		t.Fatal("decode broker-outage receipt")
	}
	if !brokerOutageJobIDPattern.MatchString(receipt.JobID) || receipt.ArtifactID == "" || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(receipt.SHA256) || receipt.ExpiresAt.IsZero() {
		t.Fatal("broker-outage receipt has invalid public artifact data")
	}
	if err = decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatal("broker-outage receipt contains trailing data")
	}
	return receipt
}
