// e06-jetstream-bootstrap runs with dedicated bootstrap credentials only.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/leafrun/leafrun-infra/internal/e04events"
	"github.com/leafrun/leafrun-infra/internal/e05events"
	"github.com/leafrun/leafrun-infra/internal/e06events"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func main() {
	if run() != nil {
		fmt.Fprintln(os.Stderr, "E06 JetStream bootstrap failed")
		os.Exit(1)
	}
	fmt.Println("E06 operational and Audit streams/durable ready")
}
func run() error {
	if os.Getenv("LEAFRUN_E06_BOOTSTRAP") != "true" {
		return errors.New("explicit E06 bootstrap role required")
	}
	address, user, password := os.Getenv("LEAFRUN_NATS_ADDRESS"), os.Getenv("LEAFRUN_NATS_USER"), os.Getenv("LEAFRUN_NATS_PASSWORD")
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "nats" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || user != "bootstrap" || password == "" {
		return errors.New("invalid bootstrap configuration")
	}
	retention := e06events.DefaultRetention
	if raw := os.Getenv("LEAFRUN_AUDIT_RETENTION"); raw != "" {
		retention, err = time.ParseDuration(raw)
		if err != nil || retention < time.Hour || retention > 365*24*time.Hour {
			return errors.New("invalid audit retention")
		}
	}
	conn, err := nats.Connect(address, nats.UserInfo(user, password), nats.NoReconnect(), nats.Timeout(5*time.Second), nats.CustomInboxPrefix("_INBOX.bootstrap"))
	if err != nil {
		return errors.New("bootstrap connection failed")
	}
	defer conn.Close()
	js, err := jetstream.New(conn)
	if err != nil {
		return errors.New("bootstrap initialization failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if e04events.Provision(ctx, js) != nil || e05events.Provision(ctx, js) != nil {
		return errors.New("operational provisioning failed")
	}
	return e06events.Provision(ctx, js, retention)
}
