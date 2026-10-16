package ownerrouter

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoutesAccessToExplicitFixtureAndOwnershipToGeneration(t *testing.T) {
	var fixtureCalls, generationCalls int
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixtureCalls++
		if r.Header.Get("Authorization") != "Bearer fixture" || r.URL.Path != "/internal/v1/checks/access" {
			t.Error("incorrect access route")
		}
		io.WriteString(w, `{"allowed":true}`)
	}))
	defer fixture.Close()
	generation := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		generationCalls++
		if r.Header.Get("Authorization") != "Bearer generation" || r.URL.Path == "/internal/v1/checks/access" {
			t.Error("incorrect owner route")
		}
		io.WriteString(w, `{"publishable":false}`)
	}))
	defer generation.Close()
	router, err := New(Config{DevFixture: true, FixtureURL: fixture.URL, FixtureToken: "fixture", GenerationURL: generation.URL, GenerationToken: "generation", StorageToken: "storage"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"access", "publishable", "pin-owner"} {
		body := `{}`
		if path == "pin-owner" {
			body = `{"holderId":"leafrun-generation","holderDomain":"service"}`
		}
		req := httptest.NewRequest("POST", "/internal/v1/checks/"+path, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer storage")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if fixtureCalls != 1 || generationCalls != 2 {
		t.Fatal(fixtureCalls, generationCalls)
	}
}
func TestRouterFailsClosedAndRequiresDevProfile(t *testing.T) {
	if _, err := New(Config{}, nil); err == nil {
		t.Fatal("implicit dev fixture")
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("must not fallback to fixture")
		w.WriteHeader(500)
	}))
	defer fixture.Close()
	generation := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer generation.Close()
	router, err := New(Config{DevFixture: true, FixtureURL: fixture.URL, FixtureToken: "fixture", GenerationURL: generation.URL, GenerationToken: "generation", StorageToken: "storage"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/internal/v1/checks/publishable", bytes.NewBufferString(`{}`))
	req.Header.Set("Authorization", "Bearer storage")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	req = httptest.NewRequest("POST", "/internal/v1/checks/pin-owner", bytes.NewBufferString(`{}`))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}
