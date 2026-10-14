// Command jetstream-bootstrap provisions durable interest before producers start.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/leafrun/leafrun-infra/internal/e04events"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "JetStream bootstrap:", err)
		os.Exit(1)
	}
}
func run() error {
	if os.Getenv("LEAFRUN_DEV_FIXTURE") != "true" {
		return fmt.Errorf("explicit development profile required")
	}
	address, user, password := os.Getenv("LEAFRUN_NATS_ADDRESS"), os.Getenv("LEAFRUN_NATS_USER"), os.Getenv("LEAFRUN_NATS_PASSWORD")
	if address == "" || user == "" || password == "" {
		return fmt.Errorf("NATS address and bootstrap credentials are required")
	}
	conn, err := nats.Connect(address, nats.UserInfo(user, password), nats.Timeout(5*time.Second))
	if err != nil {
		return err
	}
	defer conn.Close()
	js, err := jetstream.New(conn)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return e04events.Provision(ctx, js)
}
