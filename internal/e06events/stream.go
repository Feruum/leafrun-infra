package e06events

import (
	"context"
	"errors"
	"github.com/nats-io/nats.go/jetstream"
	"time"
)

type StreamManager interface {
	CreateOrUpdateStream(context.Context, jetstream.StreamConfig) (jetstream.Stream, error)
	CreateOrUpdateConsumer(context.Context, string, jetstream.ConsumerConfig) (jetstream.Consumer, error)
}

const StreamName = "LEAFRUN_AUDIT"
const ConsumerName = "LEAFRUN_AUDIT_V1"
const DefaultRetention = 30 * 24 * time.Hour

func Subjects() []string {
	return []string{"leafrun.audit.access.action.v1", "leafrun.audit.projects.action.v1", "leafrun.audit.generation.action.v1", "leafrun.audit.storage.action.v1"}
}

// Provision is restricted to bootstrap credentials. Runtime consumers bind the
// existing durable and publishers require only their exact owner subject.
func Provision(ctx context.Context, manager StreamManager, retention time.Duration) error {
	if retention == 0 {
		retention = DefaultRetention
	}
	if manager == nil || retention < time.Hour || retention > 365*24*time.Hour {
		return errors.New("invalid audit provisioning configuration")
	}
	_, err := manager.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: StreamName, Subjects: Subjects(), Storage: jetstream.FileStorage,
		Retention: jetstream.LimitsPolicy, Discard: jetstream.DiscardNew,
		MaxAge: retention, MaxBytes: 256 << 20, Duplicates: 2 * time.Minute,
	})
	if err != nil {
		return errors.New("audit provisioning failed")
	}
	_, err = manager.CreateOrUpdateConsumer(ctx, StreamName, jetstream.ConsumerConfig{
		Durable: ConsumerName, AckPolicy: jetstream.AckExplicitPolicy, AckWait: 30 * time.Second,
		MaxDeliver: -1, MaxAckPending: 64, FilterSubjects: Subjects(),
	})
	if err != nil {
		return errors.New("audit provisioning failed")
	}
	return nil
}
