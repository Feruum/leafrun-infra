package fixture

import (
	"bytes"
	"encoding/json"
	v "github.com/leafrun/leafrun-contracts/storage/v1"
	"net/http/httptest"
	"testing"
)

func TestDevGenerationOnlyRuleRestrictsSourceAndExactFileIDs(t *testing.T) {
	server := New("owner", "admin")
	actor := v.Principal{Subject: "generator", Domain: "user"}
	scope := v.Scope{WorkspaceID: "demo", ProjectID: "roundtrip"}
	state := State{Scope: scope, AccessRules: []AccessRule{{Principal: actor, Operations: map[string]bool{"generation.create": true, "resource.read": true}, ObjectIDs: map[string][]string{"resource.read": {"allowed-input"}}}}}
	data, _ := json.Marshal(state)
	req := httptest.NewRequest("PUT", "/dev/v1/state", bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer admin")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Fatalf("state setup: %d", w.Code)
	}
	for _, test := range []struct {
		operation, object string
		allowed           bool
	}{
		{"generation.create", "", true}, {"source.read", "snapshot", false}, {"snapshot.read", "snapshot", false}, {"resource.read", "allowed-input", true}, {"resource.read", "source-resource", false},
	} {
		data, _ = json.Marshal(v.AccessCheck{Principal: actor, Scope: scope, Operation: test.operation, ObjectID: test.object})
		req = httptest.NewRequest("POST", "/internal/v1/checks/access", bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer owner")
		w = httptest.NewRecorder()
		server.ServeHTTP(w, req)
		var decision v.AccessDecision
		if err := json.Unmarshal(w.Body.Bytes(), &decision); err != nil || decision.Allowed != test.allowed {
			t.Errorf("operation %s object %s: %+v %v", test.operation, test.object, decision, err)
		}
	}
}

func TestDevProjectsServiceIdentityIsExplicitlyAllowed(t *testing.T) {
	server := New("owner", "admin")
	data, _ := json.Marshal(v.AccessCheck{Principal: v.Principal{Subject: "leafrun-projects", Domain: "service"}, Scope: v.Scope{WorkspaceID: "demo", ProjectID: "roundtrip"}, Operation: "pin.release", ObjectID: "pin"})
	req := httptest.NewRequest("POST", "/internal/v1/checks/access", bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer owner")
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)
	var decision v.AccessDecision
	_ = json.Unmarshal(w.Body.Bytes(), &decision)
	if w.Code != 200 || !decision.Allowed {
		t.Fatalf("explicit Projects service denied: %d %+v", w.Code, decision)
	}
}
