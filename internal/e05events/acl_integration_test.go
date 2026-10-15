package e05events

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func TestLiveProjectsNATSPermissions(t *testing.T) {
	if os.Getenv("LEAFRUN_E05_INTEGRATION") != "1" {
		t.Skip("explicit E05 NATS fixture is disabled")
	}
	errorsSeen := make(chan error, 8)
	connection, err := nats.Connect(os.Getenv("LEAFRUN_NATS_ADDRESS"),
		nats.UserInfo("projects", os.Getenv("LEAFRUN_NATS_PROJECTS_PASSWORD")), nats.NoReconnect(), nats.Timeout(3*time.Second),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			select {
			case errorsSeen <- err:
			default:
			}
		}))
	if err != nil {
		t.Fatal("connect Projects publisher principal")
	}
	defer connection.Close()
	if err = connection.Publish("leafrun.projects.permissionprobe", nil); err != nil {
		t.Fatal(err)
	}
	if err = connection.FlushTimeout(3 * time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case <-errorsSeen:
		t.Fatal("Projects publisher was denied its own event namespace")
	case <-time.After(100 * time.Millisecond):
	}
	denied := func(operation func() error) {
		t.Helper()
		if err := operation(); err != nil {
			t.Fatal("send Projects permission probe")
		}
		if err := connection.FlushTimeout(3 * time.Second); err != nil {
			t.Fatal("flush Projects permission probe")
		}
		select {
		case err := <-errorsSeen:
			if !strings.Contains(strings.ToLower(err.Error()), "permissions violation") {
				t.Fatal("unexpected Projects permission response")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("Projects credential could operate outside its producer ACL")
		}
	}
	denied(func() error { return connection.Publish("leafrun.generation.permissionprobe", nil) })
	denied(func() error { return connection.Publish("$JS.API.STREAM.DELETE.E05_PERMISSION_PROBE", []byte("{}")) })
	denied(func() error {
		_, err := connection.Subscribe("leafrun.projects.>", func(*nats.Msg) {})
		return err
	})
}
