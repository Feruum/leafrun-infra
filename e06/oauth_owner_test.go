package e06

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	access "github.com/leafrun/leafrun-contracts/access/v1"
	generation "github.com/leafrun/leafrun-contracts/generation/v1"
	identity "github.com/leafrun/leafrun-contracts/identity/v1"
	"github.com/leafrun/leafrun-infra/internal/jsonbound"
	"github.com/leafrun/leafrun-infra/internal/oauthdemo"
	"golang.org/x/net/html"
)

type oauthIntro struct {
	Active  bool           `json:"active"`
	Use     string         `json:"token_use"`
	Subject string         `json:"sub"`
	Client  string         `json:"client_id"`
	Scope   string         `json:"scope"`
	Ext     map[string]any `json:"ext"`
	Exp     int64          `json:"exp"`
}

func oauthIntrospect(t *testing.T, token string) oauthIntro {
	t.Helper()
	status, data := ownerRequest(t, ownerHTTP(nil), "POST", os.Getenv("E06_HYDRA_ADMIN_URL")+"/admin/oauth2/introspect", "", "", "application/x-www-form-urlencoded", []byte(url.Values{"token": {token}, "token_type_hint": {"access_token"}}.Encode()))
	var out oauthIntro
	if status != 200 || jsonbound.Decode(data, &out) != nil {
		t.Fatal("private Hydra introspection failed")
	}
	return out
}
func oauthRefresh(t *testing.T, refresh string) (oauthdemo.Grant, bool) {
	t.Helper()
	status, data := ownerRequest(t, ownerHTTP(nil), "POST", os.Getenv("E06_HYDRA_INTERNAL_URL")+"/oauth2/token", "", "", "application/x-www-form-urlencoded", []byte(url.Values{"grant_type": {"refresh_token"}, "client_id": {"leafrun-e06-demo"}, "refresh_token": {refresh}}.Encode()))
	if status != 200 {
		return oauthdemo.Grant{}, false
	}
	var out struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Scope   string `json:"scope"`
		Expires int    `json:"expires_in"`
	}
	if jsonbound.Decode(data, &out) != nil || out.Access == "" || out.Refresh == "" || out.Expires <= 0 {
		t.Fatal("private refresh result invalid")
	}
	return oauthdemo.Grant{AccessToken: out.Access, RefreshToken: out.Refresh, Scopes: strings.Fields(out.Scope), Expires: time.Now().Add(time.Duration(out.Expires) * time.Second)}, true
}
func oauthRevoke(t *testing.T, token string) {
	t.Helper()
	status, _ := ownerRequest(t, ownerHTTP(nil), "POST", os.Getenv("E06_HYDRA_INTERNAL_URL")+"/oauth2/revoke", "", "", "application/x-www-form-urlencoded", []byte(url.Values{"client_id": {"leafrun-e06-demo"}, "token": {token}}.Encode()))
	if status != 200 {
		t.Fatal("private token revoke failed")
	}
}
func oauthDenied(t *testing.T, token string) {
	t.Helper()
	for _, node := range []string{ownerNodeA, ownerNodeB} {
		if _, err := ownerAccess(t, node).Session(context.Background(), access.ResolveRequest{Bearer: token}); !errors.Is(err, access.ErrUnauthenticated) {
			t.Fatal("invalidated OAuth token resolved on A/B")
		}
	}
}
func oauthStage(t *testing.T, stage string) {
	t.Helper()
	if os.WriteFile("/evidence/oauth-stage.txt", []byte(stage), 0600) != nil {
		t.Fatal("OAuth stage receipt failed")
	}
}
func oauthGrant(t *testing.T, ch <-chan oauthdemo.Grant) oauthdemo.Grant {
	t.Helper()
	select {
	case g := <-ch:
		return g
	case <-time.After(4 * time.Minute):
		t.Fatal("actual browser OAuth callback did not complete")
		return oauthdemo.Grant{}
	}
}

type oauthExchangeTransport struct {
	base http.RoundTripper
	mode *atomic.Int32
}

func (r oauthExchangeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == "POST" && req.URL.Path == "/oauth2/token" && (r.mode.Load() == 1 || r.mode.Load() == 2) {
		data, err := io.ReadAll(io.LimitReader(req.Body, 16384))
		req.Body.Close()
		if err != nil {
			return nil, errors.New("fixture exchange invalid")
		}
		form, err := url.ParseQuery(string(data))
		if err != nil {
			return nil, errors.New("fixture exchange invalid")
		}
		if r.mode.Load() == 1 {
			form.Set("code_verifier", strings.Repeat("x", 43))
		} else {
			form.Del("code_verifier")
		}
		req = req.Clone(req.Context())
		req.Body = io.NopCloser(strings.NewReader(form.Encode()))
		req.ContentLength = int64(len(form.Encode()))
	}
	if req.Method == "POST" && req.URL.Path == "/oauth2/token" && r.mode.Load() == 3 {
		req = req.Clone(req.Context())
		copyURL := *req.URL
		copyURL.Host = "hydra-expiry:4444"
		req.URL = &copyURL
		req.Host = ""
	}
	return r.base.RoundTrip(req)
}

// The first grant MUST be completed by independent Chromium. Further isolated
// transactions use the same real browser-form protocol for provider negatives.
func TestOAuthActualBrowserOwnerLifecycle(t *testing.T) {
	if os.Getenv("E06_OAUTH_BROWSER_OWNER") != "true" {
		t.Skip("actual Chromium owner harness not requested")
	}
	ownerEnabled(t)
	ctx := context.Background()
	data, err := os.ReadFile("/evidence/oauth-browser-fixture.json")
	var fixture browserFixture
	if err != nil || json.Unmarshal(data, &fixture) != nil || fixture.Human.ID == "" {
		t.Fatal("independent OAuth browser fixture unavailable")
	}
	grants := make(chan oauthdemo.Grant, 16)
	var mode atomic.Int32
	c := ownerHTTP(nil)
	c.Transport = oauthExchangeTransport{c.Transport, &mode}
	demo, err := oauthdemo.New(oauthdemo.Config{PublicURL: "http://127.0.0.1:54444", TokenURL: "http://hydra:4444", CallbackURL: "http://localhost:58200/oauth/callback", ClientID: "leafrun-e06-demo", Audience: "leafrun-api", Scopes: []string{"leafrun.generate", "offline_access"}, HTTPClient: c, OnGrant: func(g oauthdemo.Grant) { grants <- g }})
	if err != nil {
		t.Fatal("owned demo configuration failed")
	}
	listener, err := net.Listen("tcp", ":58200")
	if err != nil {
		t.Fatal("owned demo loopback mapping unavailable")
	}
	server := &http.Server{Handler: demo, ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(listener)
	defer server.Close()
	oauthStage(t, "waiting-chromium")
	original := oauthGrant(t, grants)
	oauthStage(t, "browser-complete")
	intro := oauthIntrospect(t, original.AccessToken)
	sessionID, ok := intro.Ext["kratos_session_id"].(string)
	if !intro.Active || intro.Use != "access_token" || intro.Subject != fixture.Human.ID || !ok || sessionID == "" {
		t.Fatal("actual browser token binding invalid")
	}
	a, b := ownerAccess(t, ownerNodeA), ownerAccess(t, ownerNodeB)
	resolved, err := a.Session(ctx, access.ResolveRequest{Bearer: original.AccessToken})
	if err != nil || resolved.Identity.Authority.Kind != identity.KindOAuthToken || resolved.Identity.Principal.Subject != fixture.Human.ID {
		t.Fatal("actual browser OAuth authority failed")
	}
	other, err := b.Session(ctx, access.ResolveRequest{Bearer: original.AccessToken})
	if err != nil || other.Identity != resolved.Identity || !other.ExpiresAt.Equal(*resolved.ExpiresAt) {
		t.Fatal("A/B exact token binding diverged")
	}
	if _, err = a.CreateOrganization(ctx, access.ResolveRequest{Bearer: original.AccessToken}, access.NameRequest{Name: "OAuth ceiling escape"}); !errors.Is(err, access.ErrDenied) {
		t.Fatal("generate-only human OAuth acquired management")
	}
	manager := ownerLogin(t, fixture.Human)
	defer ownerLogout(t, manager)
	oauthManagementCeiling(t, fixture, manager, original.AccessToken)
	for _, op := range []string{"source.read", "resource.read", "artifact.read", "download", "generation.read"} {
		for _, node := range []string{ownerNodeA, ownerNodeB} {
			checker, err := access.NewClient(node, os.Getenv("E06_PROJECTS_SERVICE_TOKEN"), ownerHTTP(nil))
			if err != nil {
				t.Fatal("OAuth operation checker fixture invalid")
			}
			allowed, err := checker.Check(ctx, access.CheckRequest{Principal: resolved.Identity.Principal, Authority: &resolved.Identity.Authority, Scope: access.PermissionScope{WorkspaceID: fixture.Scope.WorkspaceID, ProjectID: fixture.Scope.ProjectID}, Operation: op})
			if allowed || !errors.Is(err, access.ErrDenied) {
				t.Fatal("OAuth operation ceiling did not return an explicit denial")
			}
		}
	}
	managerGeneration, _ := generation.NewClient(ownerGeneration, manager.Token, ownerHTTP(nil))
	client, _ := generation.NewClient(ownerGeneration, original.AccessToken, ownerHTTP(nil))
	selection := generation.RenderSelectionRequest{Scope: fixture.Scope, ReleaseID: fixture.ReleaseID, Parameters: json.RawMessage(`{}`)}
	admitted, err := client.CreateRender(ctx, "oauth-success-"+ownerNonce(t), selection)
	if err != nil {
		t.Fatal("OAuth real render admission failed")
	}
	success := ownerJob(t, managerGeneration, admitted.Job.JobID)
	if success.Status != generation.Succeeded || success.Artifact == nil {
		t.Fatal("OAuth real PDF render failed")
	}
	resultURL := ownerStorage + "/v1/artifacts/" + success.Artifact.ArtifactID + "/content"
	status, _ := ownerRequest(t, ownerHTTP(nil), "GET", resultURL, original.AccessToken, "", "", nil)
	if status != 404 {
		t.Fatal("generate-only OAuth downloaded PDF")
	}
	status, pdf := ownerRequest(t, ownerHTTP(nil), "GET", resultURL, manager.Token, "", "", nil)
	if status != 200 || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("current native owner could not read historical OAuth PDF")
	}
	accessPool, generationPool := ownerPool(t, "E06_ACCESS_DATABASE_URL"), ownerPool(t, "E06_GENERATION_DATABASE_URL")
	var ciphertext, binding []byte
	var keyID string
	var expiry time.Time
	if accessPool.QueryRow(ctx, `SELECT secret_ciphertext,token_binding,key_id,expires_at FROM access_oauth_authorities WHERE authority_id=$1`, resolved.Identity.Authority.ID).Scan(&ciphertext, &binding, &keyID, &expiry) != nil || len(ciphertext) == 0 || keyID != os.Getenv("E06_ACCESS_OAUTH_KEY_ID") || bytes.Contains(ciphertext, []byte(original.AccessToken)) || bytes.Contains(binding, []byte(original.AccessToken)) {
		t.Fatal("OAuth exact encrypted persistence invalid")
	}
	task06OriginalBotAuthority(t, generationPool, success.JobID, resolved.Identity, []string{original.AccessToken, original.RefreshToken})
	// The coordinator restarts Access and worker processes while this original
	// token remains in this test process only. No fixture credential export.
	os.Remove("/evidence/oauth-restart-complete")
	oauthStage(t, "restart-ready")
	deadline := time.Now().Add(90 * time.Second)
	for {
		if _, err := os.Stat("/evidence/oauth-restart-complete"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("coordinator restart receipt missing")
		}
		time.Sleep(200 * time.Millisecond)
	}
	after, err := b.Session(ctx, access.ResolveRequest{Bearer: original.AccessToken})
	if err != nil || after.Identity != resolved.Identity || !after.ExpiresAt.Equal(*resolved.ExpiresAt) {
		t.Fatal("restart rebound exact original authority")
	}
	oauthStage(t, "restart-verified")
	// Queue under original A, refresh into B at a real final-publication wait.
	successor, failedJobID := oauthPausedRefresh(t, generationPool, original, resolved.Identity, selection, managerGeneration)
	fresh, err := b.Session(ctx, access.ResolveRequest{Bearer: successor.AccessToken})
	if err != nil || fresh.Identity.Authority.ID == resolved.Identity.Authority.ID {
		t.Fatal("refresh did not create distinct immutable authority")
	}
	var oldExpiry time.Time
	if accessPool.QueryRow(ctx, `SELECT expires_at FROM access_oauth_authorities WHERE authority_id=$1`, resolved.Identity.Authority.ID).Scan(&oldExpiry) != nil || !oldExpiry.Equal(expiry) {
		t.Fatal("refresh extended original authority expiry")
	}
	oauthDenied(t, original.AccessToken)
	if !oauthIntrospect(t, successor.AccessToken).Active {
		t.Fatal("refresh successor inactive")
	}
	oauthRevoke(t, original.AccessToken)
	if !oauthIntrospect(t, successor.AccessToken).Active {
		t.Fatal("absent original AT revoke unexpectedly invalidated successor")
	}
	// A distinct same-human/client consent chain survives chain-wide revocation.
	oauthRunBrowserForm(t, fixture.Human, "state-replay")
	independent := oauthGrant(t, grants)
	oauthRevoke(t, successor.AccessToken)
	if oauthIntrospect(t, successor.AccessToken).Active || oauthIntrospect(t, successor.RefreshToken).Active || !oauthIntrospect(t, independent.AccessToken).Active {
		t.Fatal("active successor revoke did not isolate consent chain")
	}
	oauthDenied(t, successor.AccessToken)
	oauthRunBrowserForm(t, fixture.Human, "normal")
	reuseOriginal := oauthGrant(t, grants)
	reuseSuccessor, ok := oauthRefresh(t, reuseOriginal.RefreshToken)
	if !ok {
		t.Fatal("reuse fixture refresh failed")
	}
	if _, ok = oauthRefresh(t, reuseOriginal.RefreshToken); ok || oauthIntrospect(t, reuseSuccessor.AccessToken).Active || !oauthIntrospect(t, independent.AccessToken).Active {
		t.Fatal("strict old RT reuse detection or independent chain isolation failed")
	}
	oauthRunBrowserForm(t, fixture.Human, "normal")
	consentChain := oauthGrant(t, grants)
	ci := oauthIntrospect(t, consentChain.AccessToken)
	bindingID, _ := ci.Ext["consent_binding_id"].(string)
	var providerID string
	if accessPool.QueryRow(ctx, `SELECT provider_request_id::text FROM access_oauth_consents WHERE consent_binding_id=$1`, bindingID).Scan(&providerID) != nil || providerID == bindingID {
		t.Fatal("provider consent UUID mapping missing")
	}
	status, _ = ownerRequest(t, ownerHTTP(nil), "DELETE", os.Getenv("E06_HYDRA_ADMIN_URL")+"/admin/oauth2/auth/sessions/consent?"+url.Values{"consent_request_id": {providerID}}.Encode(), "", "", "", nil)
	if status != 204 || oauthIntrospect(t, consentChain.AccessToken).Active || oauthIntrospect(t, consentChain.RefreshToken).Active || !oauthIntrospect(t, independent.AccessToken).Active {
		t.Fatal("provider consent chain revocation isolation failed")
	}
	oauthStage(t, "chromium-downscope-ready")
	downscoped := oauthGrant(t, grants)
	if len(downscoped.Scopes) != 1 || downscoped.Scopes[0] != "leafrun.generate" || downscoped.RefreshToken != "" {
		t.Fatal("actual explicit proper consent downscope failed")
	}
	oauthChromiumAck(t, "chromium-decline-ready")
	select {
	case <-grants:
		t.Fatal("declined consent produced grant")
	default:
	}
	oauthRunBrowserForm(t, fixture.Human, "session-swap")
	sessionSwapGrant := oauthGrant(t, grants)
	// Wrong and absent verifier use fresh actual provider codes; no grant observed.
	for _, m := range []int32{1, 2} {
		mode.Store(m)
		stage := "chromium-wrong-pkce-ready"
		if m == 2 {
			stage = "chromium-missing-pkce-ready"
		}
		oauthChromiumAck(t, stage)
		select {
		case <-grants:
			t.Fatal("invalid PKCE exchanged")
		default:
		}
	}
	mode.Store(0)
	mode.Store(3)
	oauthRunBrowserForm(t, fixture.Human, "normal")
	short := oauthGrant(t, grants)
	mode.Store(0)
	shortResolved, err := a.Session(ctx, access.ResolveRequest{Bearer: short.AccessToken})
	if err != nil || shortResolved.ExpiresAt == nil || time.Until(*shortResolved.ExpiresAt) > 6*time.Second {
		t.Fatal("short provider TTL fixture invalid")
	}
	time.Sleep(time.Until(*shortResolved.ExpiresAt) + 300*time.Millisecond)
	oauthDenied(t, short.AccessToken)
	if oauthIntrospect(t, short.AccessToken).Active {
		t.Fatal("expired exact access token remained active")
	}
	var shortExpiry time.Time
	if accessPool.QueryRow(ctx, `SELECT expires_at FROM access_oauth_authorities WHERE authority_id=$1`, shortResolved.Identity.Authority.ID).Scan(&shortExpiry) != nil || !shortExpiry.Equal(*shortResolved.ExpiresAt) {
		t.Fatal("expiry changed immutable original authority")
	}
	// Logout revokes the exact bound Kratos session, even while Hydra remains active.
	independentIntro := oauthIntrospect(t, independent.AccessToken)
	independentSession, _ := independentIntro.Ext["kratos_session_id"].(string)
	if independentSession == "" {
		t.Fatal("independent session binding missing")
	}
	ownerLogout(t, ownerSession{ID: independentSession})
	if !oauthIntrospect(t, independent.AccessToken).Active {
		t.Fatal("logout fixture no longer proves independent Kratos gate")
	}
	oauthDenied(t, independent.AccessToken)
	ownerLogout(t, ownerSession{ID: sessionID})
	status, pdf = ownerRequest(t, ownerHTTP(nil), "GET", resultURL, manager.Token, "", "", nil)
	if status != 200 || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		t.Fatal("historical committed PDF lost after OAuth revocation")
	}
	historical := ownerJob(t, managerGeneration, success.JobID)
	if historical.Artifact == nil || !historical.Artifact.ExpiresAt.Equal(success.Artifact.ExpiresAt) {
		t.Fatal("OAuth lifecycle changed historical PDF TTL")
	}
	oauthSecretAbsence(t, generationPool, []string{success.JobID, failedJobID}, original, successor, independent, reuseOriginal, reuseSuccessor, consentChain, downscoped, sessionSwapGrant, short)
	oauthStage(t, "complete")
	t.Log("Actual Chromium code+S256+consent, A/B immutable encrypted authority, real PDF, final refresh denial, absent-old-AT no-op, active-chain revoke, old-RT reuse, consent UUID isolation, wrong/missing PKCE, state replay, restart and exact Kratos logout passed without event transport")
}

func oauthRunBrowserForm(t *testing.T, h ownerHuman, mode string) {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	c := ownerHTTP(jar)
	target := "http://localhost:58200/"
	method := "GET"
	form := url.Values{}
	origin := ""
	var acceptedForms []struct {
		target string
		form   url.Values
	}
	var callback string
	for step := 0; step < 30; step++ {
		var body io.Reader
		if method == "POST" {
			body = strings.NewReader(form.Encode())
		}
		req, _ := http.NewRequest(method, target, body)
		req.Header.Set("Accept", "text/html")
		if method == "POST" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", origin)
		}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal("actual browser-form request failed")
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 65536))
		resp.Body.Close()
		if err != nil {
			t.Fatal("browser-form response failed")
		}
		u, _ := url.Parse(target)
		if u.Path == "/oauth/callback" {
			callback = target
			if mode == "exchange-deny" || mode == "decline" {
				if resp.StatusCode != 400 {
					t.Fatal("invalid actual PKCE callback allowed")
				}
				return
			}
			if resp.StatusCode != 303 || resp.Header.Get("Location") != "/result" {
				t.Fatal("actual browser-form callback failed")
			}
			for _, accepted := range acceptedForms {
				post, _ := http.NewRequest("POST", accepted.target, strings.NewReader(accepted.form.Encode()))
				post.Header.Set("Origin", ownerNodeA)
				post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				replayed, err := c.Do(post)
				if err != nil {
					t.Fatal("Access form replay probe failed")
				}
				replayed.Body.Close()
				if replayed.StatusCode != 401 {
					t.Fatal("Access accepted login/consent form replay")
				}
			}
			if mode == "state-replay" {
				replay, _ := http.NewRequest("GET", callback, nil)
				replayed, err := c.Do(replay)
				if err != nil {
					t.Fatal("callback replay request failed")
				}
				replayed.Body.Close()
				if replayed.StatusCode != 400 {
					t.Fatal("actual callback replay accepted")
				}
			}
			return
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			next, err := u.Parse(resp.Header.Get("Location"))
			if err != nil {
				t.Fatal("browser continuation malformed")
			}
			if next.Path == "/oauth/callback" && mode == "state-replay" {
				swapped := *next
				q := swapped.Query()
				q.Set("state", ownerNonce(t))
				swapped.RawQuery = q.Encode()
				bad, _ := http.NewRequest("GET", swapped.String(), nil)
				badResp, err := c.Do(bad)
				if err != nil {
					t.Fatal("swapped state request failed")
				}
				badResp.Body.Close()
				if badResp.StatusCode != 400 {
					t.Fatal("swapped actual callback state accepted")
				}
			}
			target = next.String()
			method = "GET"
			continue
		}
		if resp.StatusCode != 200 {
			t.Fatal("actual browser-form transition denied")
		}
		action, fields := oauthHTMLForm(t, data)
		next, err := u.Parse(action)
		if err != nil {
			t.Fatal("browser form action invalid")
		}
		if next.Host != "localhost:58200" && next.Host != "127.0.0.1:58197" && next.Host != "127.0.0.1:54433" {
			t.Fatal("browser form left fixed origins")
		}
		if next.Path == "/oauth/start" {
			fields["scope"] = []string{"leafrun.generate", "offline_access"}
		}
		if fields.Get("identifier") != "" || next.Path == "/self-service/login" {
			fields.Set("identifier", h.Email)
			fields.Set("password", h.Password)
			fields.Set("method", "password")
		}
		if next.Path == "/oauth/login" || next.Path == "/oauth/consent" {
			fields.Set("action", "accept")
			if next.Path == "/oauth/consent" && mode == "downscope" {
				fields["scope"] = []string{"leafrun.generate"}
			}
			if next.Path == "/oauth/consent" && mode == "decline" {
				fields.Set("action", "deny")
			}
			if next.Path == "/oauth/login" && mode == "session-swap" {
				oauthSessionSwap(t, c, next.String(), fields, h)
			}
			badFields := url.Values{}
			for k, v := range fields {
				badFields[k] = append([]string(nil), v...)
			}
			badFields.Set("csrf", strings.Repeat("A", 43))
			bad, _ := http.NewRequest("POST", next.String(), strings.NewReader(badFields.Encode()))
			bad.Header.Set("Origin", "http://127.0.0.1:58197")
			bad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			badResp, err := c.Do(bad)
			if err != nil {
				t.Fatal("Access CSRF negative request failed")
			}
			badResp.Body.Close()
			if badResp.StatusCode != 401 {
				t.Fatal("actual Access POST CSRF accepted")
			}
			acceptedForms = append(acceptedForms, struct {
				target string
				form   url.Values
			}{next.String(), fields})
		}
		target = next.String()
		method = "POST"
		form = fields
		origin = next.Scheme + "://" + next.Host
	}
	t.Fatal("browser-form transition limit exceeded")
}
func oauthHTMLForm(t *testing.T, data []byte) (string, url.Values) {
	t.Helper()
	root, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		t.Fatal("escaped browser form parse failed")
	}
	var form *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "form" && form == nil {
			form = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if form == nil {
		t.Fatal("browser form absent")
	}
	attrs := func(n *html.Node) map[string]string {
		a := map[string]string{}
		for _, x := range n.Attr {
			a[x.Key] = x.Val
		}
		return a
	}
	action := attrs(form)["action"]
	fields := url.Values{}
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "input" || n.Data == "button") {
			a := attrs(n)
			if a["name"] != "" && a["type"] != "submit" {
				fields.Add(a["name"], a["value"])
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(form)
	return action, fields
}

func oauthPausedRefresh(t *testing.T, p *pgxpool.Pool, original oauthdemo.Grant, want identity.Identity, selection generation.RenderSelectionRequest, manager *generation.Client) (oauthdemo.Grant, string) {
	t.Helper()
	ctx := context.Background()
	nonce := ownerNonce(t)
	function, trigger := "e06_oauth_final_"+nonce, "e06_oauth_trigger_"+nonce
	key := "oauth-final-" + nonce
	lockID := int64(time.Now().UnixNano()&0x3fffffff) + 1
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	sql := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $e06$ BEGIN IF NEW.idempotency_key=%s AND NEW.workspace_id=%s AND NEW.project_id=%s AND NEW.status='succeeded' AND OLD.status<>'succeeded' THEN PERFORM pg_advisory_xact_lock(%d::bigint); END IF; RETURN NEW; END $e06$`, function, quote(key), quote(selection.Scope.WorkspaceID), quote(selection.Scope.ProjectID), lockID)
	if _, err := p.Exec(ctx, sql); err != nil {
		t.Fatal("OAuth final wait fixture failed")
	}
	if _, err := p.Exec(ctx, "CREATE TRIGGER "+trigger+" BEFORE UPDATE ON generation_jobs FOR EACH ROW EXECUTE FUNCTION "+function+"()"); err != nil {
		t.Fatal("OAuth final wait trigger failed")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := p.Exec(cleanup, "DROP TRIGGER IF EXISTS "+trigger+" ON generation_jobs"); err != nil {
			t.Error("OAuth final trigger cleanup failed")
		}
		if _, err := p.Exec(cleanup, "DROP FUNCTION IF EXISTS "+function+"()"); err != nil {
			t.Error("OAuth final function cleanup failed")
		}
	}()
	holder, err := p.Begin(ctx)
	if err != nil {
		t.Fatal("OAuth final lock holder failed")
	}
	defer holder.Rollback(ctx)
	if _, err = holder.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockID); err != nil {
		t.Fatal("OAuth final lock failed")
	}
	client, _ := generation.NewClient(ownerGeneration, original.AccessToken, ownerHTTP(nil))
	admission, err := client.CreateRender(ctx, key, selection)
	if err != nil {
		t.Fatal("OAuth original queued work admission failed")
	}
	deadline := time.Now().Add(80 * time.Second)
	for {
		var waiting bool
		if p.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND NOT granted AND classid=0 AND objid=$1::oid AND objsubid=1)`, lockID).Scan(&waiting) != nil {
			t.Fatal("OAuth final wait observation failed")
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("actual OAuth worker never reached final wait")
		}
		time.Sleep(10 * time.Millisecond)
	}
	successor, ok := oauthRefresh(t, original.RefreshToken)
	if !ok || successor.AccessToken == original.AccessToken || successor.RefreshToken == original.RefreshToken {
		t.Fatal("strict actual refresh did not rotate token pair")
	}
	oauthDenied(t, original.AccessToken)
	if err = holder.Commit(ctx); err != nil {
		t.Fatal("OAuth final wait release failed")
	}
	job := ownerJob(t, manager, admission.Job.JobID)
	if job.Status != generation.Failed || job.FailureCode != "access_denied" || job.Artifact != nil {
		t.Fatal("refreshed-away original authority published queued work")
	}
	task06OriginalBotAuthority(t, p, job.JobID, want, []string{original.AccessToken, original.RefreshToken, successor.AccessToken, successor.RefreshToken})
	fresh, _ := generation.NewClient(ownerGeneration, successor.AccessToken, ownerHTTP(nil))
	replay, err := fresh.CreateRender(ctx, key, selection)
	if err != nil || replay.Job.JobID != job.JobID || replay.Job.Status != generation.Failed {
		t.Fatal("successor authority rescued or duplicated original work")
	}
	return successor, job.JobID
}

func oauthManagementCeiling(t *testing.T, f browserFixture, manager ownerSession, token string) {
	t.Helper()
	ctx := context.Background()
	a := ownerAccess(t, ownerNodeA)
	auth := access.ResolveRequest{Bearer: manager.Token}
	var org string
	if ownerPool(t, "E06_ACCESS_DATABASE_URL").QueryRow(ctx, `SELECT organization_id::text FROM access_workspaces WHERE workspace_id=$1`, f.Scope.WorkspaceID).Scan(&org) != nil {
		t.Fatal("OAuth owned organization binding missing")
	}
	team, err := a.CreateTeam(ctx, auth, org, access.NameRequest{Name: "OAuth ceiling fixture"})
	if err != nil {
		t.Fatal("OAuth management fixture team failed")
	}
	target := ownerIdentity(t)
	invitation, err := a.IssueInvitation(ctx, auth, org, access.InvitationRequest{HumanID: target.ID, Role: "member", ExpiresAt: time.Now().Add(20 * time.Minute)})
	if err != nil {
		t.Fatal("OAuth invitation fixture failed")
	}
	selfInvitation, err := a.IssueInvitation(ctx, auth, org, access.InvitationRequest{HumanID: f.Human.ID, Role: "member", ExpiresAt: time.Now().Add(20 * time.Minute)})
	if err != nil {
		t.Fatal("OAuth invitation recipient fixture failed")
	}
	// Team membership has an existing organization-membership prerequisite.
	// Satisfy it before checking the OAuth operation ceiling.
	if _, err = a.SetOrganizationMembership(ctx, auth, org, access.MembershipRequest{HumanID: target.ID, Role: "member"}); err != nil {
		t.Fatal("OAuth team membership prerequisite fixture failed")
	}
	bot, err := a.CreateBot(ctx, auth, f.Scope.WorkspaceID, f.Scope.ProjectID, access.CreateBotRequest{Name: "OAuth ceiling bot", OperationKey: ownerNonce(t)})
	if err != nil {
		t.Fatal("OAuth bot management fixture failed")
	}
	key, err := a.IssueBotKey(ctx, auth, bot.BotID, access.BotKeyRequest{OperationKey: ownerNonce(t), Operations: []string{"generation.create"}})
	if err != nil {
		t.Fatal("OAuth key management fixture failed")
	}
	type route struct {
		method, path string
		body         any
	}
	orgPath := "/v1/organizations/" + org
	teamPath := "/v1/teams/" + team.TeamID
	workspacePath := "/v1/workspaces/" + f.Scope.WorkspaceID
	projectPath := workspacePath + "/projects/" + f.Scope.ProjectID
	routes := []route{{"POST", "/v1/organizations", map[string]any{"name": "Blocked"}}, {"POST", orgPath + "/workspaces", map[string]any{"name": "Blocked"}}, {"POST", orgPath + "/teams", map[string]any{"name": "Blocked"}}, {"POST", workspacePath + "/projects", map[string]any{"name": "Blocked", "kind": "personal", "ownerId": f.Human.ID}}, {"POST", orgPath + "/memberships", map[string]any{"humanId": target.ID, "role": "member"}}, {"PUT", orgPath + "/memberships/" + target.ID, map[string]any{"humanId": target.ID, "role": "member"}}, {"DELETE", orgPath + "/memberships/" + target.ID, map[string]any{}}, {"POST", teamPath + "/memberships", map[string]any{"humanId": target.ID, "role": "member"}}, {"DELETE", teamPath + "/memberships/" + target.ID, map[string]any{}}, {"POST", orgPath + "/invitations", map[string]any{"humanId": target.ID, "role": "member", "expiresAt": time.Now().Add(20 * time.Minute)}}, {"DELETE", orgPath + "/invitations/" + invitation.Invitation.InvitationID, map[string]any{}}, {"POST", "/v1/invitations/accept", map[string]any{"secret": selfInvitation.Secret}}, {"POST", projectPath + "/bots", map[string]any{"name": "Blocked", "operationKey": ownerNonce(t)}}, {"POST", "/v1/bots/" + bot.BotID + "/keys", map[string]any{"operationKey": ownerNonce(t), "operations": []string{"generation.create"}}}, {"POST", "/v1/bots/" + bot.BotID + "/keys/" + key.Key.KeyID + "/rotate", map[string]any{"operationKey": ownerNonce(t), "expectedVersion": 1, "operations": []string{"generation.create"}}}, {"DELETE", "/v1/bots/" + bot.BotID + "/keys/" + key.Key.KeyID, map[string]any{"operationKey": ownerNonce(t), "expectedVersion": 1}}}
	for _, base := range []string{"/v1/organizations", orgPath + "/workspaces", orgPath + "/teams", orgPath + "/memberships", orgPath + "/invitations", workspacePath + "/projects", teamPath + "/memberships", orgPath, orgPath + "/workspaces/" + f.Scope.WorkspaceID, orgPath + "/teams/" + team.TeamID, projectPath, orgPath + "/invitations/" + invitation.Invitation.InvitationID} {
		routes = append(routes, route{"GET", base, nil})
	}
	for _, base := range []string{orgPath, workspacePath, projectPath} {
		for _, method := range []string{"PUT", "DELETE"} {
			routes = append(routes, route{method, base + "/bindings", map[string]any{"subjectKind": "human", "subjectId": target.ID, "relation": "generate"}})
		}
	}
	for i, route := range routes {
		for _, node := range []string{ownerNodeA, ownerNodeB} {
			status, _ := ownerRequest(t, ownerHTTP(nil), route.method, node+route.path, token, "", "application/json", route.body)
			if status != 403 {
				t.Fatalf("generate-only OAuth management route %d status %d", i, status)
			}
		}
	}
	t.Log("Every Access hierarchy/read/membership/invitation/binding/bot/key management route denied generate-only OAuth on both nodes despite native owner role")
}

func oauthSessionSwap(t *testing.T, c *http.Client, target string, form url.Values, h ownerHuman) {
	t.Helper()
	provider, _ := url.Parse(ownerProvider)
	var original *http.Cookie
	for _, cookie := range c.Jar.Cookies(provider) {
		if cookie.Name == "ory_kratos_session" {
			copy := *cookie
			copy.Path = "/"
			original = &copy
		}
	}
	if original == nil {
		t.Fatal("original browser session cookie absent")
	}
	jar, _ := cookiejar.New(nil)
	other := ownerHTTP(jar)
	next := ownerProvider + "/self-service/login/browser?" + url.Values{"return_to": {ownerNodeA + "/"}}.Encode()
	method := "GET"
	fields := url.Values{}
	for step := 0; step < 12; step++ {
		var body io.Reader
		if method == "POST" {
			body = strings.NewReader(fields.Encode())
		}
		req, _ := http.NewRequest(method, next, body)
		req.Header.Set("Accept", "text/html")
		if method == "POST" {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("Origin", ownerProvider)
		}
		resp, err := other.Do(req)
		if err != nil {
			t.Fatal("second same-human browser session request failed")
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 65536))
		resp.Body.Close()
		if err != nil {
			t.Fatal("second browser session response failed")
		}
		u, _ := url.Parse(next)
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			nextURL, err := u.Parse(resp.Header.Get("Location"))
			if err != nil {
				t.Fatal("second browser continuation invalid")
			}
			next = nextURL.String()
			method = "GET"
			if next == ownerNodeA+"/" {
				break
			}
			continue
		}
		if resp.StatusCode != 200 {
			t.Fatal("second browser session login denied")
		}
		action, values := oauthHTMLForm(t, data)
		nextURL, err := u.Parse(action)
		if err != nil || nextURL.Host != "127.0.0.1:54433" || nextURL.Path != "/self-service/login" {
			t.Fatal("second browser form target invalid")
		}
		values.Set("method", "password")
		values.Set("identifier", h.Email)
		values.Set("password", h.Password)
		fields = values
		next = nextURL.String()
		method = "POST"
	}
	var replacement *http.Cookie
	for _, cookie := range jar.Cookies(provider) {
		if cookie.Name == "ory_kratos_session" {
			copy := *cookie
			copy.Path = "/"
			replacement = &copy
		}
	}
	if replacement == nil || replacement.Value == original.Value {
		t.Fatal("distinct same-human browser session not established")
	}
	c.Jar.SetCookies(provider, []*http.Cookie{replacement})
	defer c.Jar.SetCookies(provider, []*http.Cookie{original})
	req, _ := http.NewRequest("POST", target, strings.NewReader(form.Encode()))
	req.Header.Set("Origin", ownerNodeA)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal("same-human browser session swap request failed")
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatal("OAuth accepted same-human different Kratos session")
	}
}

func oauthChromiumAck(t *testing.T, stage string) {
	t.Helper()
	const path = "/evidence/oauth-browser-step-complete"
	os.Remove(path)
	oauthStage(t, stage)
	deadline := time.Now().Add(3 * time.Minute)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("actual Chromium negative decision receipt missing")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
