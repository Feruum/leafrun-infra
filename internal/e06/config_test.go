package e06

import (
	"os"
	"strings"
	"testing"
)

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile("../../" + path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func TestComposeIsSeparateAndAdminPrivate(t *testing.T) {
	s := read(t, "compose.e06.yaml")
	for _, v := range []string{"name: leafrun-e06", "127.0.0.1:58534:5432", "127.0.0.1:54433:4433", "127.0.0.1:58197:8080", "127.0.0.1:58198:8080", "access-api-a:", "access-api-b:", "e06-postgres-data:", "oryd/kratos:v26.2.0@sha256:2a13bb8d362c7a7ae33bd7c0f5168aee46921f15c916a06346db91c06dc76643"} {
		if !strings.Contains(s, v) {
			t.Fatalf("missing %s", v)
		}
	}
	for _, v := range []string{"4434:4434", "4445:4445", "leafrun-e04", "--watch-courier"} {
		if strings.Contains(s, v) {
			t.Fatalf("forbidden %s", v)
		}
	}
}
func TestKratosConfigurationHasMinimalBrowserProtocol(t *testing.T) {
	s := read(t, "ory/kratos.yaml")
	for _, v := range []string{"version: v26.2.0", "http://127.0.0.1:54433/", "http://kratos:4434/", "http://127.0.0.1:58197", "http://127.0.0.1:58198", "same_site: Lax", "name: ory_kratos_session", "min_password_length: 12", "haveibeenpwned_enabled: false"} {
		if !strings.Contains(s, v) {
			t.Fatalf("missing %s", v)
		}
	}
	for _, v := range []string{"${", "dsn:", "cookie:", "cipher:", "smtp:", "domain:"} {
		if strings.Contains(s, v) && v != "cookie:" {
			t.Fatalf("secret or unneeded setting %s", v)
		}
	}
	schema := read(t, "ory/identity.schema.json")
	if !strings.Contains(schema, `"identifier": true`) || strings.Contains(schema, "verification") || strings.Contains(schema, "recovery") {
		t.Fatal("schema identifier ownership")
	}
}

func TestBothNodesUseCanonicalBrowserReturnOrigin(t *testing.T) {
	s := read(t, "compose.e06.yaml")
	if strings.Count(s, "ACCESS_PUBLIC_URL:") != 1 || !strings.Contains(s, "ACCESS_PUBLIC_URL: http://127.0.0.1:58197/") {
		t.Fatal("provider fixed login UI requires the same canonical browser return origin on both nodes")
	}
}
