package e06

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestHydraFixedProviderContract(t *testing.T) {
	if os.Getenv("E06_INTEGRATION") != "true" {
		t.Skip("E06_INTEGRATION not enabled")
	}
	base := os.Getenv("E06_HYDRA_INTERNAL_URL")
	admin := os.Getenv("E06_HYDRA_ADMIN_URL")
	if base == "" || admin == "" {
		t.Fatal("Hydra private fixture configuration missing")
	}
	c := ownerHTTP(nil)
	status, data := ownerRequest(t, c, "GET", base+"/.well-known/openid-configuration", "", "", "", nil)
	var discovery struct {
		Issuer  string   `json:"issuer"`
		Methods []string `json:"code_challenge_methods_supported"`
	}
	if status != 200 || json.Unmarshal(data, &discovery) != nil || discovery.Issuer != "http://127.0.0.1:54444/" || !strings.Contains(" "+strings.Join(discovery.Methods, " ")+" ", " S256 ") {
		t.Fatal("Pinned Hydra issuer or S256 contract invalid")
	}
	status, data = ownerRequest(t, c, "GET", admin+"/admin/clients/leafrun-e06-demo", "", "", "", nil)
	var client struct {
		RedirectURIs []string `json:"redirect_uris"`
		GrantTypes   []string `json:"grant_types"`
		Auth         string   `json:"token_endpoint_auth_method"`
		Subject      string   `json:"subject_type"`
	}
	if status != 200 || json.Unmarshal(data, &client) != nil || len(client.RedirectURIs) != 1 || client.RedirectURIs[0] != "http://localhost:58200/oauth/callback" || strings.Join(client.GrantTypes, " ") != "authorization_code refresh_token" || client.Auth != "none" || client.Subject != "public" {
		t.Fatal("Fixed public client contract invalid")
	}
	for caseIndex, modify := range []func(url.Values){func(q url.Values) { q.Set("redirect_uri", "http://localhost:58201/oauth/callback") }, func(q url.Values) { q.Set("scope", "unknown_scope") }, func(q url.Values) { q.Set("audience", "unknown-audience") }} {
		q := url.Values{"client_id": {"leafrun-e06-demo"}, "response_type": {"code"}, "redirect_uri": {"http://localhost:58200/oauth/callback"}, "scope": {"leafrun.generate"}, "audience": {"leafrun-api"}, "state": {"safe-fixture-state"}, "code_challenge": {"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, "code_challenge_method": {"S256"}}
		modify(q)
		r, err := http.NewRequest("GET", "http://127.0.0.1:54444/oauth2/auth?"+q.Encode(), nil)
		if err != nil {
			t.Fatal("authorization fixture invalid")
		}
		resp, err := c.Do(r)
		if err != nil {
			t.Fatal("authorization probe failed")
		}
		io.Copy(io.Discard, io.LimitReader(resp.Body, 65536))
		resp.Body.Close()
		location, err := url.Parse(resp.Header.Get("Location"))
		if err != nil || resp.StatusCode < 300 || resp.StatusCode >= 500 || location.Path == "/oauth/login" || location.Query().Get("login_challenge") != "" {
			t.Fatalf("Unsafe authorization probe %d returned status %d path %s", caseIndex, resp.StatusCode, location.Path)
		}
	}
	form := url.Values{"client_id": {"leafrun-e06-demo"}, "grant_type": {"client_credentials"}}
	status, _ = ownerRequest(t, c, "POST", base+"/oauth2/token", "", "", "application/x-www-form-urlencoded", []byte(form.Encode()))
	if status != 400 && status != 401 {
		t.Fatal("Human demo client credentials grant permitted")
	}
	// The admin listener is absent from the public router.
	status, _ = ownerRequest(t, c, "GET", base+"/admin/clients/leafrun-e06-demo", "", "", "", nil)
	if status != 404 {
		t.Fatal("Hydra public listener exposed admin registration")
	}
	t.Log("Actual pinned provider rejects alternate localhost port, unregistered scope/audience and client_credentials; admin remains private")
}

func TestHydraPKCERequestBoundary(t *testing.T) {
	if os.Getenv("E06_INTEGRATION") != "true" {
		t.Skip("E06_INTEGRATION not enabled")
	}
	for _, plain := range []bool{false, true} {
		c := ownerHTTP(nil)
		q := url.Values{"client_id": {"leafrun-e06-demo"}, "response_type": {"code"}, "redirect_uri": {"http://localhost:58200/oauth/callback"}, "scope": {"leafrun.generate"}, "audience": {"leafrun-api"}, "state": {"safe-fixture-state"}, "code_challenge_method": {"S256"}}
		if plain {
			q.Set("code_challenge_method", "plain")
			q.Set("code_challenge", strings.Repeat("x", 43))
		}
		r, _ := http.NewRequest("GET", "http://127.0.0.1:54444/oauth2/auth?"+q.Encode(), nil)
		resp, err := c.Do(r)
		if err != nil {
			t.Fatal("actual PKCE request failed")
		}
		resp.Body.Close()
		if resp.StatusCode == 400 {
			continue
		}
		location, err := url.Parse(resp.Header.Get("Location"))
		if err != nil || location.Scheme != "http" || location.Host != "127.0.0.1:58197" || location.Path != "/oauth/login" {
			t.Fatal("PKCE request did not deny or use fixed Access login")
		}
		req, _ := http.NewRequest("GET", location.String(), nil)
		denied, err := c.Do(req)
		if err != nil {
			t.Fatal("Access PKCE boundary request failed")
		}
		denied.Body.Close()
		if denied.StatusCode != 401 {
			t.Fatal("Access admitted missing/plain PKCE request")
		}
	}
	t.Log("Pinned Hydra can defer PKCE checking until its login stage; actual fixed Access request validation denied missing/plain S256 before acceptance")
}
