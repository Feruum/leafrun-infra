package e06

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	access "github.com/leafrun/leafrun-contracts/access/v1"
	generation "github.com/leafrun/leafrun-contracts/generation/v1"
	identity "github.com/leafrun/leafrun-contracts/identity/v1"
	projects "github.com/leafrun/leafrun-contracts/projects/v1"
	storage "github.com/leafrun/leafrun-contracts/storage/v1"
	"strings"
	"sync"
	"testing"
	"time"
)

var botGenerateOps = []string{"generation.create", "generation.execute", "generation.publish", "generation.read"}

func task06BotDeniedSession(t *testing.T, node, secret string) {
	t.Helper()
	if _, e := ownerAccess(t, node).Session(context.Background(), access.ResolveRequest{Bearer: secret}); !errors.Is(e, access.ErrUnauthenticated) {
		t.Fatal("invalidated bot key authenticated on another node")
	}
}
func TestRealBotKeysAcrossNodesAndOriginalWork(t *testing.T) {
	ownerEnabled(t)
	ctx := context.Background()
	manager, reader := ownerIdentity(t), ownerIdentity(t)
	managerSession, readerSession := ownerLogin(t, manager), ownerLogin(t, reader)
	a, b := ownerAccess(t, ownerNodeA), ownerAccess(t, ownerNodeB)
	auth := access.ResolveRequest{Bearer: managerSession.Token}
	org, e := a.CreateOrganization(ctx, auth, access.NameRequest{Name: "Task6 bot lifecycle"})
	if e != nil {
		t.Fatal("bot fixture organization failed")
	}
	if _, e = b.SetOrganizationMembership(ctx, auth, org.OrganizationID, access.MembershipRequest{HumanID: reader.ID, Role: "member"}); e != nil {
		t.Fatal("reader membership failed")
	}
	projectPool, generationPool, accessPool := ownerPool(t, "E06_PROJECTS_DATABASE_URL"), ownerPool(t, "E06_GENERATION_DATABASE_URL"), ownerPool(t, "E06_ACCESS_DATABASE_URL")
	scope := storage.Scope{WorkspaceID: "bot-render-" + ownerNonce(t), ProjectID: "same-project-id"}
	_, revision := ownerSeedRevision(t, projectPool, scope, ownerSuccessSource)
	if _, e = ownerImport(t, ownerNodeA, org.OrganizationID, scope, revision, manager.ID); e != nil {
		t.Fatal("exact bot scope import failed")
	}
	permissionScope := access.PermissionScope{OrganizationID: org.OrganizationID, WorkspaceID: scope.WorkspaceID, ProjectID: scope.ProjectID}
	bot, e := a.CreateBot(ctx, auth, scope.WorkspaceID, scope.ProjectID, access.CreateBotRequest{OperationKey: "create-" + ownerNonce(t), Name: "Compiler"})
	if e != nil {
		t.Fatal("bot create failed")
	}
	// Body cannot choose initiating actor or overwrite the server-owned bot ID.
	status, _ := ownerRequest(t, ownerHTTP(nil), "POST", ownerNodeA+"/v1/workspaces/"+scope.WorkspaceID+"/projects/"+scope.ProjectID+"/bots", managerSession.Token, "", "application/json", map[string]any{"operationKey": "spoof", "name": "Forged", "botId": bot.BotID, "actor": manager.ID})
	if status != 400 {
		t.Fatal("body bot/actor spoof accepted")
	}
	if _, e = b.Bind(ctx, auth, permissionScope, access.BindingRequest{SubjectKind: "bot", SubjectID: bot.BotID, Relation: "generate"}, false); e != nil {
		t.Fatal("bot generate binding failed")
	}
	ownerBind(t, b, auth, permissionScope, reader.ID, "download", false)
	request := access.BotKeyRequest{OperationKey: "issue-" + ownerNonce(t), Operations: botGenerateOps}
	issued, e := a.IssueBotKey(ctx, auth, bot.BotID, request)
	if e != nil || issued.Disclosure != "first" || len(issued.Secret) != 50 {
		t.Fatal("first bot disclosure failed")
	}
	receipt, e := b.IssueBotKey(ctx, auth, bot.BotID, request)
	if e != nil || receipt.Secret != "" || receipt.Disclosure != "unavailable" || receipt.Key.KeyID != issued.Key.KeyID {
		t.Fatal("lost-disclosure receipt replay failed")
	}
	initial, e := a.Session(ctx, access.ResolveRequest{Bearer: issued.Secret})
	if e != nil || initial.Identity.Principal.Domain != "bot" || initial.Identity.Principal.Subject != bot.BotID {
		t.Fatal("server-derived bot principal failed")
	}
	if _, e = a.CreateOrganization(ctx, access.ResolveRequest{Bearer: issued.Secret}, access.NameRequest{Name: "Upward"}); e == nil {
		t.Fatal("bot bootstrapped human organization")
	}
	var humanCount int
	if e = accessPool.QueryRow(ctx, `SELECT count(*) FROM access_humans WHERE human_id=$1`, bot.BotID).Scan(&humanCount); e != nil || humanCount != 0 {
		t.Fatal("bot inserted as human")
	}
	release := ownerRelease(t, managerSession.Token, scope, revision)
	publisher, _ := projects.NewClient(ownerProjects, managerSession.Token, ownerHTTP(nil))
	release, e = publisher.SetPublication(ctx, release.ReleaseID, projects.PublicationRequest{Scope: scope, ExpectedVersion: release.PublicationVersion, Published: true})
	if e != nil {
		t.Fatal("bot published fixture failed")
	}
	selection := generation.RenderSelectionRequest{Scope: scope, ReleaseID: release.ReleaseID, Parameters: json.RawMessage(`{}`)}
	client, _ := generation.NewClient(ownerGeneration, issued.Secret, ownerHTTP(nil))
	managerGeneration, _ := generation.NewClient(ownerGeneration, managerSession.Token, ownerHTTP(nil))
	operation := "bot-success-" + ownerNonce(t)
	admission, e := client.CreateRender(ctx, operation, selection)
	if e != nil {
		t.Fatal("bot render admission failed")
	}
	job := ownerJob(t, managerGeneration, admission.Job.JobID)
	if job.Status != generation.Succeeded || job.Artifact == nil {
		t.Fatal("valid bot render failed")
	}
	resultURL := ownerStorage + "/v1/artifacts/" + job.Artifact.ArtifactID + "/content"
	status, _ = ownerRequest(t, ownerHTTP(nil), "GET", resultURL, issued.Secret, "", "", nil)
	if status != 404 {
		t.Fatal("generate-only bot acquired download")
	}
	status, pdf := ownerRequest(t, ownerHTTP(nil), "GET", resultURL, readerSession.Token, "", "", nil)
	if status != 200 || !strings.HasPrefix(string(pdf), "%PDF-") {
		t.Fatal("separate current reader cannot download bot output")
	}
	rotated, e := b.RotateBotKey(ctx, auth, bot.BotID, issued.Key.KeyID, access.RotateBotKeyRequest{OperationKey: "rotate-" + ownerNonce(t), ExpectedVersion: 1, Operations: botGenerateOps})
	if e != nil || rotated.Key.Version != 2 {
		t.Fatal("nodeB rotation failed")
	}
	task06BotDeniedSession(t, ownerNodeA, issued.Secret)
	fresh, e := a.Session(ctx, access.ResolveRequest{Bearer: rotated.Secret})
	if e != nil || fresh.Identity.Principal != initial.Identity.Principal || fresh.Identity.Authority.Version != 2 {
		t.Fatal("new key changed bot or version")
	}
	freshClient, _ := generation.NewClient(ownerGeneration, rotated.Secret, ownerHTTP(nil))
	replay, e := freshClient.CreateRender(ctx, operation, selection)
	if e != nil || replay.Job.JobID != job.JobID || replay.Job.Artifact == nil || !replay.Job.Artifact.ExpiresAt.Equal(job.Artifact.ExpiresAt) {
		t.Fatal("same-bot new-key receipt duplicated work or extended TTL")
	}
	task06OriginalBotAuthority(t, generationPool, job.JobID, initial.Identity, []string{issued.Secret, rotated.Secret})
	status, _ = ownerRequest(t, ownerHTTP(nil), "GET", resultURL, readerSession.Token, "", "", nil)
	if status != 200 {
		t.Fatal("historical committed bot artifact lost readability after key rotation")
	}
	// Independent concurrent rotations use the same expected version: one wins.
	var wg sync.WaitGroup
	results := make(chan access.BotKeyResult, 2)
	errs := make(chan error, 2)
	for i, node := range []*access.SessionClient{a, b} {
		wg.Go(func() {
			out, err := node.RotateBotKey(ctx, auth, bot.BotID, rotated.Key.KeyID, access.RotateBotKeyRequest{OperationKey: fmt.Sprintf("distinct-%d-%s", i, ownerNonce(t)), ExpectedVersion: 2, Operations: botGenerateOps})
			results <- out
			errs <- err
		})
	}
	wg.Wait()
	close(results)
	winner := access.BotKeyResult{}
	success := 0
	for out := range results {
		if out.Secret != "" {
			success++
			winner = out
		}
	}
	conflicts := 0
	for range 2 {
		err := <-errs
		if errors.Is(err, access.ErrConflict) {
			conflicts++
		} else if err != nil {
			t.Fatal("rotation race unexpected dependency error")
		}
	}
	if success != 1 || conflicts != 1 || winner.Key.Version != 3 {
		t.Fatal("independent concurrent rotations had multiple current winners")
	}
	task06BotDeniedSession(t, ownerNodeA, rotated.Secret)
	// Pause real final publication after admission; node B invalidates the exact
	// initiating version, then the existing worker resumes without event delivery.
	current := winner
	for _, mode := range []string{"rotate", "revoke"} {
		key := "bot-final-" + mode + "-" + ownerNonce(t)
		task06PausedBotPublication(t, generationPool, a, b, auth, bot.BotID, current, mode, key, selection, managerGeneration, readerSession)
		if mode == "rotate" {
			current, e = a.IssueBotKey(ctx, auth, bot.BotID, access.BotKeyRequest{OperationKey: "next-" + ownerNonce(t), Operations: botGenerateOps})
			if e != nil {
				t.Fatal("fresh second bot key failed")
			}
		}
	}
	expiry := time.Now().Add(500 * time.Millisecond)
	expiring, e := a.IssueBotKey(ctx, auth, bot.BotID, access.BotKeyRequest{OperationKey: "expiry-" + ownerNonce(t), Operations: botGenerateOps, ExpiresAt: &expiry})
	if e != nil {
		t.Fatal("finite bot key failed")
	}
	time.Sleep(650 * time.Millisecond)
	task06BotDeniedSession(t, ownerNodeB, expiring.Secret)
	var metadata string
	if e = accessPool.QueryRow(ctx, `SELECT string_agg(result::text,',') FROM access_bot_receipts WHERE result::text LIKE '%'||$1||'%'`, bot.BotID).Scan(&metadata); e != nil {
		t.Fatal("bot receipt metadata absent")
	}
	for _, secret := range []string{issued.Secret, rotated.Secret, winner.Secret, expiring.Secret} {
		if strings.Contains(metadata, secret) {
			t.Fatal("recoverable secret persisted in bot receipts")
		}
	}
	var hash []byte
	if e = accessPool.QueryRow(ctx, `SELECT secret_hash FROM access_bot_keys WHERE key_id::text=$1`, issued.Key.KeyID).Scan(&hash); e != nil || len(hash) != 32 {
		t.Fatal("key hash storage invalid")
	}
	t.Log("actual A/B bot lifecycle, SHA256-only keys, metadata receipts, real PDF, historical TTL, same-actor replay, rotation/revoke final fencing and expiry passed with no event transport")
}
func task06OriginalBotAuthority(t *testing.T, p *pgxpool.Pool, job string, want identity.Identity, secrets []string) {
	t.Helper()
	var raw []byte
	if e := p.QueryRow(context.Background(), `SELECT original_identity FROM generation_jobs WHERE job_id=$1`, job).Scan(&raw); e != nil {
		t.Fatal("original bot identity missing")
	}
	var saved identity.Identity
	if json.Unmarshal(raw, &saved) != nil || saved != want {
		t.Fatal("fresh key rebound old job authority")
	}
	for _, secret := range secrets {
		if strings.Contains(string(raw), secret) {
			t.Fatal("secret persisted in owner authority")
		}
	}
}

func task06PausedBotPublication(t *testing.T, p *pgxpool.Pool, a, b *access.SessionClient, auth access.ResolveRequest, bot string, current access.BotKeyResult, mode, key string, selection generation.RenderSelectionRequest, manager *generation.Client, reader ownerSession) {
	t.Helper()
	ctx := context.Background()
	resolved, e := a.Session(ctx, access.ResolveRequest{Bearer: current.Secret})
	if e != nil {
		t.Fatal("original bot credential unavailable")
	}
	client, _ := generation.NewClient(ownerGeneration, current.Secret, ownerHTTP(nil))
	suffix := ownerNonce(t)
	function, trigger := "e06_bot_final_"+suffix, "e06_bot_trigger_"+suffix
	lockID := int64(time.Now().UnixNano()&0x3fffffff) + 1
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	sql := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $e06$ BEGIN IF NEW.idempotency_key=%s AND NEW.workspace_id=%s AND NEW.project_id=%s AND NEW.status='succeeded' AND OLD.status<>'succeeded' THEN PERFORM pg_advisory_xact_lock(%d::bigint); END IF; RETURN NEW; END $e06$`, function, quote(key), quote(selection.Scope.WorkspaceID), quote(selection.Scope.ProjectID), lockID)
	if _, e = p.Exec(ctx, sql); e != nil {
		t.Fatal("bot final wait fixture failed")
	}
	if _, e = p.Exec(ctx, "CREATE TRIGGER "+trigger+" BEFORE UPDATE ON generation_jobs FOR EACH ROW EXECUTE FUNCTION "+function+"()"); e != nil {
		t.Fatal("bot final wait trigger failed")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, e := p.Exec(cleanup, "DROP TRIGGER IF EXISTS "+trigger+" ON generation_jobs"); e != nil {
			t.Error("bot final trigger cleanup failed")
		}
		if _, e := p.Exec(cleanup, "DROP FUNCTION IF EXISTS "+function+"()"); e != nil {
			t.Error("bot final function cleanup failed")
		}
	}()
	holder, e := p.Begin(ctx)
	if e != nil {
		t.Fatal("bot final lock holder failed")
	}
	defer holder.Rollback(ctx)
	if _, e = holder.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockID); e != nil {
		t.Fatal("bot final lock failed")
	}
	admission, e := client.CreateRender(ctx, key, selection)
	if e != nil {
		t.Fatal("old bot admitted job failed")
	}
	deadline := time.Now().Add(80 * time.Second)
	for {
		var waiting bool
		if e = p.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND NOT granted AND classid=0 AND objid=$1::oid AND objsubid=1)`, lockID).Scan(&waiting); e != nil {
			t.Fatal("bot final wait observation failed")
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("real bot worker never reached final publication wait")
		}
		time.Sleep(10 * time.Millisecond)
	}
	var artifact string
	if e = p.QueryRow(ctx, `SELECT artifact_id FROM generation_publication_candidates WHERE job_id=$1 ORDER BY created_at DESC LIMIT 1`, admission.Job.JobID).Scan(&artifact); e != nil || artifact == "" {
		t.Fatal("bot final candidate absent")
	}
	resultURL := ownerStorage + "/v1/artifacts/" + artifact + "/content"
	status, _ := ownerRequest(t, ownerHTTP(nil), "GET", resultURL, reader.Token, "", "", nil)
	if status != 404 {
		t.Fatal("pending bot candidate disclosed")
	}
	fresh := access.BotKeyResult{}
	if mode == "rotate" {
		fresh, e = b.RotateBotKey(ctx, auth, bot, current.Key.KeyID, access.RotateBotKeyRequest{OperationKey: "final-rotate-" + ownerNonce(t), ExpectedVersion: current.Key.Version, Operations: botGenerateOps})
	} else {
		_, e = b.RevokeBotKey(ctx, auth, bot, current.Key.KeyID, access.RevokeBotKeyRequest{OperationKey: "final-revoke-" + ownerNonce(t), ExpectedVersion: current.Key.Version})
	}
	if e != nil {
		t.Fatal("nodeB bot invalidation during final pause failed")
	}
	task06BotDeniedSession(t, ownerNodeA, current.Secret)
	if e = holder.Commit(ctx); e != nil {
		t.Fatal("bot final pause release failed")
	}
	job := ownerJob(t, manager, admission.Job.JobID)
	if job.Status != generation.Failed || job.FailureCode != "access_denied" || job.Artifact != nil {
		t.Fatal("old bot work published after invalidation")
	}
	status, _ = ownerRequest(t, ownerHTTP(nil), "GET", resultURL, reader.Token, "", "", nil)
	if status != 404 {
		t.Fatal("invalidated bot candidate exposed")
	}
	if mode == "revoke" {
		fresh, e = a.IssueBotKey(ctx, auth, bot, access.BotKeyRequest{OperationKey: "read-old-receipt-" + ownerNonce(t), Operations: botGenerateOps})
		if e != nil {
			t.Fatal("fresh bot receipt reader failed")
		}
	}
	freshClient, _ := generation.NewClient(ownerGeneration, fresh.Secret, ownerHTTP(nil))
	replay, e := freshClient.CreateRender(ctx, key, selection)
	if e != nil || replay.Job.JobID != job.JobID || replay.Job.Status != generation.Failed {
		t.Fatal("fresh bot key rescued old work or duplicated receipt")
	}
	task06OriginalBotAuthority(t, p, job.JobID, resolved.Identity, []string{current.Secret, fresh.Secret})
	task06RevokedBotCleanup(t, p, job.JobID)
}

func task06RevokedBotCleanup(t *testing.T, p *pgxpool.Pool, job string) {
	t.Helper()
	ctx := context.Background()
	rows, e := p.Query(ctx, `SELECT storage_pin_id FROM generation_pin_acquires WHERE job_id=$1 AND response IS NOT NULL AND storage_pin_id<>''`, job)
	if e != nil {
		t.Fatal("bot cleanup pins unavailable")
	}
	var pins []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			t.Fatal("bot cleanup pin binding invalid")
		}
		pins = append(pins, id)
	}
	rows.Close()
	if rows.Err() != nil || len(pins) == 0 {
		t.Fatal("bot cleanup lacks actual pins")
	}
	// Make only this owned failed fixture's 24-hour diagnostic retention due.
	// The production janitor/owner callback still performs the real service cleanup.
	if _, e = p.Exec(ctx, `UPDATE generation_jobs SET terminal_at=clock_timestamp()-interval '25 hours' WHERE job_id=$1 AND status='failed'`, job); e != nil {
		t.Fatal("bot cleanup retention fixture failed")
	}
	if _, e = p.Exec(ctx, `UPDATE generation_cleanup_intents SET not_before=clock_timestamp() WHERE job_id=$1 AND kind='pin.release' AND state='pending'`, job); e != nil {
		t.Fatal("bot cleanup due fixture failed")
	}
	storagePool := ownerPool(t, "E06_STORAGE_DATABASE_URL")
	deadline := time.Now().Add(40 * time.Second)
	for {
		var active, complete, storageActive int
		if e = p.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state<>'released') FROM generation_pin_acquires WHERE job_id=$1 AND response IS NOT NULL AND storage_pin_id<>''`, job).Scan(&active); e != nil {
			t.Fatal("bot cleanup generation observation failed")
		}
		if e = p.QueryRow(ctx, `SELECT count(*) FROM generation_cleanup_intents WHERE job_id=$1 AND kind='pin.release' AND state='complete'`, job).Scan(&complete); e != nil {
			t.Fatal("bot cleanup receipt observation failed")
		}
		if e = storagePool.QueryRow(ctx, `SELECT count(*) FROM storage_pins WHERE id=ANY($1) AND state<>'released'`, pins).Scan(&storageActive); e != nil {
			t.Fatal("bot cleanup Storage observation failed")
		}
		if active == 0 && storageActive == 0 && complete >= len(pins) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("revoked bot authority prevented separate service pin cleanup")
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Log("revoked-key service pin cleanup completed after only the owned failed fixture retention was made due")
}
