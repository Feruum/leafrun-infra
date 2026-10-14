package e04events

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	generationv1 "github.com/leafrun/leafrun-contracts/generation/v1"
	"github.com/leafrun/leafrun-contracts/jsonvalue/v1"
	"github.com/leafrun/leafrun-platform/events"
)

type JobOwner interface {
	GetJob(context.Context, string) (generationv1.Job, error)
}

func MigrateProjection(ctx context.Context, db interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}) error {
	_, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS dev_generation_projection (
		job_id text PRIMARY KEY,workspace_id text NOT NULL,project_id text NOT NULL,
		version bigint NOT NULL CHECK(version>=0),status text NOT NULL,artifact_id text NOT NULL DEFAULT '',updated_at timestamptz NOT NULL DEFAULT clock_timestamp())`)
	return err
}

// ProjectionHandler stores only IDs, version and safe status in the consumer's
// own database. Generation remains the authority; event gaps trigger an API
// refresh within the same effect/inbox transaction.
func ProjectionHandler(owner JobOwner) events.Handler {
	return func(ctx context.Context, tx pgx.Tx, e events.Envelope) error {
		if owner == nil || e.EventType != events.GenerationCompletedEventType || e.Producer != "leafrun-generation" {
			return errors.New("unsupported Generation event")
		}
		var data events.GenerationCompletedData
		if err := jsonvalue.Unmarshal(e.Data, &data, events.MaxDataBytes); err != nil || data.JobID != e.AggregateID {
			return errors.New("invalid Generation event binding")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO dev_generation_projection(job_id,workspace_id,project_id,version,status) VALUES($1,$2,$3,0,'pending') ON CONFLICT(job_id) DO NOTHING`, e.AggregateID, e.WorkspaceID, e.ProjectID); err != nil {
			return err
		}
		var version uint64
		var workspace, project string
		if err := tx.QueryRow(ctx, `SELECT version,workspace_id,project_id FROM dev_generation_projection WHERE job_id=$1 FOR UPDATE`, e.AggregateID).Scan(&version, &workspace, &project); err != nil {
			return err
		}
		if workspace != e.WorkspaceID || project != e.ProjectID {
			return errors.New("event scope conflicts with projection")
		}
		if e.AggregateVersion <= version {
			return nil
		}
		job, err := owner.GetJob(ctx, e.AggregateID)
		if err != nil {
			return err
		}
		if job.JobID != e.AggregateID || job.Scope.WorkspaceID != e.WorkspaceID || job.Scope.ProjectID != e.ProjectID || job.Version < int64(e.AggregateVersion) || !job.Status.Terminal() {
			return errors.New("Generation owner has not confirmed this terminal event")
		}
		artifactID := ""
		if job.Artifact != nil {
			artifactID = job.Artifact.ArtifactID
		}
		_, err = tx.Exec(ctx, `UPDATE dev_generation_projection SET version=$2,status=$3,artifact_id=$4,updated_at=clock_timestamp() WHERE job_id=$1`, job.JobID, job.Version, string(job.Status), artifactID)
		return err
	}
}
