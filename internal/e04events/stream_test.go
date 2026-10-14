package e04events

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

type fakeManager struct {
	stream     jetstream.StreamConfig
	consumer   jetstream.ConsumerConfig
	streamName string
}

func (f *fakeManager) CreateOrUpdateStream(_ context.Context, cfg jetstream.StreamConfig) (jetstream.Stream, error) {
	f.stream = cfg
	return nil, nil
}
func (f *fakeManager) CreateOrUpdateConsumer(_ context.Context, name string, cfg jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	f.streamName = name
	f.consumer = cfg
	return nil, nil
}
func TestProvisionRetainsUnacknowledgedEventsAndUsesApplicationRetryLedger(t *testing.T) {
	var manager fakeManager
	if err := Provision(context.Background(), &manager); err != nil {
		t.Fatal(err)
	}
	if manager.stream.Name != StreamName || manager.stream.Storage != jetstream.FileStorage || manager.stream.Retention != jetstream.InterestPolicy || manager.stream.Discard != jetstream.DiscardNew || manager.stream.MaxAge != 0 {
		t.Fatalf("unsafe pending-event retention: %+v", manager.stream)
	}
	if manager.consumer.Durable != ConsumerName || manager.streamName != StreamName || manager.consumer.AckPolicy != jetstream.AckExplicitPolicy || manager.consumer.MaxDeliver != -1 {
		t.Fatalf("broker must not replace durable application retries: %+v", manager.consumer)
	}
	if manager.consumer.FilterSubject != "leafrun.generation.job.completed.v1" {
		t.Fatal(manager.consumer.FilterSubject)
	}
}
