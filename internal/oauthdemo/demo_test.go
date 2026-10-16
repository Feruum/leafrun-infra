package oauthdemo

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBrowserBindingOneUseAndPrivateExchange(t *testing.T) {
	exchanges := 0
	var exchangedVerifier string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exchanges++
		r.ParseForm()
		exchangedVerifier = r.Form.Get("code_verifier")
		if r.Method != "POST" || r.Form.Get("code") != "private-code" || r.Form.Get("code_verifier") == "" || r.Form.Get("redirect_uri") != "http://localhost:58200/oauth/callback" {
			t.Error("exchange binding invalid")
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "private-access", "refresh_token": "private-refresh", "token_type": "Bearer", "expires_in": 300, "scope": "leafrun.generate offline_access"})
	}))
	defer provider.Close()
	var accepted Grant
	d, err := New(Config{PublicURL: "http://127.0.0.1:54444", TokenURL: provider.URL, CallbackURL: "http://localhost:58200/oauth/callback", ClientID: "leafrun-e06-demo", Audience: "leafrun-api", Scopes: []string{"leafrun.generate", "offline_access"}, OnGrant: func(g Grant) { accepted = g }})
	if err != nil {
		t.Fatal(err)
	}
	home := httptest.NewRecorder()
	d.ServeHTTP(home, httptest.NewRequest("GET", "http://localhost:58200/", nil))
	cookies := home.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("browser cookie missing")
	}
	csrf := hiddenValue(home.Body.String(), "csrf_token")
	if csrf == "" {
		t.Fatal("start csrf missing")
	}
	start := httptest.NewRequest("POST", "http://localhost:58200/oauth/start", strings.NewReader(url.Values{"csrf_token": {csrf}, "scope": {"leafrun.generate", "offline_access"}}.Encode()))
	start.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	start.Header.Set("Origin", "http://localhost:58200")
	start.AddCookie(cookies[0])
	started := httptest.NewRecorder()
	d.ServeHTTP(started, start)
	u, err := url.Parse(started.Header().Get("Location"))
	if err != nil || u.Query().Get("state") == "" {
		t.Fatal("authorize redirect missing")
	}
	state := u.Query().Get("state")
	if u.Query().Get("code_challenge_method") != "S256" {
		t.Fatal("S256 missing")
	}
	wrong := httptest.NewRecorder()
	d.ServeHTTP(wrong, httptest.NewRequest("GET", "http://localhost:58200/oauth/callback?code=private-code&state="+state, nil))
	if wrong.Code != 400 || exchanges != 0 {
		t.Fatal("unbound browser exchanged")
	}
	cb := httptest.NewRequest("GET", "http://localhost:58200/oauth/callback?code=private-code&scope=leafrun.generate%20offline_access&state="+state, nil)
	cb.AddCookie(cookies[0])
	good := httptest.NewRecorder()
	d.ServeHTTP(good, cb)
	if good.Code != 303 || good.Header().Get("Location") != "/result" || exchanges != 1 || accepted.AccessToken != "private-access" || accepted.RefreshToken != "private-refresh" {
		t.Fatal("valid callback did not complete privately")
	}
	replay := httptest.NewRecorder()
	d.ServeHTTP(replay, cb)
	if replay.Code != 400 || exchanges != 1 {
		t.Fatal("callback replay exchanged")
	}
	result := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://localhost:58200/result", nil)
	req.AddCookie(cookies[0])
	d.ServeHTTP(result, req)
	for _, secret := range []string{"private-code", "private-access", "private-refresh"} {
		if strings.Contains(result.Body.String(), secret) || strings.Contains(good.Body.String(), secret) {
			t.Fatal("private material rendered")
		}
	}
	if result.Header().Get("Referrer-Policy") != "no-referrer" || result.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("browser headers missing")
	}
	hash := sha256.Sum256([]byte(exchangedVerifier))
	if base64.RawURLEncoding.EncodeToString(hash[:]) != u.Query().Get("code_challenge") {
		t.Fatal("verifier does not match S256")
	}
}

func TestRejectStartCSRFAndCallbackAmbiguity(t *testing.T) {
	d, err := New(Config{PublicURL: "http://127.0.0.1:54444", TokenURL: "http://hydra:4444", CallbackURL: "http://localhost:58200/oauth/callback", ClientID: "leafrun-e06-demo", Audience: "leafrun-api", Scopes: []string{"leafrun.generate"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ method, path, body string }{{"POST", "/oauth/start", "csrf_token=bad&scope=leafrun.generate"}, {"GET", "/oauth/callback?code=a&state=b&state=c", ""}, {"GET", "/oauth/callback?error=denied&state=b", ""}, {"GET", "/oauth/callback?code=a&state=b&redirect_uri=http://evil.test", ""}} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(tc.method, "http://localhost:58200"+tc.path, strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		d.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("unsafe input allowed: %s", tc.path)
		}
	}
}
func TestRejectUnsafeConfiguration(t *testing.T) {
	for _, callback := range []string{"http://127.0.0.1:58200/oauth/callback", "http://localhost:58201/oauth/callback", "http://localhost:58200/oauth/callback?x=1"} {
		if _, err := New(Config{PublicURL: "http://127.0.0.1:54444", TokenURL: "http://hydra:4444", CallbackURL: callback, ClientID: "leafrun-e06-demo", Audience: "leafrun-api", Scopes: []string{"leafrun.generate"}}); err == nil {
			t.Fatal("nonexact callback allowed")
		}
	}
}
func TestExpiredStateAndProviderRedirectDeny(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "http://evil.test", 302) }))
	defer provider.Close()
	d, _ := New(Config{PublicURL: "http://127.0.0.1:54444", TokenURL: provider.URL, CallbackURL: "http://localhost:58200/oauth/callback", ClientID: "leafrun-e06-demo", Audience: "leafrun-api", Scopes: []string{"leafrun.generate"}})
	d.states["expired"] = pending{Browser: strings.Repeat("b", 43), Verifier: strings.Repeat("v", 43), Expires: time.Now().Add(-time.Second), Scopes: []string{"leafrun.generate"}}
	r := httptest.NewRequest("GET", "http://localhost:58200/oauth/callback?code=private-code&state=expired", nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: strings.Repeat("b", 43)})
	w := httptest.NewRecorder()
	d.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal("expired callback allowed")
	}
	d.states["valid"] = pending{Browser: strings.Repeat("b", 43), Verifier: strings.Repeat("v", 43), Expires: time.Now().Add(time.Minute), Scopes: []string{"leafrun.generate"}}
	r = httptest.NewRequest("GET", "http://localhost:58200/oauth/callback?code=private-code&state=valid", nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: strings.Repeat("b", 43)})
	w = httptest.NewRecorder()
	d.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal("provider redirect allowed")
	}
}
func hiddenValue(body, name string) string {
	marker := `name="` + name + `" value="`
	i := strings.Index(body, marker)
	if i < 0 {
		return ""
	}
	s := body[i+len(marker):]
	return strings.SplitN(s, `"`, 2)[0]
}

var _ = io.Discard

func TestGrantedScopeSubsetAndStrictTokenJSON(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"proper subset", `{"access_token":"private-access","token_type":"Bearer","expires_in":300,"scope":"leafrun.generate"}`, 303},
		{"widened", `{"access_token":"private-access","token_type":"Bearer","expires_in":300,"scope":"leafrun.generate leafrun.download"}`, 400},
		{"duplicate scope", `{"access_token":"private-access","token_type":"Bearer","expires_in":300,"scope":"leafrun.generate leafrun.generate"}`, 400},
		{"duplicate key", `{"access_token":"first","access_token":"private-access","token_type":"Bearer","expires_in":300,"scope":"leafrun.generate offline_access"}`, 400},
		{"nested duplicate extra", `{"access_token":"private-access","token_type":"Bearer","expires_in":300,"scope":"leafrun.generate offline_access","extra":{"a":1,"a":2}}`, 400},
		{"trailing", `{"access_token":"private-access","token_type":"Bearer","expires_in":300,"scope":"leafrun.generate"} {}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, tc.body) }))
			defer provider.Close()
			d, _ := New(Config{PublicURL: "http://127.0.0.1:54444", TokenURL: provider.URL, CallbackURL: "http://localhost:58200/oauth/callback", ClientID: "leafrun-e06-demo", Audience: "leafrun-api", Scopes: []string{"leafrun.generate", "offline_access"}})
			id := strings.Repeat("b", 43)
			d.states["valid"] = pending{Browser: id, Verifier: strings.Repeat("v", 43), Expires: time.Now().Add(time.Minute), Scopes: []string{"leafrun.generate", "offline_access"}}
			d.browsers[id] = browser{Expires: time.Now().Add(time.Minute)}
			r := httptest.NewRequest("GET", "http://localhost:58200/oauth/callback?code=private-code&state=valid", nil)
			r.AddCookie(&http.Cookie{Name: cookieName, Value: id})
			w := httptest.NewRecorder()
			d.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("callback status %d expected %d", w.Code, tc.want)
			}
		})
	}
}

func TestCaseFoldTokenJSONRejected(t *testing.T) {
	for _, alias := range []string{"ACCESS_TOKEN", "ſcope"} {
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"`+alias+`":"unexpected","access_token":"private-access","token_type":"Bearer","expires_in":300,"scope":"leafrun.generate"}`)
		}))
		d, _ := New(Config{PublicURL: "http://127.0.0.1:54444", TokenURL: provider.URL, CallbackURL: "http://localhost:58200/oauth/callback", ClientID: "leafrun-e06-demo", Audience: "leafrun-api", Scopes: []string{"leafrun.generate"}})
		id := strings.Repeat("b", 43)
		d.states["valid"] = pending{Browser: id, Verifier: strings.Repeat("v", 43), Expires: time.Now().Add(time.Minute), Scopes: []string{"leafrun.generate"}}
		d.browsers[id] = browser{Expires: time.Now().Add(time.Minute)}
		r := httptest.NewRequest("GET", "http://localhost:58200/oauth/callback?code=private-code&state=valid", nil)
		r.AddCookie(&http.Cookie{Name: cookieName, Value: id})
		w := httptest.NewRecorder()
		d.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Error("case alias token JSON accepted")
		}
		provider.Close()
	}
}

func TestFormResponsePreservesCanonicalOrigin(t *testing.T) {
	d, _ := New(Config{PublicURL: "http://127.0.0.1:54444", TokenURL: "http://hydra:4444", CallbackURL: "http://localhost:58200/oauth/callback", ClientID: "leafrun-e06-demo", Audience: "leafrun-api", Scopes: []string{"leafrun.generate"}})
	w := httptest.NewRecorder()
	d.ServeHTTP(w, httptest.NewRequest("GET", "http://localhost:58200/", nil))
	if w.Header().Get("Referrer-Policy") != "origin" {
		t.Fatal("form policy suppresses actual browser canonical POST Origin")
	}
	denied := httptest.NewRecorder()
	d.ServeHTTP(denied, httptest.NewRequest("GET", "http://localhost:58200/oauth/callback?code=synthetic&state=synthetic", nil))
	if denied.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("callback policy leaked transient callback URL")
	}
}

func TestTrustedBrowserFormRedirectOrigins(t *testing.T) {
	d, _ := New(Config{PublicURL: "http://127.0.0.1:54444", TokenURL: "http://hydra:4444", CallbackURL: "http://localhost:58200/oauth/callback", ClientID: "leafrun-e06-demo", Audience: "leafrun-api", Scopes: []string{"leafrun.generate"}})
	w := httptest.NewRecorder()
	d.ServeHTTP(w, httptest.NewRequest("GET", "http://localhost:58200/", nil))
	csp := w.Header().Get("Content-Security-Policy")
	for _, origin := range []string{"http://127.0.0.1:54444", "http://127.0.0.1:58197", "http://127.0.0.1:54433"} {
		if !strings.Contains(csp, origin) {
			t.Fatal("fixed public redirect-chain origin absent")
		}
	}
	if strings.Contains(csp, "hydra:4445") || strings.Contains(csp, "*") || strings.Contains(csp, "connect-src") {
		t.Fatal("CSP widened beyond fixed public form navigation")
	}
}

func TestCallbackScopeMetadataBounded(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  int
	}{
		{"", 303}, {"&scope=leafrun.generate", 303},
		{"&scope=", 400}, {"&scope=leafrun.generate&scope=leafrun.generate", 400},
		{"&scope=leafrun.generate%20leafrun.generate", 400},
		{"&scope=leafrun.generate%20leafrun.status", 400},
		{"&scope=unknown", 400}, {"&iss=http://127.0.0.1:54444/", 400},
		{"&redirect_uri=http://evil.test", 400},
	} {
		t.Run(tc.query, func(t *testing.T) {
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.WriteString(w, `{"access_token":"private-access","token_type":"Bearer","expires_in":300,"scope":"leafrun.generate"}`)
			}))
			defer provider.Close()
			d, _ := New(Config{PublicURL: "http://127.0.0.1:54444", TokenURL: provider.URL, CallbackURL: "http://localhost:58200/oauth/callback", ClientID: "leafrun-e06-demo", Audience: "leafrun-api", Scopes: []string{"leafrun.generate", "offline_access"}})
			id := strings.Repeat("b", 43)
			d.states["valid"] = pending{Browser: id, Verifier: strings.Repeat("v", 43), Expires: time.Now().Add(time.Minute), Scopes: []string{"leafrun.generate", "offline_access"}}
			d.browsers[id] = browser{Expires: time.Now().Add(time.Minute)}
			r := httptest.NewRequest("GET", "http://localhost:58200/oauth/callback?code=private-code&state=valid"+tc.query, nil)
			r.AddCookie(&http.Cookie{Name: cookieName, Value: id})
			w := httptest.NewRecorder()
			d.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("callback metadata status %d expected %d", w.Code, tc.want)
			}
		})
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"access_token":"private-access","token_type":"Bearer","expires_in":300,"scope":"leafrun.generate"}`)
	}))
	defer provider.Close()
	d, _ := New(Config{PublicURL: "http://127.0.0.1:54444", TokenURL: provider.URL, CallbackURL: "http://localhost:58200/oauth/callback", ClientID: "leafrun-e06-demo", Audience: "leafrun-api", Scopes: []string{"leafrun.generate", "offline_access"}})
	id := strings.Repeat("b", 43)
	d.states["valid"] = pending{Browser: id, Verifier: strings.Repeat("v", 43), Expires: time.Now().Add(time.Minute), Scopes: []string{"leafrun.generate", "offline_access"}}
	d.browsers[id] = browser{Expires: time.Now().Add(time.Minute)}
	r := httptest.NewRequest("GET", "http://localhost:58200/oauth/callback?code=private-code&state=valid&scope=leafrun.generate%20offline_access", nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: id})
	w := httptest.NewRecorder()
	d.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal("callback and token scope disagreement accepted")
	}
}
