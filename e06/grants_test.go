package e06

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	access "github.com/leafrun/leafrun-contracts/access/v1"
	generation "github.com/leafrun/leafrun-contracts/generation/v1"
)

const task08Cookie = "leafrun_guest_session"

func guestFixture(t *testing.T) guestBrowserFixture {
	t.Helper()
	if os.Getenv("E06_GUEST_OWNER_INTEGRATION") != "true" {
		t.Skip("actual guest owner lifecycle not enabled")
	}
	ownerEnabled(t)
	data, e := os.ReadFile("/evidence/guest-browser-fixture.json")
	var f guestBrowserFixture
	if e != nil || json.Unmarshal(data, &f) != nil || f.Browser.Human.ID == "" {
		t.Fatal("separate guest fixture unavailable")
	}
	return f
}
func guestJar(t *testing.T, cookie string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	if cookie != "" {
		u, _ := url.Parse(ownerNodeA)
		jar.SetCookies(u, []*http.Cookie{{Name: task08Cookie, Value: cookie, Path: "/"}})
	}
	return guestHTTP(jar)
}

// Positive controls wait only for the existing finite policy projection window;
// denial statuses remain asserted exactly and credentials are never substituted.
func guestHTTP(jar http.CookieJar) *http.Client {
	c := ownerHTTP(jar)
	c.Transport = guestTransport{c.Transport}
	return c
}

type guestTransport struct{ base http.RoundTripper }

func (r guestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Path == "/v1/grants/exchange" || request.URL.Path == "/v1/guest/logout" {
		// A dependency denial may already have consumed the one-use challenge.
		// Never replay a protected action transparently with the same state.
		return r.base.RoundTrip(request)
	}
	return (ownerPolicyRetry{r.base}).RoundTrip(request)
}
func guestChallenge(t *testing.T, c *http.Client, action string) string {
	t.Helper()
	status, data := ownerRequest(t, c, "GET", ownerNodeA+"/v1/guest/challenge?action="+action, "", ownerNodeA, "", nil)
	var out struct {
		CSRF string `json:"csrf"`
	}
	if status != 200 || json.Unmarshal(data, &out) != nil || len(out.CSRF) != 43 {
		t.Fatalf("guest challenge failed: status=%d", status)
	}
	return out.CSRF
}
func guestExchange(t *testing.T, secret string) (*http.Client, string, access.ResolveResponse) {
	t.Helper()
	c := guestJar(t, "")
	csrf := guestChallenge(t, c, "exchange")
	status, data := ownerRequest(t, c, "POST", ownerNodeA+"/v1/grants/exchange", "", ownerNodeA, "application/json", access.ExchangeGrantRequest{Secret: secret, CSRF: csrf})
	var out access.ResolveResponse
	if status != 200 || json.Unmarshal(data, &out) != nil || out.Identity.Principal.Domain != "guest" || out.Identity.Authority.Kind != "guest_session" || out.ExpiresAt == nil {
		t.Fatalf("protected guest exchange failed: status=%d", status)
	}
	u, _ := url.Parse(ownerNodeA)
	cookie := ""
	for _, value := range c.Jar.Cookies(u) {
		if value.Name == task08Cookie {
			cookie = value.Value
		}
	}
	if cookie == "" || cookie == secret {
		t.Fatal("separate guest credential absent")
	}
	return c, cookie, out
}
func guestSessionStatus(t *testing.T, c *http.Client, want int) {
	t.Helper()
	for _, node := range []string{ownerNodeA, ownerNodeB} {
		status, _ := ownerRequest(t, c, "GET", node+"/v1/session", "", "", "", nil)
		if status != want {
			t.Fatalf("guest A/B session status=%d want=%d", status, want)
		}
	}
}

func guestOwnersDenied(t *testing.T, f guestBrowserFixture, credential, replayKey string, selection generation.RenderSelectionRequest) {
	t.Helper()
	c := guestJar(t, credential)
	for _, endpoint := range []string{ownerGeneration + "/v1/jobs/" + f.JobID, ownerProjects + "/v1/releases/" + f.Browser.ReleaseID, ownerStorage + "/v1/artifacts/" + f.ArtifactID, ownerStorage + "/v1/artifacts/" + f.ArtifactID + "/content"} {
		status, _ := ownerRequest(t, c, "GET", endpoint, "", "", "", nil)
		if status != 401 {
			t.Fatalf("stale guest owner read/source/download not denied: status=%d", status)
		}
	}
	body, _ := json.Marshal(selection)
	r, _ := http.NewRequest("POST", ownerGeneration+"/v1/renders", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", ownerGeneration)
	r.Header.Set("Idempotency-Key", replayKey)
	response, err := c.Do(r)
	if err != nil {
		t.Fatal("stale guest replay request failed")
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatalf("stale guest accepted original render replay: status=%d", response.StatusCode)
	}
}
func guestIssue(t *testing.T, f guestBrowserFixture, manager ownerSession, expiry time.Time) access.GrantResult {
	t.Helper()
	in := access.CreateGrantRequest{OperationKey: ownerGrantOperationKey(t), Role: "editor", ExpiresAt: expiry}
	route := ownerNodeB + "/v1/workspaces/" + f.Browser.Scope.WorkspaceID + "/projects/" + f.Browser.Scope.ProjectID + "/grants"
	status, data := ownerRequest(t, guestHTTP(nil), "POST", route, manager.Token, ownerNodeA, "application/json", in)
	var first, receipt access.GrantResult
	if status != 200 || json.Unmarshal(data, &first) != nil || first.Secret == "" || first.Disclosure != "first" || first.Grant.Version != 1 {
		t.Fatalf("guest issue failed: status=%d", status)
	}
	status, data = ownerRequest(t, guestHTTP(nil), "POST", route, manager.Token, ownerNodeA, "application/json", in)
	if status != 200 || json.Unmarshal(data, &receipt) != nil || receipt.Secret != "" || receipt.Disclosure != "unavailable" || receipt.Grant.GrantID != first.Grant.GrantID {
		t.Fatalf("grant retry invalid: status=%d secretPresent=%t sameGrant=%t", status, receipt.Secret != "", receipt.Grant.GrantID == first.Grant.GrantID)
	}
	return first
}
func guestRotate(t *testing.T, manager ownerSession, current access.GrantResult) access.GrantResult {
	t.Helper()
	in := access.RotateGrantRequest{OperationKey: "guest-rotate-" + ownerNonce(t), ExpectedVersion: current.Grant.Version, Role: "editor", ExpiresAt: current.Grant.ExpiresAt}
	status, data := ownerRequest(t, guestHTTP(nil), "POST", ownerNodeB+"/v1/grants/"+current.Grant.GrantID+"/rotate", manager.Token, ownerNodeA, "application/json", in)
	var out access.GrantResult
	if status != 200 || json.Unmarshal(data, &out) != nil || out.Grant.Version != current.Grant.Version+1 || out.Secret == "" || out.Grant.GrantID != current.Grant.GrantID {
		t.Fatalf("nodeB grant rotation failed: status=%d", status)
	}
	ownerPermission(t, manager, generation.Scope{WorkspaceID: out.Grant.Scope.WorkspaceID, ProjectID: out.Grant.Scope.ProjectID}, "download", true)
	return out
}
func guestRevoke(t *testing.T, manager ownerSession, current access.GrantResult) {
	t.Helper()
	status, _ := ownerRequest(t, guestHTTP(nil), "DELETE", ownerNodeB+"/v1/grants/"+current.Grant.GrantID, manager.Token, ownerNodeA, "application/json", access.RevokeGrantRequest{OperationKey: "guest-revoke-" + ownerNonce(t), ExpectedVersion: current.Grant.Version})
	if status != 200 {
		t.Fatalf("nodeB grant revoke failed: status=%d", status)
	}
}

func TestRealGuestGrantLifecycle(t *testing.T) {
	f := guestFixture(t)
	manager := ownerLogin(t, f.Browser.Human)
	defer ownerLogout(t, manager)
	issued := guestIssue(t, f, manager, time.Now().UTC().Add(30*time.Minute))
	c, credential, first := guestExchange(t, issued.Secret)
	other, otherCredential, second := guestExchange(t, issued.Secret)
	if first.Identity.Principal != second.Identity.Principal || first.Identity.Authority == second.Identity.Authority || credential == otherCredential {
		t.Fatal("repeated exchange did not create independent guest sessions")
	}
	guestSessionStatus(t, c, 200)
	if first.ExpiresAt.After(issued.Grant.ExpiresAt) {
		t.Fatal("guest expiry exceeded grant ceiling")
	}
	selection := generation.RenderSelectionRequest{Scope: f.Browser.Scope, ReleaseID: f.Browser.ReleaseID, Parameters: json.RawMessage(`{}`)}
	replayKey := "guest-lifecycle-success-" + ownerNonce(t)
	guestGeneration, _ := generation.NewCookieClient(ownerGeneration, task08Cookie, credential, ownerGeneration, true, guestHTTP(nil))
	admitted, err := guestGeneration.CreateRender(context.Background(), replayKey, selection)
	if err != nil {
		t.Fatal("cookie-only guest render admission failed")
	}
	managerGeneration, _ := generation.NewClient(ownerGeneration, manager.Token, guestHTTP(nil))
	guestSuccess := ownerJob(t, managerGeneration, admitted.Job.JobID)
	if guestSuccess.Status != generation.Succeeded || guestSuccess.Artifact == nil {
		t.Fatal("cookie-only guest render did not succeed")
	}
	task06OriginalBotAuthority(t, ownerPool(t, "E06_GENERATION_DATABASE_URL"), guestSuccess.JobID, first.Identity, []string{issued.Secret, credential})
	resultURL := ownerStorage + "/v1/artifacts/" + f.ArtifactID + "/content"
	status, pdf := ownerRequest(t, c, "GET", resultURL, "", "", "", nil)
	if status != 200 || !strings.HasPrefix(string(pdf), "%PDF-") {
		t.Fatal("editor guest historical download denied")
	}
	for _, endpoint := range []string{ownerGeneration + "/v1/jobs/" + f.JobID, ownerProjects + "/v1/releases/" + f.Browser.ReleaseID, ownerStorage + "/v1/artifacts/" + f.ArtifactID} {
		status, _ = ownerRequest(t, c, "GET", endpoint, "", "", "", nil)
		if status != 200 {
			t.Fatalf("guest owner read failed: status=%d", status)
		}
	}
	status, _ = ownerRequest(t, c, "POST", ownerNodeA+"/v1/organizations", "", ownerNodeA, "application/json", map[string]string{"name": "Guest escape"})
	if status != 403 {
		t.Fatalf("guest organization management escaped: status=%d", status)
	}
	status, _ = ownerRequest(t, c, "POST", ownerNodeA+"/v1/workspaces/"+f.Browser.Scope.WorkspaceID+"/projects/"+f.Browser.Scope.ProjectID+"/grants", "", ownerNodeA, "application/json", access.CreateGrantRequest{OperationKey: ownerGrantOperationKey(t), Role: "reader", ExpiresAt: time.Now().UTC().Add(time.Hour)})
	if status != 403 {
		t.Fatalf("guest delegated another grant: status=%d", status)
	}
	for _, endpoint := range []string{ownerNodeA + "/v1/session", ownerNodeB + "/v1/session", ownerGeneration + "/v1/jobs/" + f.JobID, ownerProjects + "/v1/releases/" + f.Browser.ReleaseID, ownerStorage + "/v1/artifacts/" + f.ArtifactID} {
		request, _ := http.NewRequest("GET", endpoint, nil)
		request.Header.Set("Cookie", task08Cookie+"="+credential+"; ory_kratos_session=selected-invalid-human")
		response, e := guestHTTP(nil).Do(request)
		if e != nil {
			t.Fatal("ambiguity request failed")
		}
		response.Body.Close()
		if response.StatusCode != 401 {
			t.Fatalf("both cookies gained precedence: status=%d", response.StatusCode)
		}
		status, _ = ownerRequest(t, c, "GET", endpoint, manager.Token, "", "", nil)
		if status != 401 {
			t.Fatalf("bearer plus guest cookie accepted: status=%d", status)
		}
	}
	// Logout only one separate guest authority; reusable grant and other guests survive.
	csrf := guestChallenge(t, c, "logout")
	status, _ = ownerRequest(t, c, "POST", ownerNodeA+"/v1/guest/logout", "", ownerNodeA, "application/json", access.GuestLogoutRequest{CSRF: csrf})
	if status != 204 {
		t.Fatalf("guest logout failed: status=%d", status)
	}
	guestSessionStatus(t, guestJar(t, credential), 401)
	guestOwnersDenied(t, f, credential, replayKey, selection)
	guestSessionStatus(t, other, 200)
	c, credential, first = guestExchange(t, issued.Secret)
	// Issuance credential liveness is provenance, independent of guest authority.
	issuerOnly := ownerLogin(t, f.Browser.Human)
	separateGrant := guestIssue(t, f, issuerOnly, time.Now().UTC().Add(20*time.Minute))
	separate, _, _ := guestExchange(t, separateGrant.Secret)
	ownerLogout(t, issuerOnly)
	guestSessionStatus(t, separate, 200)
	// Stable issuer enablement suspends; restoration permits the same live version.
	setIssuer := func(state string) {
		status, _ := ownerRequest(t, guestHTTP(nil), "PUT", os.Getenv("E06_KRATOS_ADMIN_URL")+"/admin/identities/"+f.Browser.Human.ID, "", "", "application/json", map[string]any{"schema_id": "default", "state": state, "traits": map[string]string{"email": f.Browser.Human.Email}})
		if status != 200 {
			t.Fatal("issuer state fixture update failed")
		}
	}
	setIssuer("inactive")
	t.Cleanup(func() { setIssuer("active") })
	guestSessionStatus(t, c, 401)
	setIssuer("active")
	guestSessionStatus(t, c, 200)
	fresh := guestRotate(t, manager, issued)
	guestSessionStatus(t, c, 401)
	guestSessionStatus(t, other, 401)
	guestOwnersDenied(t, f, credential, replayKey, selection)
	freshClient, freshCredential, freshIdentity := guestExchange(t, fresh.Secret)
	if freshIdentity.Identity.Principal != first.Identity.Principal || freshIdentity.Identity.Authority == first.Identity.Authority {
		t.Fatal("rotation changed grant actor or reused guest authority")
	}
	// Rotated old grant versions remain dead through issuer suspension/restoration.
	setIssuer("inactive")
	setIssuer("active")
	guestSessionStatus(t, guestJar(t, credential), 401)
	guestSessionStatus(t, freshClient, 200)
	guestRevoke(t, manager, fresh)
	guestSessionStatus(t, guestJar(t, freshCredential), 401)
	guestOwnersDenied(t, f, freshCredential, replayKey, selection)
	guestSessionStatus(t, separate, 200)
	short := guestIssue(t, f, manager, time.Now().UTC().Add(3*time.Second))
	shortClient, _, shortIdentity := guestExchange(t, short.Secret)
	if shortIdentity.ExpiresAt.After(short.Grant.ExpiresAt) {
		t.Fatal("short session exceeded grant expiry")
	}
	time.Sleep(time.Until(short.Grant.ExpiresAt) + 100*time.Millisecond)
	guestSessionStatus(t, shortClient, 401)
	// Current separate readers retain the immutable historical result expiry.
	gc, _ := generation.NewClient(ownerGeneration, manager.Token, guestHTTP(nil))
	before := ownerJob(t, gc, f.JobID)
	status, _ = ownerRequest(t, guestHTTP(nil), "GET", resultURL, manager.Token, "", "", nil)
	if status != 200 {
		t.Fatal("authorized historical result lost after grant revocation")
	}
	after := ownerJob(t, gc, f.JobID)
	if before.Artifact == nil || after.Artifact == nil || !before.Artifact.ExpiresAt.Equal(after.Artifact.ExpiresAt) {
		t.Fatal("guest lifecycle extended historical artifact TTL")
	}
	guestRevoke(t, manager, separateGrant)
	t.Log("Actual PG/Keto A/B grants: first-disclosure retry, independent guest logout/issuer logout, issuer loss/restoration, rotation/revoke, expiry, owner reads/download and ambiguity passed without event transport")
}

func TestRealGuestOriginalWorkFinalBoundary(t *testing.T) {
	f := guestFixture(t)
	manager := ownerLogin(t, f.Browser.Human)
	defer ownerLogout(t, manager)
	pool := ownerPool(t, "E06_GENERATION_DATABASE_URL")
	managerClient, _ := generation.NewClient(ownerGeneration, manager.Token, guestHTTP(nil))
	selection := generation.RenderSelectionRequest{Scope: f.Browser.Scope, ReleaseID: f.Browser.ReleaseID, Parameters: json.RawMessage(`{}`)}
	for _, mode := range []string{"rotate", "revoke"} {
		t.Run(mode, func(t *testing.T) {
			issued := guestIssue(t, f, manager, time.Now().UTC().Add(30*time.Minute))
			guestPausedPublication(t, pool, manager, issued, mode, selection, managerClient)
		})
	}
}

func guestPausedPublication(t *testing.T, p *pgxpool.Pool, manager ownerSession, current access.GrantResult, mode string, selection generation.RenderSelectionRequest, managerClient *generation.Client) {
	t.Helper()
	ctx := context.Background()
	c, credential, resolved := guestExchange(t, current.Secret)
	_ = c
	client, e := generation.NewCookieClient(ownerGeneration, task08Cookie, credential, ownerGeneration, true, guestHTTP(nil))
	if e != nil {
		t.Fatal("guest generation cookie client invalid")
	}
	key := "guest-final-" + mode + "-" + ownerNonce(t)
	suffix := ownerNonce(t)
	function, trigger := "e06_guest_final_"+suffix, "e06_guest_trigger_"+suffix
	lockID := int64(time.Now().UnixNano()&0x3fffffff) + 1
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
	sql := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $e06$ BEGIN IF NEW.idempotency_key=%s AND NEW.workspace_id=%s AND NEW.project_id=%s AND NEW.status='succeeded' AND OLD.status<>'succeeded' THEN PERFORM pg_advisory_xact_lock(%d::bigint); END IF; RETURN NEW; END $e06$`, function, quote(key), quote(selection.Scope.WorkspaceID), quote(selection.Scope.ProjectID), lockID)
	if _, e = p.Exec(ctx, sql); e != nil {
		t.Fatal("guest final wait fixture failed")
	}
	if _, e = p.Exec(ctx, "CREATE TRIGGER "+trigger+" BEFORE UPDATE ON generation_jobs FOR EACH ROW EXECUTE FUNCTION "+function+"()"); e != nil {
		t.Fatal("guest final trigger failed")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, e := p.Exec(cleanup, "DROP TRIGGER IF EXISTS "+trigger+" ON generation_jobs"); e != nil {
			t.Error("guest final trigger cleanup failed")
		}
		if _, e := p.Exec(cleanup, "DROP FUNCTION IF EXISTS "+function+"()"); e != nil {
			t.Error("guest final function cleanup failed")
		}
	}()
	holder, e := p.Begin(ctx)
	if e != nil {
		t.Fatal("guest final lock holder failed")
	}
	defer holder.Rollback(ctx)
	if _, e = holder.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockID); e != nil {
		t.Fatal("guest final lock acquisition failed")
	}
	admitted, e := client.CreateRender(ctx, key, selection)
	if e != nil {
		t.Fatal("guest original render admission failed")
	}
	deadline := time.Now().Add(80 * time.Second)
	for {
		var waiting bool
		if e = p.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND NOT granted AND classid=0 AND objid=$1::oid AND objsubid=1)`, lockID).Scan(&waiting); e != nil {
			t.Fatal("guest final wait observation failed")
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("actual guest render never reached final publication wait")
		}
		time.Sleep(10 * time.Millisecond)
	}
	var artifact string
	if p.QueryRow(ctx, `SELECT artifact_id FROM generation_publication_candidates WHERE job_id=$1 ORDER BY created_at DESC LIMIT 1`, admitted.Job.JobID).Scan(&artifact) != nil || artifact == "" {
		t.Fatal("guest final candidate missing")
	}
	resultURL := ownerStorage + "/v1/artifacts/" + artifact + "/content"
	status, _ := ownerRequest(t, guestHTTP(nil), "GET", resultURL, manager.Token, "", "", nil)
	if status != 404 {
		t.Fatal("pending guest candidate disclosed")
	}
	var fresh access.GrantResult
	if mode == "rotate" {
		fresh = guestRotate(t, manager, current)
	} else {
		guestRevoke(t, manager, current)
	}
	guestSessionStatus(t, guestJar(t, credential), 401)
	if e = holder.Commit(ctx); e != nil {
		t.Fatal("guest final wait release failed")
	}
	job := ownerJob(t, managerClient, admitted.Job.JobID)
	if job.Status != generation.Failed || job.FailureCode != "access_denied" || job.Artifact != nil {
		t.Fatal("old guest original work published after grant invalidation")
	}
	status, _ = ownerRequest(t, guestHTTP(nil), "GET", resultURL, manager.Token, "", "", nil)
	if status != 404 {
		t.Fatal("invalidated guest candidate exposed")
	}
	if mode == "rotate" {
		_, freshCredential, _ := guestExchange(t, fresh.Secret)
		freshClient, _ := generation.NewCookieClient(ownerGeneration, task08Cookie, freshCredential, ownerGeneration, true, guestHTTP(nil))
		replay, e := freshClient.CreateRender(ctx, key, selection)
		if e != nil || replay.Job.JobID != job.JobID || replay.Job.Status != generation.Failed {
			t.Fatal("fresh guest session rescued old work or duplicated receipt")
		}
		task06OriginalBotAuthority(t, p, job.JobID, resolved.Identity, []string{current.Secret, credential, fresh.Secret, freshCredential})
		guestRevoke(t, manager, fresh)
	} else {
		task06OriginalBotAuthority(t, p, job.JobID, resolved.Identity, []string{current.Secret, credential})
	}
	var originalVersion uint64
	accessPool := ownerPool(t, "E06_ACCESS_DATABASE_URL")
	if accessPool.QueryRow(ctx, `SELECT grant_version FROM access_guest_sessions WHERE session_id::text=$1`, resolved.Identity.Authority.ID).Scan(&originalVersion) != nil || originalVersion != current.Grant.Version {
		t.Fatal("original guest session migrated grant version")
	}
	task06RevokedBotCleanup(t, p, job.JobID)
	t.Log("Actual final publication wait, nodeB grant invalidation, original exact guest grant/session version, hidden candidate rollback and service-owned cleanup passed")
}
