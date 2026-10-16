// Package hydraprovision provisions only the fixed E06 public demo client.
package hydraprovision

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/leafrun/leafrun-infra/internal/jsonbound"
)

type Client struct {
	ID                      string   `json:"client_id"`
	Name                    string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	Scope                   string   `json:"scope"`
	Audience                []string `json:"audience"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	SubjectType             string   `json:"subject_type"`
	AccessTokenStrategy     string   `json:"access_token_strategy"`
	SkipConsent             bool     `json:"skip_consent"`
	AllowedCORSOrigins      []string `json:"allowed_cors_origins"`
}

func FixedClient() Client {
	return Client{ID: "leafrun-e06-demo", Name: "Leafrun E06 OAuth demo", RedirectURIs: []string{"http://localhost:58200/oauth/callback"}, GrantTypes: []string{"authorization_code", "refresh_token"}, ResponseTypes: []string{"code"}, Scope: "leafrun.generate leafrun.status leafrun.source.read leafrun.resource.read leafrun.download offline_access", Audience: []string{"leafrun-api"}, TokenEndpointAuthMethod: "none", SubjectType: "public", AccessTokenStrategy: "opaque", SkipConsent: false, AllowedCORSOrigins: []string{}}
}
func set(a, b []string) bool {
	a = append([]string(nil), a...)
	b = append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	return len(a) == len(b) && strings.Join(a, "\x00") == strings.Join(b, "\x00")
}
func equal(a, b Client) bool {
	return a.ID == b.ID && a.Name == b.Name && set(a.RedirectURIs, b.RedirectURIs) && set(a.GrantTypes, b.GrantTypes) && set(a.ResponseTypes, b.ResponseTypes) && set(strings.Fields(a.Scope), strings.Fields(b.Scope)) && set(a.Audience, b.Audience) && a.TokenEndpointAuthMethod == b.TokenEndpointAuthMethod && a.SubjectType == b.SubjectType && a.AccessTokenStrategy == b.AccessTokenStrategy && a.SkipConsent == b.SkipConsent && set(a.AllowedCORSOrigins, b.AllowedCORSOrigins)
}

var failure = errors.New("fixed Hydra client provisioning or verification failed; no existing client rewritten")

func Provision(base string, client *http.Client) error {
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || !(u.Host == "hydra:4445" || u.Hostname() == "127.0.0.1") {
		return failure
	}
	c := http.Client{Timeout: 5 * time.Second}
	if client != nil {
		c = *client
	}
	c.Jar = nil
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if c.Timeout <= 0 || c.Timeout > 10*time.Second {
		c.Timeout = 5 * time.Second
	}
	base = strings.TrimRight(base, "/")
	get := func() (int, Client, error) {
		resp, err := c.Get(base + "/admin/clients/leafrun-e06-demo")
		if err != nil {
			return 0, Client{}, failure
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
		if err != nil || len(data) > 65536 {
			return 0, Client{}, failure
		}
		var got Client
		if resp.StatusCode == 200 && jsonbound.Decode(data, &got) != nil {
			return 0, Client{}, failure
		}
		return resp.StatusCode, got, nil
	}
	status, got, err := get()
	if err != nil {
		return failure
	}
	if status == 200 {
		if !equal(got, FixedClient()) {
			return failure
		}
		return nil
	}
	if status != 404 {
		return failure
	}
	data, _ := json.Marshal(FixedClient())
	resp, err := c.Post(base+"/admin/clients", "application/json", bytes.NewReader(data))
	if err != nil {
		return failure
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 65536))
	resp.Body.Close()
	if resp.StatusCode != 201 && resp.StatusCode != 409 {
		return failure
	}
	status, got, err = get()
	if err != nil || status != 200 || !equal(got, FixedClient()) {
		return failure
	}
	return nil
}
