// dev-consumer exercises a real independent inbox/effect database. It is
// explicitly restricted to the internal development stand.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	generationv1 "github.com/leafrun/leafrun-contracts/generation/v1"
	"github.com/leafrun/leafrun-infra/internal/e04events"
	"github.com/leafrun/leafrun-platform/events"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type delivery struct{ jetstream.Msg }

func (d delivery) Payload() []byte { return d.Data() }

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "dev consumer stopped:", err)
		os.Exit(1)
	}
}
func run(ctx context.Context) error {
	if os.Getenv("LEAFRUN_DEV_FIXTURE") != "true" {
		return errors.New("explicit development profile required")
	}
	for _, key := range []string{"LEAFRUN_CONSUMER_DATABASE_URL", "LEAFRUN_NATS_ADDRESS", "LEAFRUN_NATS_USER", "LEAFRUN_NATS_PASSWORD", "LEAFRUN_GENERATION_URL", "LEAFRUN_GENERATION_TOKEN"} {
		if os.Getenv(key) == "" {
			return fmt.Errorf("%s is required", key)
		}
	}
	pool, err := pgxpool.New(ctx, os.Getenv("LEAFRUN_CONSUMER_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	if err = events.ApplyMigration(ctx, pool); err != nil {
		return err
	}
	if err = e04events.MigrateProjection(ctx, pool); err != nil {
		return err
	}
	owner, err := generationv1.NewClient(os.Getenv("LEAFRUN_GENERATION_URL"), os.Getenv("LEAFRUN_GENERATION_TOKEN"), &http.Client{Timeout: 10 * time.Second})
	if err != nil {
		return err
	}
	nc, err := nats.Connect(os.Getenv("LEAFRUN_NATS_ADDRESS"), nats.UserInfo(os.Getenv("LEAFRUN_NATS_USER"), os.Getenv("LEAFRUN_NATS_PASSWORD")), nats.Timeout(5*time.Second))
	if err != nil {
		return err
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		return err
	}
	broker, err := js.Consumer(ctx, e04events.StreamName, e04events.ConsumerName)
	if err != nil {
		return err
	}
	consumer := &events.Consumer{DB: pool, Name: e04events.ConsumerName, Handler: e04events.ProjectionHandler(owner)}
	nextPurge := time.Time{}
	for ctx.Err() == nil {
		if !time.Now().Before(nextPurge) {
			if err = purgeResolved(ctx, pool); err != nil {
				return err
			}
			nextPurge = time.Now().Add(5 * time.Minute)
		}
		if _, err = consumer.RunReplayOnce(ctx); err != nil {
			return err
		}
		batch, fetchErr := broker.Fetch(1, jetstream.FetchMaxWait(time.Second))
		if fetchErr != nil {
			if errors.Is(fetchErr, nats.ErrTimeout) {
				continue
			}
			return fetchErr
		}
		for msg := range batch.Messages() {
			if err = consumer.Consume(ctx, delivery{msg}); err != nil {
				return err
			}
		}
		if err = batch.Error(); err != nil && !errors.Is(err, nats.ErrTimeout) {
			return err
		}
	}
	return ctx.Err()
}

func purgeResolved(ctx context.Context, pool *pgxpool.Pool) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if _, err = events.PurgeExpired(ctx, tx, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
