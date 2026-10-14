package e04events

import (
	"context"
	"testing"

	generationv1 "github.com/leafrun/leafrun-contracts/generation/v1"
	"github.com/leafrun/leafrun-platform/events"
)

type projectionOwner struct {
	job   generationv1.Job
	calls int
}

func (o *projectionOwner) GetJob(context.Context, string) (generationv1.Job, error) {
	o.calls++
	return o.job, nil
}

func TestLiveProjectionRepairsGapAndIgnoresReorderedEvent(t *testing.T) {
	ctx, pool, _, _, _, id, _ := liveFixture(t)
	if err := MigrateProjection(ctx, pool); err != nil {
		t.Fatal(err)
	}
	e := liveEnvelope(id)
	e.AggregateVersion = 8
	owner := &projectionOwner{job: generationv1.Job{JobID: e.AggregateID, Scope: generationv1.Scope{WorkspaceID: e.WorkspaceID, ProjectID: e.ProjectID}, Status: generationv1.Succeeded, Version: 9}}
	consumer := events.Consumer{DB: pool, Name: "projection", Handler: ProjectionHandler(owner)}
	if disposition, err := consumer.Process(ctx, e); err != nil || disposition != events.DispositionApplied {
		t.Fatalf("gap resync: %s %v", disposition, err)
	}
	e.EventID += "-old"
	e.AggregateVersion = 3
	if disposition, err := consumer.Process(ctx, e); err != nil || disposition != events.DispositionApplied {
		t.Fatalf("reordered: %s %v", disposition, err)
	}
	var version int64
	var status string
	if err := pool.QueryRow(ctx, `SELECT version,status FROM dev_generation_projection WHERE job_id=$1`, e.AggregateID).Scan(&version, &status); err != nil || version != 9 || status != "succeeded" || owner.calls != 1 {
		t.Fatalf("projection regressed: version=%d status=%s calls=%d err=%v", version, status, owner.calls, err)
	}
}

func TestLiveProjectionRejectsMismatchedOwnerScope(t *testing.T) {
	ctx, pool, _, _, _, id, _ := liveFixture(t)
	if err := MigrateProjection(ctx, pool); err != nil {
		t.Fatal(err)
	}
	e := liveEnvelope(id)
	owner := &projectionOwner{job: generationv1.Job{JobID: e.AggregateID, Scope: generationv1.Scope{WorkspaceID: "different", ProjectID: e.ProjectID}, Status: generationv1.Succeeded, Version: 9}}
	consumer := events.Consumer{DB: pool, Name: "projection", Handler: ProjectionHandler(owner)}
	if disposition, err := consumer.Process(ctx, e); err != nil || disposition != events.DispositionRetry {
		t.Fatalf("wrong owner binding: %s %v", disposition, err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM dev_generation_projection`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed owner lookup committed effect: %d %v", count, err)
	}
}
