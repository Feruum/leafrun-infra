// Package ownerrouter is the internal dev stand's explicitly configured owner
// routing. Access fixtures never answer Generation publication/pin ownership.
package ownerrouter

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	jsonvalue "github.com/leafrun/leafrun-contracts/jsonvalue/v1"
	storagev1 "github.com/leafrun/leafrun-contracts/storage/v1"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	DevFixture                                                             bool
	FixtureURL, FixtureToken, GenerationURL, GenerationToken, StorageToken string
	ProjectsURL, ProjectsToken                                             string
}
type Server struct {
	config Config
	client *http.Client
}

func New(config Config, client *http.Client) (*Server, error) {
	if !config.DevFixture || config.FixtureToken == "" || config.GenerationToken == "" || config.StorageToken == "" {
		return nil, errors.New("explicit dev fixture and all owner credentials are required")
	}
	for _, base := range []string{config.FixtureURL, config.GenerationURL} {
		u, err := url.Parse(base)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(base, "#") {
			return nil, errors.New("invalid configured owner URL")
		}
	}
	if config.ProjectsURL != "" || config.ProjectsToken != "" {
		u, err := url.Parse(config.ProjectsURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(config.ProjectsURL, "#") || config.ProjectsToken == "" {
			return nil, errors.New("Projects owner URL and fixed credential are required together")
		}
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	copy := *client
	if copy.Timeout <= 0 {
		copy.Timeout = 10 * time.Second
	}
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Server{config: config, client: &copy}, nil
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == "/healthz" {
		w.WriteHeader(204)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")), []byte(s.config.StorageToken)) != 1 {
		w.WriteHeader(401)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	base, token := s.config.GenerationURL, s.config.GenerationToken
	switch r.URL.Path {
	case "/internal/v1/checks/access":
		base, token = s.config.FixtureURL, s.config.FixtureToken
	case "/internal/v1/checks/publishable", "/internal/v1/checks/pin-owner":
	default:
		w.WriteHeader(404)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		w.WriteHeader(413)
		return
	}
	if r.URL.Path == "/internal/v1/checks/pin-owner" {
		var check storagev1.PinOwnerCheck
		if jsonvalue.Unmarshal(body, &check, 1<<20) != nil {
			w.WriteHeader(400)
			return
		}
		switch {
		case check.HolderID == "leafrun-projects" && check.HolderDomain == "service":
			if s.config.ProjectsURL == "" {
				w.WriteHeader(503)
				return
			}
			base, token = s.config.ProjectsURL, s.config.ProjectsToken
		case check.HolderID == "leafrun-generation" && check.HolderDomain == "service":
		case check.HolderID == "demo" && check.HolderDomain == "core":
			base, token = s.config.FixtureURL, s.config.FixtureToken
		default:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(storagev1.PinOwnerDecision{State: storagev1.OwnerUnknown})
			return
		}
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, strings.TrimRight(base, "/")+r.URL.Path, bytes.NewReader(body))
	if err != nil {
		w.WriteHeader(503)
		return
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		w.WriteHeader(503)
		return
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 || response.StatusCode != 200 {
		w.WriteHeader(503)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write(data)
}
