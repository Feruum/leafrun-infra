package e06

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	access "github.com/leafrun/leafrun-contracts/access/v1"
	audit "github.com/leafrun/leafrun-contracts/audit/v1"
	"github.com/leafrun/leafrun-platform/events"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type capturedAuditFact struct {
	event   events.Envelope
	subject string
	raw     []byte
}

func TestAuditTransportOwnSchema(t *testing.T) {
	if os.Getenv("LEAFRUN_AUDIT_TEST_DATABASE_URL") == "" {
		t.Skip("owned audit_test database not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool := auditTransportPool(t, ctx)
	var database, schema string
	if pool.QueryRow(ctx, `SELECT current_database(),current_schema()`).Scan(&database, &schema) != nil || database != "audit_test" || !strings.HasPrefix(schema, "e06_audit_transport_") {
		t.Fatal("captured transport fixture escaped its unique owned test schema")
	}
	var tables int
	if pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name IN('platform_event_outbox','platform_event_inbox','platform_event_history')`).Scan(&tables) != nil || tables != 3 {
		t.Fatal("captured transport fixture migration incomplete")
	}
}

// Replay only byte-identical facts selected from this test's real owner commits.
// No IDs, actors, operation IDs, scopes or business payload are synthesized.
// The ACK-loss retry uses a unique audit_test schema, never a production outbox.
func ownerAuditTransport(t *testing.T, items []audit.HistoryItem, expected map[audit.ActionName]string, scope access.PermissionScope) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var facts []capturedAuditFact
	for _, item := range items {
		resource, wanted := expected[item.Action.Action]
		if !wanted || resource != item.Action.Resource.ID {
			continue
		}
		pool := ownerPool(t, "E06_"+strings.ToUpper(string(item.Action.Producer))+"_DATABASE_URL")
		var raw []byte
		var subject string
		if pool.QueryRow(ctx, `SELECT subject,encoded_envelope FROM platform_event_outbox WHERE event_id=$1`, item.EventID).Scan(&subject, &raw) != nil {
			t.Fatal("this run's committed owner outbox fact unavailable")
		}
		e, action, err := events.DecodeAuditAction(subject, raw)
		canonical, encodeErr := events.Encode(e)
		if err != nil || encodeErr != nil || !bytes.Equal(canonical, raw) || e.EventID != item.EventID || action.Actor != item.Action.Actor || action.Authority != item.Action.Authority || e.OrganizationID != scope.OrganizationID || e.WorkspaceID != scope.WorkspaceID || e.ProjectID != scope.ProjectID {
			t.Fatal("captured owner provenance/bytes mismatch")
		}
		facts = append(facts, capturedAuditFact{e, subject, raw})
	}
	if len(facts) != len(expected) {
		t.Fatal("bounded captured owner fact set incomplete")
	}
	admin := auditBroker(t, "admin")
	observer, err := jetstream.New(admin)
	if err != nil {
		t.Fatal("owned Audit broker observer unavailable")
	}
	durable, err := observer.Consumer(ctx, "LEAFRUN_AUDIT", "LEAFRUN_AUDIT_V1")
	if err != nil {
		t.Fatal("fixed Audit durable observation unavailable")
	}
	var maxSequence uint64
	var storageFact capturedAuditFact
	// Page order is descending application sequence. Replaying that exact order
	// redelivers the immutable facts in reverse of the initial application order.
	for _, fact := range facts {
		role := fact.event.Producer
		js, err := jetstream.New(auditBroker(t, role))
		if err != nil {
			t.Fatal("owned captured-fact publisher unavailable")
		}
		ack, err := js.Publish(ctx, fact.subject, fact.raw) // no broker msg-ID: exercise DB deduplication
		if err != nil || ack == nil || ack.Stream != "LEAFRUN_AUDIT" {
			t.Fatal("bounded exact owner redelivery failed")
		}
		if ack.Sequence > maxSequence {
			maxSequence = ack.Sequence
		}
		if role == "storage" {
			storageFact = fact
		}
	}
	pool := auditTransportPool(t, ctx)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal("isolated captured-fact transaction unavailable")
	}
	defer tx.Rollback(ctx)
	if events.Enqueue(ctx, tx, storageFact.event, storageFact.subject) != nil || tx.Commit(ctx) != nil {
		t.Fatal("isolated captured-fact outbox commit failed")
	}
	storageJS, err := jetstream.New(auditBroker(t, "storage"))
	if err != nil {
		t.Fatal("owned ACK-loss publisher unavailable")
	}
	publisher := &auditLostAcknowledgement{js: storageJS, loseOnce: true}
	relay := &events.Relay{DB: pool, Publisher: events.AuditPublisher{Publisher: publisher}, RetryBase: time.Millisecond}
	if n, err := relay.RunOnce(ctx); n != 0 || err == nil {
		t.Fatal("lost durable-ACK observation marked captured outbox sent")
	}
	var pending bool
	var attempts int
	if pool.QueryRow(ctx, `SELECT sent_at IS NULL,attempts FROM platform_event_outbox WHERE event_id=$1`, storageFact.event.EventID).Scan(&pending, &attempts) != nil || !pending || attempts != 1 {
		t.Fatal("ACK-loss did not preserve committed pending captured fact")
	}
	time.Sleep(10 * time.Millisecond)
	if n, err := relay.RunOnce(ctx); n != 1 || err != nil {
		t.Fatal("real NATS ACK-loss retry did not recover captured fact")
	}
	if publisher.sequence > maxSequence {
		maxSequence = publisher.sequence
	}
	if pool.QueryRow(ctx, `SELECT sent_at IS NOT NULL,attempts FROM platform_event_outbox WHERE event_id=$1`, storageFact.event.EventID).Scan(&pending, &attempts) != nil || !pending || attempts != 2 {
		t.Fatal("recovered captured fact receipt invalid")
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		info, err := durable.Info(ctx)
		if err == nil && info.AckFloor.Stream >= maxSequence {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("actual Audit consumer did not ACK bounded duplicate/reordered deliveries")
		}
		time.Sleep(100 * time.Millisecond)
	}
	live := ownerPool(t, "E06_AUDIT_DATABASE_URL")
	for _, fact := range facts {
		var count int
		if live.QueryRow(ctx, `SELECT count(*) FROM audit_actions WHERE event_id=$1 AND organization_id=$2 AND workspace_id=$3 AND project_id=$4`, fact.event.EventID, scope.OrganizationID, scope.WorkspaceID, scope.ProjectID).Scan(&count) != nil || count != 1 {
			t.Fatal("duplicate/reordered actual owner fact changed history cardinality")
		}
		var saved []byte
		if live.QueryRow(ctx, `SELECT encoded_envelope FROM platform_event_inbox WHERE consumer_name='leafrun-audit-v1' AND event_id=$1 AND status='applied'`, fact.event.EventID).Scan(&saved) != nil || !bytes.Equal(saved, fact.raw) {
			t.Fatal("redelivery changed validated applied inbox bytes")
		}
	}
	t.Log("Byte-identical actual owner facts survived reverse redelivery; isolated captured-fact PG outbox remained pending after real NATS durable ACK was hidden once and recovered on retry")
}

type auditLostAcknowledgement struct {
	js       jetstream.JetStream
	loseOnce bool
	sequence uint64
}

func (p *auditLostAcknowledgement) Publish(ctx context.Context, subject string, raw []byte, _ string) error {
	ack, err := p.js.Publish(ctx, subject, raw)
	if err != nil || ack == nil || ack.Stream != "LEAFRUN_AUDIT" {
		return errors.New("owned captured-fact broker unavailable")
	}
	p.sequence = ack.Sequence
	if p.loseOnce {
		p.loseOnce = false
		return errors.New("owned durable acknowledgement observation lost")
	}
	return nil
}

func auditBroker(t *testing.T, role string) *nats.Conn {
	t.Helper()
	credential := os.Getenv("E06_AUDIT_NATS_" + strings.ToUpper(role) + "_PASSWORD")
	if credential == "" || os.Getenv("E06_AUDIT_NATS_URL") == "" {
		t.Fatal("owned captured-fact broker setting missing")
	}
	user := role
	if role == "admin" {
		user = "bootstrap"
	}
	nc, err := nats.Connect(os.Getenv("E06_AUDIT_NATS_URL"), nats.UserInfo(user, credential), nats.CustomInboxPrefix("_INBOX."+user), nats.NoReconnect(), nats.Timeout(3*time.Second), nats.ErrorHandler(func(*nats.Conn, *nats.Subscription, error) {}))
	if err != nil {
		t.Fatal("owned captured-fact broker connection failed")
	}
	t.Cleanup(nc.Close)
	return nc
}

func auditTransportPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("LEAFRUN_AUDIT_TEST_DATABASE_URL")
	config, err := pgxpool.ParseConfig(dsn)
	if dsn == "" || err != nil || config.ConnConfig.Database != "audit_test" {
		t.Fatal("isolated Audit transport fixture requires owned audit_test database")
	}
	admin, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal("isolated Audit schema connection failed")
	}
	t.Cleanup(admin.Close)
	schema := pgx.Identifier{"e06_audit_transport_" + ownerNonce(t)}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal("isolated Audit schema creation failed")
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error("owned isolated Audit schema cleanup failed")
		}
	})
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal("isolated Audit schema pool failed")
	}
	t.Cleanup(pool.Close)
	if events.ApplyMigration(ctx, pool) != nil {
		t.Fatal("isolated platform outbox migration failed")
	}
	return pool
}
