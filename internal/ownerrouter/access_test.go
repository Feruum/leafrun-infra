package ownerrouter

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccessModeRouterHasNoFixtureFallback(t *testing.T) {
	calls := 0
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"publishable":true}`))
	}))
	defer owner.Close()
	router, err := New(Config{AccessMode: true, GenerationURL: owner.URL, GenerationToken: "generation", StorageToken: "storage"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, body string
		status     int
	}{{"access", `{}`, 404}, {"publishable", `{}`, 200}, {"pin-owner", `{"holderId":"demo","holderDomain":"core"}`, 200}} {
		r := httptest.NewRequest("POST", "/internal/v1/checks/"+tc.path, strings.NewReader(tc.body))
		r.Header.Set("Authorization", "Bearer storage")
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
		if tc.path == "pin-owner" && !strings.Contains(w.Body.String(), "unknown") {
			t.Fatal("unconfigured legacy owner answered")
		}
	}
	if calls != 1 {
		t.Fatalf("unexpected owner calls %d", calls)
	}
}
