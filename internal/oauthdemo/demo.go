// Package oauthdemo implements the fixed, loopback-only E06 public OAuth client.
// Credentials remain in process memory; the browser receives only safe status.
package oauthdemo

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/leafrun/leafrun-infra/internal/jsonbound"
)

const cookieName = "leafrun_e06_oauth_demo"
const callbackURL = "http://localhost:58200/oauth/callback"
const browserOrigin = "http://localhost:58200"

var allowedScopes = map[string]bool{"leafrun.generate": true, "leafrun.status": true, "leafrun.source.read": true, "leafrun.resource.read": true, "leafrun.download": true, "offline_access": true}

type Config struct {
	PublicURL, TokenURL, CallbackURL, ClientID, Audience string
	Scopes                                               []string
	HTTPClient                                           *http.Client
	// OnGrant is an in-process qualification hook, never a browser endpoint.
	OnGrant func(Grant)
}
type Grant struct {
	AccessToken, RefreshToken string
	Scopes                    []string
	Expires                   time.Time
}
type pending struct {
	Browser, Verifier string
	Expires           time.Time
	Scopes            []string
}
type browser struct {
	CSRF    string
	Expires time.Time
	Grant   *Grant
}
type Demo struct {
	mu       sync.Mutex
	config   Config
	client   *http.Client
	states   map[string]pending
	browsers map[string]browser
}

func New(c Config) (*Demo, error) {
	if c.PublicURL != "http://127.0.0.1:54444" && c.PublicURL != "http://127.0.0.1:54444/" {
		return nil, errors.New("fixed provider required")
	}
	if c.CallbackURL != callbackURL || c.ClientID != "leafrun-e06-demo" || c.Audience != "leafrun-api" || len(c.Scopes) == 0 {
		return nil, errors.New("fixed client configuration required")
	}
	u, err := url.Parse(c.TokenURL)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || !(u.Host == "hydra:4444" || u.Hostname() == "127.0.0.1") {
		return nil, errors.New("private provider configuration invalid")
	}
	seen := map[string]bool{}
	for _, scope := range c.Scopes {
		if !allowedScopes[scope] || seen[scope] {
			return nil, errors.New("scope configuration invalid")
		}
		seen[scope] = true
	}
	c.Scopes = append([]string(nil), c.Scopes...)
	sort.Strings(c.Scopes)
	client := &http.Client{Timeout: 5 * time.Second}
	if c.HTTPClient != nil {
		*client = *c.HTTPClient
	}
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if client.Timeout <= 0 || client.Timeout > 10*time.Second {
		client.Timeout = 5 * time.Second
	}
	return &Demo{config: c, client: client, states: map[string]pending{}, browsers: map[string]browser{}}, nil
}
func random() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("secure random unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}
func same(a, b string) bool {
	return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func (d *Demo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Chromium applies form-action to every public origin in the redirect chain.
	// These are the fixed E06 Hydra, Access and Kratos browser origins.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self' http://127.0.0.1:54444 http://127.0.0.1:58197 http://127.0.0.1:54433; base-uri 'none'; frame-ancestors 'none'")
	// A fixed Host stops DNS rebinding and accidental alternate-port callbacks.
	if r.Host != "localhost:58200" {
		d.deny(w)
		return
	}
	switch {
	case r.Method == "GET" && r.URL.Path == "/":
		d.home(w, r)
	case r.Method == "POST" && r.URL.Path == "/oauth/start":
		d.start(w, r)
	case r.Method == "GET" && r.URL.Path == "/oauth/callback":
		d.callback(w, r)
	case r.Method == "GET" && r.URL.Path == "/result":
		d.result(w, r)
	default:
		http.NotFound(w, r)
	}
}
func (d *Demo) deny(w http.ResponseWriter) {
	http.Error(w, "OAuth request could not be completed. Start again from the demo.", 400)
}
func (d *Demo) prune() {
	now := time.Now()
	for k, v := range d.states {
		if !v.Expires.After(now) {
			delete(d.states, k)
		}
	}
	for k, v := range d.browsers {
		if !v.Expires.After(now) {
			delete(d.browsers, k)
		}
	}
}
func browserID(r *http.Request) (string, bool) {
	var id string
	for _, c := range r.Cookies() {
		if c.Name == cookieName {
			if id != "" || len(c.Value) != 43 {
				return "", false
			}
			id = c.Value
		}
	}
	return id, id != ""
}

var page = template.Must(template.New("page").Parse(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Leafrun OAuth demo</title><style>body{font:18px system-ui;max-width:42rem;margin:4rem auto;padding:0 1rem;color:#202520}label{display:block;margin:1rem 0}button{font:inherit;padding:.6rem 1rem}</style><h1>Leafrun OAuth demo</h1>{{if .Complete}}<p>Authorization completed.</p><p>Granted scopes:</p><ul>{{range .Scopes}}<li>{{.}}</li>{{end}}</ul>{{else}}<p>Choose the permissions to request, then sign in and approve them.</p><form method="post" action="/oauth/start"><input type="hidden" name="csrf_token" value="{{.CSRF}}">{{range .Scopes}}<label><input type="checkbox" name="scope" value="{{.}}" checked> {{.}}</label>{{end}}<button type="submit">Authorize with Leafrun</button></form>{{end}}<p><a href="/">Start a new authorization</a></p></html>`))

func (d *Demo) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		d.deny(w)
		return
	}
	id, valid := browserID(r)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.prune()
	if !valid || d.browsers[id].Expires.IsZero() {
		id = random()
	}
	if len(d.browsers) >= 1024 {
		d.deny(w)
		return
	}
	b := browser{CSRF: random(), Expires: time.Now().Add(10 * time.Minute)}
	d.browsers[id] = b
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	// Chrome suppresses navigation POST Origin under no-referrer. Only this
	// form page uses origin-only referrers; callback/result pages keep no-referrer
	// so transient authorization codes can never become a referrer URL.
	w.Header().Set("Referrer-Policy", "origin")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = page.Execute(w, struct {
		Complete bool
		CSRF     string
		Scopes   []string
	}{false, b.CSRF, d.config.Scopes})
}
func (d *Demo) start(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" || r.Header.Get("Origin") != browserOrigin || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		d.deny(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if r.ParseForm() != nil {
		d.deny(w)
		return
	}
	for key := range r.PostForm {
		if key != "csrf_token" && key != "scope" {
			d.deny(w)
			return
		}
	}
	csrf := r.PostForm["csrf_token"]
	id, ok := browserID(r)
	if !ok || len(csrf) != 1 {
		d.deny(w)
		return
	}
	scopes := r.PostForm["scope"]
	if len(scopes) == 0 || len(scopes) > len(d.config.Scopes) {
		d.deny(w)
		return
	}
	allowed := map[string]bool{}
	for _, s := range d.config.Scopes {
		allowed[s] = true
	}
	for _, s := range scopes {
		if !allowed[s] {
			d.deny(w)
			return
		}
		delete(allowed, s)
	}
	scopes = append([]string(nil), scopes...)
	sort.Strings(scopes)
	d.mu.Lock()
	d.prune()
	b, ok := d.browsers[id]
	if !ok || b.CSRF == "" || !same(b.CSRF, csrf[0]) || len(d.states) >= 1024 {
		d.mu.Unlock()
		d.deny(w)
		return
	}
	b.CSRF = ""
	d.browsers[id] = b
	state, verifier := random(), random()
	d.states[state] = pending{id, verifier, time.Now().Add(5 * time.Minute), scopes}
	d.mu.Unlock()
	hash := sha256.Sum256([]byte(verifier))
	q := url.Values{"response_type": {"code"}, "client_id": {d.config.ClientID}, "redirect_uri": {d.config.CallbackURL}, "scope": {strings.Join(scopes, " ")}, "audience": {d.config.Audience}, "state": {state}, "code_challenge": {base64.RawURLEncoding.EncodeToString(hash[:])}, "code_challenge_method": {"S256"}}
	http.Redirect(w, r, strings.TrimRight(d.config.PublicURL, "/")+"/oauth2/auth?"+q.Encode(), 303)
}
func (d *Demo) callback(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.RawQuery) > 8192 {
		d.deny(w)
		return
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	_, scopePresent := q["scope"]
	if err != nil || (len(q) != 2 && !(len(q) == 3 && scopePresent)) || len(q["code"]) != 1 || len(q["state"]) != 1 || (scopePresent && (len(q["scope"]) != 1 || len(q.Get("scope")) > 2048)) || q.Get("code") == "" || len(q.Get("code")) > 4096 || len(q.Get("state")) > 128 {
		d.deny(w)
		return
	}
	id, ok := browserID(r)
	if !ok {
		d.deny(w)
		return
	}
	d.mu.Lock()
	d.prune()
	p, ok := d.states[q.Get("state")]
	if !ok || !same(p.Browser, id) {
		d.mu.Unlock()
		d.deny(w)
		return
	}
	callbackScopes := strings.Fields(q.Get("scope"))
	sort.Strings(callbackScopes)
	if scopePresent && !scopeSubset(callbackScopes, p.Scopes) {
		d.mu.Unlock()
		d.deny(w)
		return
	}
	delete(d.states, q.Get("state"))
	d.mu.Unlock()
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {d.config.ClientID}, "redirect_uri": {d.config.CallbackURL}, "code": {q.Get("code")}, "code_verifier": {p.Verifier}}
	req, err := http.NewRequestWithContext(r.Context(), "POST", strings.TrimRight(d.config.TokenURL, "/")+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		d.deny(w)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		d.deny(w)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 65536))
		d.deny(w)
		return
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(data) > 65536 || jsonbound.Decode(data, &out) != nil || !strings.EqualFold(out.TokenType, "Bearer") || out.AccessToken == "" || len(out.AccessToken) > 8192 || len(out.RefreshToken) > 8192 || out.ExpiresIn <= 0 || out.ExpiresIn > 3600 {
		d.deny(w)
		return
	}
	scopes := strings.Fields(out.Scope)
	sort.Strings(scopes)
	if !scopeSubset(scopes, p.Scopes) || (scopePresent && strings.Join(callbackScopes, " ") != strings.Join(scopes, " ")) {
		d.deny(w)
		return
	}
	g := Grant{out.AccessToken, out.RefreshToken, scopes, time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)}
	d.mu.Lock()
	b, ok := d.browsers[id]
	if !ok || !b.Expires.After(time.Now()) {
		d.mu.Unlock()
		d.deny(w)
		return
	}
	b.Grant = &g
	d.browsers[id] = b
	d.mu.Unlock()
	if d.config.OnGrant != nil {
		d.config.OnGrant(g)
	}
	http.Redirect(w, r, "/result", 303)
}

func scopeSubset(scopes, requested []string) bool {
	allowed := map[string]bool{}
	for _, scope := range requested {
		allowed[scope] = true
	}
	if len(scopes) == 0 {
		return false
	}
	for _, scope := range scopes {
		if !allowed[scope] {
			return false
		}
		delete(allowed, scope)
	}
	return true
}
func (d *Demo) result(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		d.deny(w)
		return
	}
	id, ok := browserID(r)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.prune()
	b := d.browsers[id]
	if !ok || b.Grant == nil || !b.Grant.Expires.After(time.Now()) {
		d.deny(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = page.Execute(w, struct {
		Complete bool
		CSRF     string
		Scopes   []string
	}{true, "", b.Grant.Scopes})
}
