package e06

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/leafrun/leafrun-infra/internal/oauthdemo"
	enums "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"google.golang.org/protobuf/proto"
)

func oauthContainsSecret(data []byte, tokens []string) bool {
	for _, token := range tokens {
		if token == "" {
			continue
		}
		for _, encoded := range []string{token, hex.EncodeToString([]byte(token)), base64.StdEncoding.EncodeToString([]byte(token)), base64.RawStdEncoding.EncodeToString([]byte(token)), base64.URLEncoding.EncodeToString([]byte(token)), base64.RawURLEncoding.EncodeToString([]byte(token))} {
			if bytes.Contains(data, []byte(encoded)) {
				return true
			}
		}
	}
	return false
}

func TestOAuthSecretScanEncodings(t *testing.T) {
	secret := "synthetic-private-credential"
	for _, data := range []string{secret, hex.EncodeToString([]byte(secret)), base64.StdEncoding.EncodeToString([]byte(secret)), base64.RawURLEncoding.EncodeToString([]byte(secret))} {
		if !oauthContainsSecret([]byte(data), []string{"", secret}) {
			t.Fatal("credential encoding escaped scan")
		}
	}
	if oauthContainsSecret([]byte(`{"authority_id":"safe-reference"}`), []string{"", secret}) {
		t.Fatal("reference-only data falsely matched credential")
	}
}

type oauthSilentLogger struct{}

func (oauthSilentLogger) Debug(string, ...interface{}) {}
func (oauthSilentLogger) Info(string, ...interface{})  {}
func (oauthSilentLogger) Warn(string, ...interface{})  {}
func (oauthSilentLogger) Error(string, ...interface{}) {}

// Scan only the two jobs admitted by this test. Credentials and all inspected
// rows/history stay in memory; the persisted evidence contains counts/booleans.
func oauthSecretAbsence(t *testing.T, pool *pgxpool.Pool, jobs []string, grants ...oauthdemo.Grant) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var tokens []string
	for _, grant := range grants {
		tokens = append(tokens, grant.AccessToken, grant.RefreshToken)
	}
	totalBytes, rowCount := 0, 0
	scan := func(data []byte) {
		totalBytes += len(data)
		if totalBytes > 8<<20 || oauthContainsSecret(data, tokens) {
			t.Fatal("bounded owned persistence credential absence check failed")
		}
	}
	for _, table := range []string{"generation_jobs", "generation_admission_receipts", "generation_terminal_outbox", "generation_workflow_starts", "generation_operation_intents", "generation_pin_acquires", "generation_input_resources", "generation_attempts", "generation_publication_candidates", "generation_cleanup_intents"} {
		rows, err := pool.Query(ctx, "SELECT to_jsonb(t)::text FROM "+table+" t WHERE job_id=ANY($1) LIMIT 257", jobs)
		if err != nil {
			t.Fatal("owned persistence scan query failed")
		}
		count := 0
		for rows.Next() {
			var data string
			if rows.Scan(&data) != nil || count >= 256 {
				rows.Close()
				t.Fatal("owned persistence scan bound exceeded")
			}
			scan([]byte(data))
			count++
			rowCount++
		}
		if rows.Err() != nil {
			rows.Close()
			t.Fatal("owned persistence scan read failed")
		}
		rows.Close()
		if (table == "generation_jobs" || table == "generation_admission_receipts" || table == "generation_workflow_starts") && count != len(jobs) {
			t.Fatal("owned persistence evidence incomplete")
		}
	}
	rows, err := pool.Query(ctx, `SELECT to_jsonb(o)::text FROM platform_event_outbox o JOIN generation_terminal_outbox t ON o.event_id=t.event_id WHERE t.job_id=ANY($1) LIMIT 3`, jobs)
	if err != nil {
		t.Fatal("owned event outbox scan query failed")
	}
	outboxCount := 0
	for rows.Next() {
		var data string
		if rows.Scan(&data) != nil {
			t.Fatal("owned outbox scan read failed")
		}
		scan([]byte(data))
		outboxCount++
	}
	if rows.Err() != nil || outboxCount != len(jobs) {
		t.Fatal("owned terminal event evidence incomplete")
	}
	rows.Close()
	temporal, err := client.Dial(client.Options{HostPort: "temporal:7233", Namespace: "leafrun-generation", Logger: oauthSilentLogger{}})
	if err != nil {
		t.Fatal("owned Temporal history scan unavailable")
	}
	defer temporal.Close()
	eventCount := 0
	for _, job := range jobs {
		history := temporal.GetWorkflowHistory(ctx, "render/"+job, "", false, enums.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)
		count := 0
		for history.HasNext() {
			event, err := history.Next()
			if err != nil || count >= 512 {
				t.Fatal("owned Temporal history scan failed or exceeded bound")
			}
			data, err := proto.Marshal(event)
			if err != nil {
				t.Fatal("owned Temporal event scan failed")
			}
			scan(data)
			count++
			eventCount++
		}
		if count == 0 {
			t.Fatal("owned Temporal history evidence empty")
		}
	}
	receipt, _ := json.Marshal(map[string]any{"ownedJobs": len(jobs), "persistenceRows": rowCount, "terminalOutboxRows": outboxCount, "temporalEvents": eventCount, "scannedBytes": totalBytes, "exactCredentialsAbsent": true, "logsAvailableInHarness": false})
	if os.WriteFile("/evidence/oauth-secret-absence.json", receipt, 0600) != nil {
		t.Fatal("credential absence receipt failed")
	}
}
