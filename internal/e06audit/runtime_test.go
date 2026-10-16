package e06audit

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// These probes publish only deliberately INVALID wire. They cannot create a
// trusted business action or impersonate an owner fact. All catalog access is
// read-only; resolution goes through the authenticated production operator API.
func TestLiveInvalidWireQuarantineAndOperatorReceipt(t *testing.T) {
	if os.Getenv("E06_AUDIT_INTEGRATION") != "1" {
		t.Skip("explicit owned E06 Audit runtime disabled")
	}
	for _, key := range []string{"E06_AUDIT_URL", "E06_AUDIT_DATABASE_URL", "E06_AUDIT_NATS_URL", "E06_AUDIT_NATS_STORAGE_PASSWORD", "E06_AUDIT_OPERATOR_TOKEN"} {
		if os.Getenv(key) == "" {
			t.Fatal("owned Audit runtime setting missing")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("E06_AUDIT_DATABASE_URL"))
	if err != nil || pool.Ping(ctx) != nil {
		t.Fatal("owned Audit catalog connection failed")
	}
	defer pool.Close()
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		t.Fatal("private probe nonce failed")
	}
	sentinel := "private-audit-wire-" + hex.EncodeToString(nonce[:])
	// Every untrusted location carries the sentinel, including a field NAME.
	raw, _ := json.Marshal(map[string]any{"eventId": sentinel, "data": map[string]string{sentinel: sentinel}})
	hash := sha256.Sum256(raw)
	subject := "leafrun.audit.storage.action.v1"
	nc, err := nats.Connect(os.Getenv("E06_AUDIT_NATS_URL"), nats.UserInfo("storage", os.Getenv("E06_AUDIT_NATS_STORAGE_PASSWORD")), nats.CustomInboxPrefix("_INBOX.storage"), nats.NoReconnect(), nats.Timeout(3*time.Second), nats.ErrorHandler(func(*nats.Conn, *nats.Subscription, error) {}))
	if err != nil {
		t.Fatal("owned invalid-wire publisher connection failed")
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal("owned invalid-wire JetStream client failed")
	}
	for range 2 {
		msg := &nats.Msg{Subject: subject, Data: raw, Header: nats.Header{"Private-Probe": []string{sentinel}}}
		if ack, e := js.PublishMsg(ctx, msg); e != nil || ack == nil || ack.Stream != "LEAFRUN_AUDIT" {
			t.Fatal("invalid-wire durable publish failed")
		}
	}
	var target string
	deadline := time.Now().Add(20 * time.Second)
	for {
		var observations int
		err = pool.QueryRow(ctx, `SELECT id,observations FROM audit_quarantine WHERE payload_hash=$1 AND subject=$2 AND reason='invalid_wire' AND resolved_at IS NULL`, hash[:], subject).Scan(&target, &observations)
		if err == nil && target != "" && observations >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("invalid wire was not durably metadata-quarantined before ACK")
		}
		time.Sleep(100 * time.Millisecond)
	}
	assertNoPrivatePersistence(t, ctx, pool, sentinel)
	request := map[string]string{"operationKey": sentinel, "action": "audit.resolve", "targetKind": "quarantine", "targetId": target, "reason": "discard_invalid"}
	base := os.Getenv("E06_AUDIT_URL") + "/internal/v1/audit/operator-actions"
	if status, _ := runtimeRequest(t, ctx, base, "", request); status != 401 {
		t.Fatal("anonymous operator action was not denied")
	}
	if status, _ := runtimeRequest(t, ctx, base, sentinel, request); status != 401 {
		t.Fatal("unrecognized operator action was not denied")
	}
	credential := os.Getenv("E06_AUDIT_OPERATOR_TOKEN")
	spoofed := map[string]string{}
	for key, value := range request {
		spoofed[key] = value
	}
	spoofed["actorId"] = sentinel
	if status, _ := runtimeRequest(t, ctx, base, credential, spoofed); status != 400 {
		t.Fatal("caller-selected operator identity was accepted")
	}
	status, first := runtimeRequest(t, ctx, base, credential, request)
	if status != 200 {
		t.Fatal("authenticated invalid-wire resolution failed")
	}
	var receipt struct {
		OperationID string `json:"operationId"`
		Action      string `json:"action"`
		TargetKind  string `json:"targetKind"`
		TargetID    string `json:"targetId"`
	}
	if json.Unmarshal(first, &receipt) != nil || receipt.OperationID == "" || receipt.Action != "audit.resolve" || receipt.TargetKind != "quarantine" || receipt.TargetID != target {
		t.Fatal("operator receipt invalid")
	}
	status, second := runtimeRequest(t, ctx, base, credential, request)
	if status != 200 || !bytes.Equal(first, second) {
		t.Fatal("operator retry changed its immutable receipt")
	}
	request["reason"] = "abandoned"
	if status, _ = runtimeRequest(t, ctx, base, credential, request); status != 409 {
		t.Fatal("changed operator retry did not conflict")
	}
	var receipts, ledger int
	var resolved bool
	if pool.QueryRow(ctx, `SELECT count(*) FROM audit_operator_receipts WHERE operation_id=$1`, receipt.OperationID).Scan(&receipts) != nil || receipts != 1 {
		t.Fatal("operator receipt duplicated or absent")
	}
	if pool.QueryRow(ctx, `SELECT count(*) FROM audit_operator_ledger WHERE operation_id=$1 AND actor_id<>'' AND authority_id<>'' AND authority_version>0 AND action='audit.resolve' AND target_kind='quarantine' AND target_id=$2 AND reason='discard_invalid'`, receipt.OperationID, target).Scan(&ledger) != nil || ledger != 1 {
		t.Fatal("atomic server-authenticated operator ledger absent")
	}
	if pool.QueryRow(ctx, `SELECT resolved_at IS NOT NULL FROM audit_quarantine WHERE id=$1`, target).Scan(&resolved) != nil || !resolved {
		t.Fatal("operator effect absent after receipt")
	}
	assertNoPrivatePersistence(t, ctx, pool, sentinel)
	t.Log("Actual private NATS invalid wire: duplicate metadata quarantine, raw body/header/EventID exclusion, authenticated operator resolution/receipt/ledger and conflict passed")
}

func assertNoPrivatePersistence(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sentinel string) {
	t.Helper()
	for _, query := range []string{
		`SELECT EXISTS(SELECT 1 FROM audit_quarantine q WHERE position($1 in row_to_json(q)::text)>0)`,
		`SELECT EXISTS(SELECT 1 FROM audit_actions a WHERE position($1 in convert_from(action_data,'UTF8'))>0 OR position($1 in row_to_json(a)::text)>0)`,
		`SELECT EXISTS(SELECT 1 FROM platform_event_inbox WHERE position($1 in convert_from(encoded_envelope,'UTF8'))>0 OR position($1 in event_id)>0 OR position($1 in coalesce(last_error,''))>0)`,
		`SELECT EXISTS(SELECT 1 FROM platform_event_history h WHERE position($1 in row_to_json(h)::text)>0)`,
		`SELECT EXISTS(SELECT 1 FROM audit_operator_ledger l WHERE position($1 in row_to_json(l)::text)>0)`,
		`SELECT EXISTS(SELECT 1 FROM audit_operator_receipts r WHERE position($1 in row_to_json(r)::text)>0)`,
	} {
		var leaked bool
		if pool.QueryRow(ctx, query, sentinel).Scan(&leaked) != nil {
			t.Fatal("owned Audit private-persistence check failed")
		}
		if leaked {
			t.Fatal("raw private probe entered durable Audit records")
		}
	}
}

func runtimeRequest(t *testing.T, ctx context.Context, target, credential string, input any) (int, []byte) {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal("operator probe encoding failed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(raw))
	if err != nil {
		t.Fatal("operator probe request invalid")
	}
	req.Header.Set("Content-Type", "application/json")
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address == "127.0.0.1:58199" && os.Getenv("E06_OWNER_INTEGRATION") == "true" {
			u, _ := url.Parse("http://audit-api:8199")
			address = u.Host
		}
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, address)
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer transport.CloseIdleConnections()
	response, err := client.Do(req)
	if err != nil {
		t.Fatal("operator runtime request failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 8193))
	if err != nil || len(body) > 8192 {
		t.Fatal("operator bounded response failed")
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("operator private response cache control absent")
	}
	return response.StatusCode, body
}
