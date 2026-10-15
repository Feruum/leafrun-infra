package ownerrouter

import (
	"bytes"
	"encoding/json"
	storagev1 "github.com/leafrun/leafrun-contracts/storage/v1"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouterUsesExactProjectsPinOwnerAndCredential(t *testing.T) {
	projectsCalls := 0
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("pin ownership routed to another domain")
		w.WriteHeader(500)
	}))
	defer other.Close()
	projects := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectsCalls++
		if r.Header.Get("Authorization") != "Bearer projects-owner" || r.URL.Path != "/internal/v1/checks/pin-owner" {
			t.Error("incorrect Projects route credential/path")
		}
		_, _ = io.WriteString(w, `{"state":"referenced"}`)
	}))
	defer projects.Close()
	router, err := New(Config{DevFixture: true, FixtureURL: other.URL, FixtureToken: "fixture", GenerationURL: other.URL, GenerationToken: "generation", StorageToken: "storage", ProjectsURL: projects.URL, ProjectsToken: "projects-owner"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(storagev1.PinOwnerCheck{PinID: "pin1", HolderID: "leafrun-projects", HolderDomain: "service"})
	req := httptest.NewRequest("POST", "/internal/v1/checks/pin-owner", bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer storage")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 200 || projectsCalls != 1 {
		t.Fatalf("Projects pin owner not reached: status=%d calls=%d", w.Code, projectsCalls)
	}
}

func TestProjectsOwnerUnavailableDoesNotFallBack(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("fallback to another owner"); w.WriteHeader(200) }))
	defer other.Close()
	router, err := New(Config{DevFixture: true, FixtureURL: other.URL, FixtureToken: "fixture", GenerationURL: other.URL, GenerationToken: "generation", StorageToken: "storage"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(storagev1.PinOwnerCheck{PinID: "pin1", HolderID: "leafrun-projects", HolderDomain: "service"})
	req := httptest.NewRequest("POST", "/internal/v1/checks/pin-owner", bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer storage")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatalf("missing Projects owner did not fail closed: %d", w.Code)
	}
}
