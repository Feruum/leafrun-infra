package e05events

import (
	"context"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const StreamName = "LEAFRUN_PROJECTS"

type StreamManager interface {
	CreateOrUpdateStream(context.Context, jetstream.StreamConfig) (jetstream.Stream, error)
}

// Projects events contain IDs and mutable aggregate versions only. The API is
// the source of current state; this bounded 30-day stream permits later owners
// to subscribe without silently dropping an event when the stream is full.
// A rejected publish remains in the owner's transactional PostgreSQL outbox.
func Provision(ctx context.Context, manager StreamManager) error {
	_, err := manager.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: StreamName, Subjects: []string{"leafrun.projects.>"},
		Storage: jetstream.FileStorage, Retention: jetstream.LimitsPolicy,
		Discard: jetstream.DiscardNew, MaxAge: 30 * 24 * time.Hour,
		MaxBytes: 256 << 20, Duplicates: 2 * time.Minute,
	})
	return err
}
