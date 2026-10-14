// Package fixture provides explicitly enabled E03 owner checks. It is never a production adapter.
package fixture

import (
	"crypto/subtle"
	"encoding/json"
	v "github.com/leafrun/leafrun-contracts/storage/v1"
	"net/http"
	"strings"
	"sync"
)

// State is mutable only through an independently authenticated dev control API.
type State struct {
	Scope       v.Scope                 `json:"scope"`
	Deny        bool                    `json:"deny"`
	Unavailable bool                    `json:"unavailable"`
	Published   map[string]string       `json:"published"` // artifact ID -> job ID
	Pins        map[string]v.OwnerState `json:"pins"`
}
type Server struct {
	mu                     sync.RWMutex
	state                  State
	ownerToken, adminToken string
}

func New(ownerToken, adminToken string) *Server {
	return &Server{state: State{Scope: v.Scope{WorkspaceID: "demo", ProjectID: "roundtrip"}, Published: map[string]string{}, Pins: map[string]v.OwnerState{}}, ownerToken: ownerToken, adminToken: adminToken}
}
func authorized(r *http.Request, token string) bool {
	return token != "" && subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")), []byte(token)) == 1 && strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ")
}
func decode(w http.ResponseWriter, r *http.Request, x any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(x) != nil {
		http.Error(w, "invalid request", 400)
		return false
	}
	return true
}
func respond(w http.ResponseWriter, x any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(x)
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" && r.Method == "GET" {
		w.WriteHeader(204)
		return
	}
	if r.URL.Path == "/dev/v1/state" && r.Method == "PUT" {
		if !authorized(r, s.adminToken) {
			w.WriteHeader(401)
			return
		}
		var next State
		if !decode(w, r, &next) {
			return
		}
		if next.Scope.WorkspaceID == "" || next.Scope.ProjectID == "" {
			w.WriteHeader(400)
			return
		}
		for _, state := range next.Pins {
			if state != v.OwnerReferenced && state != v.OwnerAbsent && state != v.OwnerUnknown {
				w.WriteHeader(400)
				return
			}
		}
		s.mu.Lock()
		s.state = next
		s.mu.Unlock()
		w.WriteHeader(204)
		return
	}
	if !authorized(r, s.ownerToken) {
		w.WriteHeader(401)
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.Unavailable {
		w.WriteHeader(503)
		return
	}
	switch r.URL.Path {
	case "/internal/v1/checks/access":
		var in v.AccessCheck
		if !decode(w, r, &in) {
			return
		}
		principalAllowed := in.Principal == (v.Principal{Subject: "demo", Domain: "core"}) || in.Principal == (v.Principal{Subject: "leafrun-generation", Domain: "service"})
		respond(w, v.AccessDecision{Allowed: !s.state.Deny && in.Scope == s.state.Scope && principalAllowed})
	case "/internal/v1/checks/publishable":
		var in v.PublishabilityCheck
		if !decode(w, r, &in) {
			return
		}
		job, ok := s.state.Published[in.ArtifactID]
		respond(w, v.PublishabilityDecision{Publishable: !s.state.Deny && in.Scope == s.state.Scope && ok && job == in.OriginJobID})
	case "/internal/v1/checks/pin-owner":
		var in v.PinOwnerCheck
		if !decode(w, r, &in) {
			return
		}
		state := v.OwnerUnknown
		if in.Scope == s.state.Scope {
			if x, ok := s.state.Pins[in.PinID]; ok {
				state = x
			}
		}
		respond(w, v.PinOwnerDecision{State: state})
	default:
		w.WriteHeader(404)
	}
}
