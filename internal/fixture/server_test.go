package fixture

import (
	"bytes"
	"encoding/json"
	v "github.com/leafrun/leafrun-contracts/storage/v1"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChecksDenyWithoutEvidence(t *testing.T) {
	s := New("owner-token", "admin-token")
	check := func(path, token string, body any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", path, bytes.NewReader(b))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	scope := v.Scope{WorkspaceID: "demo", ProjectID: "roundtrip"}
	if w := check("/internal/v1/checks/access", "wrong", v.AccessCheck{}); w.Code != http.StatusUnauthorized {
		t.Fatal(w.Code)
	}
	w := check("/internal/v1/checks/access", "owner-token", v.AccessCheck{Scope: scope, Principal: v.Principal{Subject: "other", Domain: "core"}})
	var access v.AccessDecision
	_ = json.Unmarshal(w.Body.Bytes(), &access)
	if access.Allowed {
		t.Fatal("unexpected principal allowed")
	}
	w = check("/internal/v1/checks/publishable", "owner-token", v.PublishabilityCheck{Scope: scope, ArtifactID: "pdf", OriginJobID: "job"})
	var pub v.PublishabilityDecision
	_ = json.Unmarshal(w.Body.Bytes(), &pub)
	if pub.Publishable {
		t.Fatal("unbound artifact published")
	}
	w = check("/internal/v1/checks/pin-owner", "owner-token", v.PinOwnerCheck{Scope: scope, PinID: "missing"})
	var pin v.PinOwnerDecision
	_ = json.Unmarshal(w.Body.Bytes(), &pin)
	if pin.State != v.OwnerUnknown {
		t.Fatal("missing owner evidence must be unknown")
	}
}
