package e04events

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/leafrun/leafrun-platform/events"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type modernDelivery struct{ jetstream.Msg }

func (d modernDelivery) Payload() []byte { return d.Data() }

type lostPublishACK struct {
	publisher events.Publisher
	lost      bool
}

func (p *lostPublishACK) Publish(ctx context.Context, subject string, data []byte, id string) error {
	if err := p.publisher.Publish(ctx, subject, data, id); err != nil {
		return err
	}
	if !p.lost {
		p.lost = true
		return errors.New("injected lost publish ACK")
	}
	return nil
}

func liveFixture(t *testing.T) (context.Context, *pgxpool.Pool, *nats.Conn, jetstream.JetStream, jetstream.Consumer, string, string) {
	t.Helper()
	if os.Getenv("LEAFRUN_E04_EVENTS_INTEGRATION") != "1" {
		t.Skip("real E04 PG/NATS fixture is not enabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	id := hex.EncodeToString(random[:])
	cfg, err := pgxpool.ParseConfig(os.Getenv("PLATFORM_TEST_DATABASE_URL"))
	if err != nil || cfg.ConnConfig.Database != "platform_test" {
		t.Fatal("requires isolated platform_test DB")
	}
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := "nats_test_" + id
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = events.ApplyMigration(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `CREATE TABLE effects(event_id text PRIMARY KEY,applications integer NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	nc, err := nats.Connect(os.Getenv("LEAFRUN_NATS_ADDRESS"), nats.UserInfo("bootstrap", os.Getenv("LEAFRUN_NATS_PASSWORD")), nats.Timeout(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	name := "E04_TEST_" + id
	subject := "leafrun.test.e04." + id
	if _, err = js.CreateStream(ctx, jetstream.StreamConfig{Name: name, Subjects: []string{subject}, Storage: jetstream.FileStorage, Retention: jetstream.InterestPolicy, Discard: jetstream.DiscardNew, MaxAge: 0}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = js.DeleteStream(context.Background(), name) })
	consumer, err := js.CreateConsumer(ctx, name, jetstream.ConsumerConfig{Durable: "TEST", AckPolicy: jetstream.AckExplicitPolicy, AckWait: time.Second, MaxDeliver: -1, FilterSubject: subject})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, pool, nc, js, consumer, id, subject
}
func liveEnvelope(id string) events.Envelope {
	return events.Envelope{SchemaVersion: 1, EventID: "evt_" + id, EventType: "generation.completed", Producer: "leafrun-generation", AggregateID: "job_" + id, AggregateVersion: 1, WorkspaceID: "w", ProjectID: "p", OccurredAt: time.Now().UTC(), Data: json.RawMessage(`{"jobId":"job_` + id + `"}`)}
}
func nextDelivery(t *testing.T, consumer jetstream.Consumer) modernDelivery {
	t.Helper()
	msg, err := consumer.Next(jetstream.FetchMaxWait(3 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return modernDelivery{msg}
}
func applyEffect(ctx context.Context, tx pgx.Tx, e events.Envelope) error {
	_, err := tx.Exec(ctx, `INSERT INTO effects VALUES($1,1) ON CONFLICT(event_id) DO UPDATE SET applications=effects.applications+1`, e.EventID)
	return err
}

func TestLiveNATSOutboxLostACKAndInboxDuplicate(t *testing.T) {
	ctx, pool, _, js, broker, id, subject := liveFixture(t)
	e := liveEnvelope(id)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = events.Enqueue(ctx, tx, e, subject); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	publisher := &lostPublishACK{publisher: events.JetStreamPublisher{Client: js}}
	relay := events.Relay{DB: pool, Publisher: publisher, RetryBase: time.Millisecond}
	if _, err = relay.RunOnce(ctx); err == nil {
		t.Fatal("lost ACK injection failed")
	}
	var sent bool
	if err = pool.QueryRow(ctx, `SELECT sent_at IS NOT NULL FROM platform_event_outbox WHERE event_id=$1`, e.EventID).Scan(&sent); err != nil || sent {
		t.Fatal("marked sent before ACK", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE platform_event_outbox SET available_at=clock_timestamp() WHERE event_id=$1`, e.EventID); err != nil {
		t.Fatal(err)
	}
	if n, err := relay.RunOnce(ctx); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	consumer := events.Consumer{DB: pool, Name: "test", Handler: applyEffect}
	if err = consumer.Consume(ctx, nextDelivery(t, broker)); err != nil {
		t.Fatal(err)
	}
	payload, _ := events.Encode(e)
	if _, err = js.Publish(ctx, subject, payload, jetstream.WithMsgID("transport_duplicate_"+id)); err != nil {
		t.Fatal(err)
	}
	if err = consumer.Consume(ctx, nextDelivery(t, broker)); err != nil {
		t.Fatal(err)
	}
	var effects int
	if err = pool.QueryRow(ctx, `SELECT applications FROM effects WHERE event_id=$1`, e.EventID).Scan(&effects); err != nil || effects != 1 {
		t.Fatal("duplicate effect", effects, err)
	}
}

func TestLiveNATSQuarantineACKAndDurableReplay(t *testing.T) {
	ctx, pool, nc, js, broker, id, subject := liveFixture(t)
	e := liveEnvelope(id)
	payload, _ := events.Encode(e)
	if _, err := js.Publish(ctx, subject, payload); err != nil {
		t.Fatal(err)
	}
	failing := true
	consumer := events.Consumer{DB: pool, Name: "test", RetryDelay: time.Millisecond, Handler: func(ctx context.Context, tx pgx.Tx, e events.Envelope) error {
		if err := applyEffect(ctx, tx, e); err != nil {
			return err
		}
		if failing {
			return errors.New("injected application failure")
		}
		return nil
	}}
	for range 5 {
		if err := consumer.Consume(ctx, nextDelivery(t, broker)); err != nil {
			t.Fatal(err)
		}
	}
	if err := nc.FlushTimeout(3 * time.Second); err != nil {
		t.Fatal(err)
	}
	status, err := consumer.Status(ctx, e.EventID)
	if err != nil || status.Status != events.DispositionQuarantined || status.Attempts != 5 {
		t.Fatal(status, err)
	}
	info, err := broker.Info(ctx)
	if err != nil || info.NumAckPending != 0 || info.NumPending != 0 {
		t.Fatal("quarantine was not ACKed", info, err)
	}
	if err = consumer.Replay(ctx, e.EventID); err != nil {
		t.Fatal(err)
	}
	failing = false
	restarted := events.Consumer{DB: pool, Name: "test", Handler: consumer.Handler}
	if n, err := restarted.RunReplayOnce(ctx); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	var effects int
	if err = pool.QueryRow(ctx, `SELECT applications FROM effects WHERE event_id=$1`, e.EventID).Scan(&effects); err != nil || effects != 1 {
		t.Fatal("failed effects were not rolled back", effects, err)
	}
}
