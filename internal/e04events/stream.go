package e04events

import (
	"context"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const StreamName = "LEAFRUN_GENERATION"
const ConsumerName = "DEV_CONSUMER"

type StreamManager interface {
	CreateOrUpdateStream(context.Context, jetstream.StreamConfig) (jetstream.Stream, error)
	CreateOrUpdateConsumer(context.Context, string, jetstream.ConsumerConfig) (jetstream.Consumer, error)
}

// Provision runs with bootstrap credentials before Generation may publish. The
// durable interest exists before the first event. Unacknowledged events have no
// age expiry; a full stream rejects new publishes so the owner's outbox retains
// them. Quarantine retains its own original bytes before the broker is ACKed.
func Provision(ctx context.Context, manager StreamManager) error {
	if _, err := manager.CreateOrUpdateStream(ctx, jetstream.StreamConfig{Name: StreamName, Subjects: []string{"leafrun.generation.>"}, Storage: jetstream.FileStorage, Retention: jetstream.InterestPolicy, Discard: jetstream.DiscardNew, MaxAge: 0, MaxBytes: 1 << 30, Duplicates: 2 * time.Minute}); err != nil {
		return err
	}
	_, err := manager.CreateOrUpdateConsumer(ctx, StreamName, jetstream.ConsumerConfig{Durable: ConsumerName, AckPolicy: jetstream.AckExplicitPolicy, AckWait: 30 * time.Second, MaxDeliver: -1, MaxAckPending: 64, FilterSubject: "leafrun.generation.job.completed.v1"})
	return err
}
