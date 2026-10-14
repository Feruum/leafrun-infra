package main

import (
	"strings"
	"testing"
)

func TestBootstrapRequiresExplicitDevelopmentProfileBeforeCredentials(t *testing.T) {
	t.Setenv("LEAFRUN_DEV_FIXTURE", "false")
	t.Setenv("LEAFRUN_NATS_ADDRESS", "")
	if err := run(); err == nil || !strings.Contains(err.Error(), "explicit development profile") {
		t.Fatalf("bootstrap did not require explicit development opt-in: %v", err)
	}
}
