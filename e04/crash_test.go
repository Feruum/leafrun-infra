package e04

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	generationv1 "github.com/leafrun/leafrun-contracts/generation/v1"
)

const controllerCrashSource = `#let a = 0
#while a < 10000 {
  a = a + 1
  let b = 0
  while b < 10000 {
    b = b + 1
    let c = 0
    while c < 10000 {
      c = c + 1
      ()
    }
    ()
  }
  ()
}`

var crashJobIDPattern = regexp.MustCompile(`^gen_[0-9a-f]{32}$`)

type crashReceipt struct {
	JobID string `json:"jobId"`
}

// TestLiveControllerCrashRecovery is split into submit and verify phases so
// the PowerShell driver can kill the real controller between them. It only
// writes a public job ID to the receipt file; the source and credentials stay
// out of the receipt and command output.
func TestLiveControllerCrashRecovery(t *testing.T) {
	receiptPath := os.Getenv("LEAFRUN_E04_CRASH_RECEIPT_FILE")
	stage := os.Getenv("LEAFRUN_E04_CRASH_STAGE")
	if receiptPath == "" || (stage != "submit" && stage != "verify") {
		t.Skip("explicit controller-crash verification stage is disabled")
	}
	s := liveStand(t)

	switch stage {
	case "submit":
		if err := os.Remove(receiptPath); err != nil && !os.IsNotExist(err) {
			t.Fatal("remove stale crash receipt:", err)
		}
		snapshot := s.snapshot(t, controllerCrashSource)
		admission, err := s.jobs.CreateJob(s.ctx, fmt.Sprintf("e04-controller-crash-%d", time.Now().UnixNano()), generationv1.CreateJobRequest{
			Scope: s.scope, SnapshotID: snapshot.SnapshotID, TargetID: "pdf", Entrypoint: "main.typ",
		})
		if err != nil {
			t.Fatal("admit watchdog job:", err)
		}
		if !crashJobIDPattern.MatchString(admission.Job.JobID) {
			t.Fatalf("generation returned an unexpected public job ID format")
		}
		if err = os.MkdirAll(filepath.Dir(receiptPath), 0700); err != nil {
			t.Fatal("create receipt directory:", err)
		}
		data, err := json.Marshal(crashReceipt{JobID: admission.Job.JobID})
		if err != nil {
			t.Fatal("encode crash receipt:", err)
		}
		if err = os.WriteFile(receiptPath, data, 0600); err != nil {
			t.Fatal("write crash receipt:", err)
		}
		t.Log("submitted bounded Typst watchdog job")
	case "verify":
		receipt := readCrashReceipt(t, receiptPath)
		job := waitCrashJob(t, s, receipt.JobID)
		if job.Status != generationv1.Failed || job.Phase != "resource_limit" || job.FailureCode != "resource_limit" || job.Artifact != nil {
			t.Fatalf("recovered job was not failed as resource_limit without an artifact: status=%s phase=%s failure=%s artifact=%t", job.Status, job.Phase, job.FailureCode, job.Artifact != nil)
		}
		assertSingleStoppedAttempt(t, s.ctx, receipt.JobID)
		t.Log("recovery persisted one stopped attempt and failed without an artifact")
	}
}

func readCrashReceipt(t *testing.T, path string) crashReceipt {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal("read crash receipt:", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 4096))
	decoder.DisallowUnknownFields()
	var receipt crashReceipt
	if err = decoder.Decode(&receipt); err != nil {
		t.Fatal("decode crash receipt:", err)
	}
	if !crashJobIDPattern.MatchString(receipt.JobID) {
		t.Fatal("crash receipt has an invalid public job ID")
	}
	return receipt
}

func waitCrashJob(t *testing.T, s *stand, id string) generationv1.Job {
	t.Helper()
	ctx, cancel := context.WithTimeout(s.ctx, 75*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var last generationv1.Job
	for {
		job, err := s.jobs.GetJob(ctx, id)
		if err != nil {
			t.Fatal("read recovered job:", err)
		}
		last = job
		if job.Status.Terminal() {
			return job
		}
		select {
		case <-ctx.Done():
			t.Fatalf("recovered job did not become terminal (status=%s phase=%s)", last.Status, last.Phase)
		case <-ticker.C:
		}
	}
}

func assertSingleStoppedAttempt(t *testing.T, parent context.Context, jobID string) {
	t.Helper()
	dsn := os.Getenv("LEAFRUN_E04_CRASH_GENERATION_DATABASE_URL")
	if dsn == "" {
		t.Fatal("internal generation database access is required for the attempt-count check")
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("open internal generation database:", err)
	}
	defer pool.Close()
	var count, maxNumber int
	var allStopped bool
	err = pool.QueryRow(ctx, `SELECT count(*),COALESCE(max(attempt_number),0),COALESCE(bool_and(state='stopped' AND stop_confirmed),false) FROM generation_attempts WHERE job_id=$1`, jobID).Scan(&count, &maxNumber, &allStopped)
	if err != nil {
		t.Fatal("read internal attempt recovery state:", err)
	}
	if count != 1 || maxNumber != 1 || !allStopped {
		t.Fatalf("crash recovery created or left an unexpected attempt: count=%d max_attempt=%d all_stopped=%t", count, maxNumber, allStopped)
	}
}
