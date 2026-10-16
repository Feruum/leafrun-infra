package hydraprovision

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProvisionCreatesThenVerifiesExactClient(t *testing.T) {
	exists := false
	posts := 0
	p := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			if !exists {
				w.WriteHeader(404)
				return
			}
			json.NewEncoder(w).Encode(FixedClient())
			return
		}
		if r.Method != "POST" || r.URL.Path != "/admin/clients" {
			t.Error("unexpected mutation")
		}
		var c Client
		if json.NewDecoder(r.Body).Decode(&c) != nil || !equal(c, FixedClient()) {
			t.Error("client not fixed")
		}
		posts++
		exists = true
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]any{"client_secret": "never-report"})
	}))
	defer p.Close()
	if Provision(p.URL, p.Client()) != nil || Provision(p.URL, p.Client()) != nil || posts != 1 {
		t.Fatal("idempotent provision failed")
	}
}
func TestExistingMismatchNeverRewrites(t *testing.T) {
	for _, mutate := range []func(*Client){func(c *Client) { c.RedirectURIs = []string{"http://localhost:58201/oauth/callback"} }, func(c *Client) { c.GrantTypes = append(c.GrantTypes, "client_credentials") }, func(c *Client) { c.SubjectType = "pairwise" }, func(c *Client) { c.TokenEndpointAuthMethod = "client_secret_basic" }, func(c *Client) { c.Scope += " unknown" }, func(c *Client) { c.SkipConsent = true }} {
		mutations := 0
		p := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				mutations++
				w.WriteHeader(500)
				return
			}
			c := FixedClient()
			mutate(&c)
			json.NewEncoder(w).Encode(c)
		}))
		if Provision(p.URL, p.Client()) == nil || mutations != 0 {
			t.Error("mismatch mutated or accepted")
		}
		p.Close()
	}
}
func TestProviderRedirectAndOversizeFailClosed(t *testing.T) {
	for _, handler := range []http.HandlerFunc{func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "http://evil.test", 302) }, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }} {
		p := httptest.NewServer(handler)
		if Provision(p.URL, p.Client()) == nil {
			t.Error("unsafe provider result accepted")
		}
		p.Close()
	}
}

func TestDuplicateClientJSONRejected(t *testing.T) {
	p := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := json.Marshal(FixedClient())
		data = append([]byte(`{"client_id":"unexpected",`), data[1:]...)
		w.Write(data)
	}))
	defer p.Close()
	if Provision(p.URL, p.Client()) == nil {
		t.Fatal("duplicate client JSON accepted")
	}
}

func TestCaseFoldClientJSONRejected(t *testing.T) {
	for _, alias := range []string{"REDIRECT_URIS", "redirect_uris"} {
		p := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data, _ := json.Marshal(FixedClient())
			data = append([]byte(`{"`+alias+`":["http://localhost:58201/oauth/callback"],`), data[1:]...)
			w.Write(data)
		}))
		if Provision(p.URL, p.Client()) == nil {
			t.Error("case alias client JSON accepted")
		}
		p.Close()
	}
}
