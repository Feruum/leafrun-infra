package e06events

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

type recordingManager struct {
	streams                []jetstream.StreamConfig
	consumers              []jetstream.ConsumerConfig
	consumerStream         string
	streamErr, consumerErr error
}

func (m *recordingManager) CreateOrUpdateStream(_ context.Context, c jetstream.StreamConfig) (jetstream.Stream, error) {
	m.streams = append(m.streams, c)
	return nil, m.streamErr
}
func (m *recordingManager) CreateOrUpdateConsumer(_ context.Context, stream string, c jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	m.consumerStream = stream
	m.consumers = append(m.consumers, c)
	return nil, m.consumerErr
}
func TestAuditDurableExistsBeforeProducersAndKeepsOperationalStreamsSeparate(t *testing.T) {
	m := &recordingManager{}
	if err := Provision(context.Background(), m, 0); err != nil {
		t.Fatal(err)
	}
	if len(m.streams) != 1 || len(m.consumers) != 1 {
		t.Fatal("audit stream and fixed durable must both be provisioned")
	}
	s, c := m.streams[0], m.consumers[0]
	want := []string{"leafrun.audit.access.action.v1", "leafrun.audit.projects.action.v1", "leafrun.audit.generation.action.v1", "leafrun.audit.storage.action.v1"}
	if s.Name != "LEAFRUN_AUDIT" || !reflect.DeepEqual(s.Subjects, want) || s.Storage != jetstream.FileStorage || s.Retention != jetstream.LimitsPolicy || s.Discard != jetstream.DiscardNew || s.MaxAge != 30*24*time.Hour || s.MaxBytes <= 0 {
		t.Fatal("bounded durable audit retention or exact owner subject isolation missing")
	}
	if m.consumerStream != s.Name || c.Durable != "LEAFRUN_AUDIT_V1" || c.AckPolicy != jetstream.AckExplicitPolicy || !reflect.DeepEqual(c.FilterSubjects, want) || c.FilterSubject != "" || c.DeliverSubject != "" || c.InactiveThreshold != 0 || c.MaxDeliver != -1 {
		t.Fatal("fixed pull durable must retain unacknowledged retry interest without runtime management")
	}
}
func TestAuditRetentionIsFiniteAndConfigurable(t *testing.T) {
	for _, retention := range []time.Duration{-1, time.Second, 366 * 24 * time.Hour} {
		m := &recordingManager{}
		if Provision(context.Background(), m, retention) == nil || len(m.streams) != 0 {
			t.Fatal("unsafe retention admitted")
		}
	}
	m := &recordingManager{}
	if err := Provision(context.Background(), m, 48*time.Hour); err != nil || len(m.streams) != 1 || m.streams[0].MaxAge != 48*time.Hour {
		t.Fatal("configured audit retention ignored")
	}
}
func TestAuditBootstrapFailureDoesNotAdmitProducers(t *testing.T) {
	for _, stage := range []string{"stream", "consumer"} {
		m := &recordingManager{}
		if stage == "stream" {
			m.streamErr = errors.New("private provider diagnostic")
		} else {
			m.consumerErr = errors.New("private provider diagnostic")
		}
		if err := Provision(context.Background(), m, 0); err == nil || err.Error() != "audit provisioning failed" {
			t.Fatal("bootstrap failure not safely surfaced")
		}
		if stage == "stream" && len(m.consumers) != 0 {
			t.Fatal("consumer created after failed stream")
		}
	}
}
