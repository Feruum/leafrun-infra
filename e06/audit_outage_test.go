package e06

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	access "github.com/leafrun/leafrun-contracts/access/v1"
	audit "github.com/leafrun/leafrun-contracts/audit/v1"
	identity "github.com/leafrun/leafrun-contracts/identity/v1"
	"github.com/leafrun/leafrun-platform/events"
)

type auditOutage struct {
	directory       string
	RunID           string                      `json:"runId"`
	Scope           access.PermissionScope      `json:"scope"`
	Resources       map[audit.ActionName]string `json:"resources,omitempty"`
	IdentityEventID string                      `json:"identityEventId,omitempty"`
}

// ROOT alone stops/restarts the owned broker and consumer. A run-bound marker
// handshake permits actual state commits during that outage without giving the
// test Docker control or touching production outbox rows. The directory must
// be fresh and explicitly mounted by ROOT; normal qualification has no gates.
func startAuditOutage(t *testing.T, scope access.PermissionScope) *auditOutage {
	t.Helper()
	directory := os.Getenv("E06_AUDIT_OUTAGE_DIRECTORY")
	if directory == "" {
		return nil
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		t.Fatal("root-controlled outage evidence directory unavailable")
	}
	gate := &auditOutage{directory: directory, RunID: ownerNonce(t), Scope: scope}
	gate.write(t, "ready.json")
	t.Log("Root-controlled owned NATS outage gate ready")
	gate.wait(t, "broker-down.json")
	return gate
}

func (g *auditOutage) committed(t *testing.T, expected map[audit.ActionName]string, actor identity.Identity) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for action, resource := range expected {
		producer := audit.ProducerGeneration
		switch action {
		case audit.ActionGrantCreate, audit.ActionBotCreate, audit.ActionKeyCreate, audit.ActionKeyRotate, audit.ActionKeyRevoke:
			producer = audit.ProducerAccess
		case audit.ActionReleasePublish:
			producer = audit.ProducerProjects
		case audit.ActionDownloadAuthorized:
			producer = audit.ProducerStorage
		}
		subject, err := audit.Subject(producer)
		if err != nil {
			t.Fatal("outage action producer invalid")
		}
		filter, _ := json.Marshal(map[string]any{"organizationId": g.Scope.OrganizationID, "workspaceId": g.Scope.WorkspaceID, "projectId": g.Scope.ProjectID, "data": map[string]any{"action": action, "resource": map[string]string{"id": resource}}})
		pool := ownerPool(t, "E06_"+strings.ToUpper(string(producer))+"_DATABASE_URL")
		rows, err := pool.Query(ctx, `SELECT encoded_envelope,sent_at IS NULL FROM platform_event_outbox WHERE subject=$1 AND convert_from(encoded_envelope,'UTF8')::jsonb @> $2::jsonb`, subject, filter)
		if err != nil {
			t.Fatal("actual outage owner outbox observation failed")
		}
		count := 0
		for rows.Next() {
			var raw []byte
			var pending bool
			if rows.Scan(&raw, &pending) != nil || !pending {
				rows.Close()
				t.Fatal("actual outage action lacks committed pending owner outbox")
			}
			e, fact, err := events.DecodeAuditAction(subject, raw)
			if err != nil || fact.Action != action || fact.Resource.ID != resource || fact.Actor != actor.Principal || fact.Authority != actor.Authority || e.OrganizationID != g.Scope.OrganizationID || e.WorkspaceID != g.Scope.WorkspaceID || e.ProjectID != g.Scope.ProjectID {
				rows.Close()
				t.Fatal("actual outage owner fact lost immutable provenance")
			}
			count++
		}
		err = rows.Err()
		rows.Close()
		if err != nil || count != 1 {
			t.Fatal("actual outage owner fact missing or logically duplicated")
		}
	}
	g.Resources = expected
	g.write(t, "committed.json")
	t.Log("Actual actions committed with pending Audit outboxes in all four owners during root-controlled broker outage")
	g.wait(t, "broker-restored.json")
}

func (g *auditOutage) write(t *testing.T, name string) {
	t.Helper()
	file, err := os.OpenFile(filepath.Join(g.directory, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("root-controlled outage marker already exists or unavailable")
	}
	err = json.NewEncoder(file).Encode(g)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatal("root-controlled safe outage evidence write failed")
	}
}

func (g *auditOutage) wait(t *testing.T, name string) {
	t.Helper()
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(filepath.Join(g.directory, name))
		var marker struct {
			RunID string `json:"runId"`
		}
		if err == nil && len(data) <= 8192 && json.Unmarshal(data, &marker) == nil && marker.RunID == g.RunID {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("root-controlled outage handshake timed out")
}
