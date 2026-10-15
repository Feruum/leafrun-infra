package e04

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	projectsv1 "github.com/leafrun/leafrun-contracts/projects/v1"
	"github.com/leafrun/leafrun-infra/internal/e05events"
	"github.com/leafrun/leafrun-platform/events"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type e05BrokerEvent struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	SHA256  string `json:"sha256"`
}

type e05BrokerReceipt struct {
	ReleaseID        string           `json:"releaseId"`
	DescriptorDigest string           `json:"descriptorDigest"`
	Events           []e05BrokerEvent `json:"events"`
}

func e05ProjectsPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(os.Getenv("LEAFRUN_E05_PROJECTS_CATALOG_URL"))
	if err != nil || cfg.ConnConfig.Database != "projects" || cfg.ConnConfig.User != "projects" {
		t.Fatal("requires the owned Projects catalog test harness")
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("open owned Projects catalog")
	}
	t.Cleanup(pool.Close)
	return pool
}

func e05ReleaseEvents(t *testing.T, ctx context.Context, pool *pgxpool.Pool, releaseID string, wantPending bool) []e05BrokerEvent {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT event_id,subject,encoded_envelope,sent_at IS NULL FROM platform_event_outbox
		WHERE convert_from(encoded_envelope,'UTF8')::jsonb->>'aggregateId'=$1 ORDER BY created_at,event_id`, releaseID)
	if err != nil {
		t.Fatal("read selected release outbox")
	}
	defer rows.Close()
	out := []e05BrokerEvent{}
	for rows.Next() {
		var id, subject string
		var encoded []byte
		var pending bool
		if err = rows.Scan(&id, &subject, &encoded, &pending); err != nil {
			t.Fatal("scan selected release outbox")
		}
		envelope, decodeErr := events.Decode(encoded)
		var payload map[string]string
		if decodeErr != nil || json.Unmarshal(envelope.Data, &payload) != nil || len(payload) != 1 || payload["releaseId"] != releaseID ||
			envelope.AggregateID != releaseID || envelope.Producer != "projects" || envelope.EventID != id {
			t.Fatal("Projects event exposed data beyond the release ID or lost its immutable binding")
		}
		if wantPending && !pending {
			t.Fatal("Projects event was marked sent while its broker was stopped")
		}
		hash := sha256.Sum256(encoded)
		out = append(out, e05BrokerEvent{ID: id, Subject: subject, SHA256: hex.EncodeToString(hash[:])})
	}
	if rows.Err() != nil || len(out) != 4 {
		t.Fatalf("release lifecycle did not retain exactly four durable events: count=%d", len(out))
	}
	return out
}

// The existing captured-container broker outage script drives both domains.
func TestE05BrokerOutageRecovery(t *testing.T) {
	stage, path := os.Getenv("LEAFRUN_E04_BROKER_STAGE"), os.Getenv("LEAFRUN_E05_BROKER_RECEIPT_FILE")
	if path == "" || stage != "submit" && stage != "verify" {
		t.Skip("explicit E05 broker outage stage is disabled")
	}
	s, projects := e05Stand(t)
	pool := e05ProjectsPool(t, s.ctx)
	if stage == "submit" {
		snapshot := s.e05Snapshot(t, "#publication_requires_no_renderer")
		_, release := s.e05Release(t, projects, snapshot)
		published, err := projects.SetPublication(s.ctx, release.ReleaseID, projectsv1.PublicationRequest{Scope: s.scope, ExpectedVersion: release.PublicationVersion, Published: true})
		if err != nil || !published.Published {
			t.Fatal("Projects publication depended on NATS availability")
		}
		withdrawn, err := projects.SetPublication(s.ctx, release.ReleaseID, projectsv1.PublicationRequest{Scope: s.scope, ExpectedVersion: published.PublicationVersion, Published: false})
		if err != nil || withdrawn.Published {
			t.Fatal("Projects withdrawal depended on NATS availability")
		}
		receipt := e05BrokerReceipt{ReleaseID: release.ReleaseID, DescriptorDigest: release.DescriptorDigest,
			Events: e05ReleaseEvents(t, s.ctx, pool, release.ReleaseID, true)}
		writeE05Receipt(t, path, receipt)
		t.Log("Projects release/publish/withdraw committed with four unsent ID-only events while NATS was stopped")
		return
	}
	var receipt e05BrokerReceipt
	readE05Receipt(t, path, &receipt)
	release, err := projects.GetRelease(s.ctx, receipt.ReleaseID)
	if err != nil || release.Published || release.DescriptorDigest != receipt.DescriptorDigest {
		t.Fatal("broker recovery changed release visibility or frozen identity")
	}
	ctx, cancel := context.WithTimeout(s.ctx, 70*time.Second)
	defer cancel()
	for {
		var pending int
		err = pool.QueryRow(ctx, `SELECT count(*) FROM platform_event_outbox WHERE sent_at IS NULL
			AND convert_from(encoded_envelope,'UTF8')::jsonb->>'aggregateId'=$1`, receipt.ReleaseID).Scan(&pending)
		if err != nil {
			t.Fatal("read durable Projects delivery status")
		}
		if pending == 0 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("Projects relay did not deliver its retained events after broker recovery")
		case <-time.After(250 * time.Millisecond):
		}
	}
	current := e05ReleaseEvents(t, ctx, pool, receipt.ReleaseID, false)
	expected := map[string]e05BrokerEvent{}
	subjects := map[string]bool{}
	for _, event := range receipt.Events {
		expected[event.ID] = event
		subjects[event.Subject] = true
	}
	if len(expected) != 4 {
		t.Fatal("broker receipt did not preserve four event identities")
	}
	for _, event := range current {
		if expected[event.ID] != event {
			t.Fatal("event identity or exact payload changed during delivery retry")
		}
	}
	connection, err := nats.Connect(os.Getenv("LEAFRUN_NATS_ADDRESS"), nats.UserInfo(os.Getenv("LEAFRUN_NATS_USER"), os.Getenv("LEAFRUN_NATS_PASSWORD")),
		nats.NoReconnect(), nats.Timeout(3*time.Second))
	if err != nil {
		t.Fatal("connect owned broker verification principal")
	}
	defer connection.Close()
	js, err := jetstream.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := js.Stream(ctx, e05events.StreamName)
	if err != nil {
		t.Fatal("Projects stream was not preserved across broker outage")
	}
	info, err := stream.Info(ctx)
	if err != nil || info.State.LastSeq > 10000 {
		t.Fatal("bounded Projects verification stream is unavailable")
	}
	seen := map[string]int{}
	for sequence := info.State.FirstSeq; sequence > 0 && sequence <= info.State.LastSeq; sequence++ {
		message, readErr := stream.GetMsg(ctx, sequence)
		if errors.Is(readErr, jetstream.ErrMsgNotFound) {
			continue
		}
		if readErr != nil {
			t.Fatal("read Projects stream message")
		}
		if !subjects[message.Subject] {
			continue // ACL permission probes are outside this release lifecycle.
		}
		envelope, decodeErr := events.Decode(message.Data)
		if decodeErr != nil {
			t.Fatal("Projects stream contains an invalid event")
		}
		if event, exists := expected[envelope.EventID]; exists {
			hash := sha256.Sum256(message.Data)
			if message.Subject != event.Subject || hex.EncodeToString(hash[:]) != event.SHA256 ||
				!bytes.Equal([]byte(message.Header.Get(nats.MsgIdHdr)), []byte(event.ID)) {
				t.Fatal("broker event differs from its exact durable outbox payload")
			}
			seen[event.ID]++
		}
	}
	for id := range expected {
		if seen[id] != 1 {
			t.Fatal("retained Projects event was missing or duplicated in JetStream")
		}
	}
	t.Log("same four Projects event IDs reached JetStream and were acknowledged without changing release state")
}
