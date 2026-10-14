package e04events

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func TestLiveNATSServicePermissions(t *testing.T) {
	if os.Getenv("LEAFRUN_E04_EVENTS_INTEGRATION") != "1" {
		t.Skip("real NATS permissions fixture is disabled")
	}
	address := os.Getenv("LEAFRUN_NATS_ADDRESS")
	connect := func(user, password string) (*nats.Conn, <-chan error) {
		t.Helper()
		errorsSeen := make(chan error, 8)
		nc, err := nats.Connect(address, nats.UserInfo(user, password), nats.Timeout(3*time.Second), nats.NoReconnect(), nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) { errorsSeen <- err }))
		if err != nil {
			t.Fatal("connect fixed service principal:", err)
		}
		t.Cleanup(nc.Close)
		return nc, errorsSeen
	}
	denied := func(nc *nats.Conn, seen <-chan error, operation func() error) {
		t.Helper()
		if err := operation(); err != nil {
			t.Fatal("permission probe could not be sent:", err)
		}
		if err := nc.FlushTimeout(3 * time.Second); err != nil {
			t.Fatal("permission probe flush:", err)
		}
		select {
		case err := <-seen:
			if !strings.Contains(strings.ToLower(err.Error()), "permissions violation") {
				t.Fatal("unexpected permission response:", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("broker accepted an operation outside the service ACL")
		}
	}
	producer, producerErrors := connect("generation", os.Getenv("LEAFRUN_NATS_GENERATION_PASSWORD"))
	if err := producer.Publish("leafrun.generation.permissionprobe", nil); err != nil {
		t.Fatal(err)
	}
	if err := producer.FlushTimeout(3 * time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-producerErrors:
		t.Fatal("publisher domain permission denied:", err)
	case <-time.After(100 * time.Millisecond):
	}
	denied(producer, producerErrors, func() error { return producer.Publish("leafrun.storage.permissionprobe", nil) })
	denied(producer, producerErrors, func() error {
		_, err := producer.Subscribe("leafrun.generation.>", func(*nats.Msg) {})
		return err
	})
	consumer, consumerErrors := connect("consumer", os.Getenv("LEAFRUN_NATS_CONSUMER_PASSWORD"))
	js, err := jetstream.New(consumer)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err = js.Consumer(ctx, StreamName, ConsumerName); err != nil {
		t.Fatal("consumer cannot inspect its own durable delivery:", err)
	}
	denied(consumer, consumerErrors, func() error { return consumer.Publish("leafrun.generation.permissionprobe", nil) })
	denied(consumer, consumerErrors, func() error { return consumer.Publish("$JS.API.STREAM.DELETE.E04_PERMISSION_PROBE", []byte(`{}`)) })
	if anonymous, err := nats.Connect(address, nats.NoReconnect(), nats.Timeout(3*time.Second)); err == nil {
		anonymous.Close()
		t.Fatal("broker accepted an anonymous connection")
	} else if !errors.Is(err, nats.ErrAuthorization) {
		t.Fatal("anonymous connection failed for an unrelated reason:", err)
	}
}
