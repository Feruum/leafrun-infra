package e06events

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func liveConnection(t *testing.T, role string) (*nats.Conn, <-chan error) {
	t.Helper()
	if os.Getenv("E06_AUDIT_INTEGRATION") != "1" {
		t.Skip("explicit owned E06 Audit broker fixture disabled")
	}
	key := strings.ToUpper(role)
	user := role
	if role == "admin" {
		user = "bootstrap"
	}
	errorsSeen := make(chan error, 16)
	nc, err := nats.Connect(os.Getenv("E06_AUDIT_NATS_URL"), nats.UserInfo(user, os.Getenv("E06_AUDIT_NATS_"+key+"_PASSWORD")), nats.NoReconnect(), nats.Timeout(3*time.Second), nats.CustomInboxPrefix("_INBOX."+user), nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
		select {
		case errorsSeen <- err:
		default:
		}
	}))
	if err != nil {
		t.Fatal("owned broker fixture connection failed")
	}
	t.Cleanup(nc.Close)
	return nc, errorsSeen
}
func deniedOperation(t *testing.T, nc *nats.Conn, errorsSeen <-chan error, operation func() error) {
	t.Helper()
	if operation() != nil || nc.FlushTimeout(3*time.Second) != nil {
		t.Fatal("send bounded permission probe")
	}
	select {
	case err := <-errorsSeen:
		if !strings.Contains(strings.ToLower(err.Error()), "permissions violation") {
			t.Fatal("unexpected broker permission response")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runtime credential exceeded its fixed ACL")
	}
}
func TestLiveAuditStreamsCoexistWithOperationalStreams(t *testing.T) {
	nc, _ := liveConnection(t, "admin")
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal("owned bootstrap client")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, name := range []string{"LEAFRUN_GENERATION", "LEAFRUN_PROJECTS", StreamName} {
		s, err := js.Stream(ctx, name)
		if err != nil {
			t.Fatal("required owned operational/Audit stream is not provisioned")
		}
		info, err := s.Info(ctx)
		if err != nil {
			t.Fatal("inspect owned stream")
		}
		if name == StreamName && (info.Config.Retention != jetstream.LimitsPolicy || info.Config.Discard != jetstream.DiscardNew || info.Config.MaxAge != DefaultRetention) {
			t.Fatal("actual Audit stream durability policy mismatch")
		}
	}
}
func TestLiveAuditPublishersHaveExactSeparateNamespaces(t *testing.T) {
	for _, role := range []string{"access", "projects", "generation", "storage"} {
		t.Run(role, func(t *testing.T) {
			nc, errorsSeen := liveConnection(t, role)
			js, err := jetstream.New(nc)
			if err != nil {
				t.Fatal("owned publisher client")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			// Benign invalid payload is a transport ACL probe, never a business fact.
			if _, err = js.Publish(ctx, "leafrun.audit."+role+".action.v1", []byte("{}")); err != nil {
				t.Fatal("owner cannot durably publish its exact Audit subject")
			}
			other := "access"
			if role == other {
				other = "storage"
			}
			deniedOperation(t, nc, errorsSeen, func() error { return nc.Publish("leafrun.audit."+other+".action.v1", nil) })
			if role == "generation" || role == "projects" {
				deniedOperation(t, nc, errorsSeen, func() error { return nc.Publish("leafrun."+role+".unregistered.v1", nil) })
			}
			deniedOperation(t, nc, errorsSeen, func() error { return nc.Publish("$JS.API.STREAM.DELETE.LEAFRUN_AUDIT", []byte("{}")) })
			deniedOperation(t, nc, errorsSeen, func() error { _, err := nc.Subscribe("leafrun.audit.>", func(*nats.Msg) {}); return err })
		})
	}
}
func TestLiveAuditConsumerCanOnlyBindPullAndAckFixedDurable(t *testing.T) {
	// The coordinator must pause the real Audit consumer for this bounded ACL
	// probe. A test must never acknowledge another consumer's business fact.
	if os.Getenv("E06_AUDIT_EXCLUSIVE_CONSUMER_PROBE") != "1" {
		t.Skip("fixed durable ACL probe requires explicitly coordinated exclusive consumer access")
	}
	nc, errorsSeen := liveConnection(t, "audit")
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal("owned Audit client")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := js.Stream(ctx, StreamName)
	if err != nil {
		t.Fatal("runtime cannot inspect fixed Audit retention horizon")
	}
	if info, err := stream.Info(ctx); err != nil || info.Config.MaxAge != DefaultRetention {
		t.Fatal("fixed Audit stream retention inspection denied")
	}
	c, err := js.Consumer(ctx, StreamName, ConsumerName)
	if err != nil {
		t.Fatal("runtime cannot bind preprovisioned fixed durable")
	}
	batch, err := c.Fetch(1, jetstream.FetchMaxWait(time.Second))
	if err != nil {
		t.Fatal("fixed durable pull denied")
	}
	got := 0
	for message := range batch.Messages() {
		if !bytes.Equal(message.Data(), []byte("{}")) {
			_ = message.Nak()
			t.Fatal("fixed durable contains a business message; ACL probe returned it without acknowledgement")
		}
		if message.Ack() != nil {
			t.Fatal("fixed durable ACK denied")
		}
		got++
	}
	if batch.Error() != nil || got != 1 || nc.FlushTimeout(3*time.Second) != nil {
		t.Fatal("fixed durable message/ACK did not complete")
	}
	deniedOperation(t, nc, errorsSeen, func() error { return nc.Publish("leafrun.audit.access.action.v1", nil) })
	deniedOperation(t, nc, errorsSeen, func() error { return nc.Publish("$JS.API.CONSUMER.CREATE.LEAFRUN_AUDIT.E06_FORBIDDEN", []byte("{}")) })
	deniedOperation(t, nc, errorsSeen, func() error { _, err := nc.Subscribe("leafrun.audit.>", func(*nats.Msg) {}); return err })
}
