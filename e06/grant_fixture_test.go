package e06

import (
	"testing"
	"time"

	access "github.com/leafrun/leafrun-contracts/access/v1"
)

func TestGuestCreateFixtureOperationKeyCanonical(t *testing.T) {
	key := ownerGrantOperationKey(t)
	if err := access.ValidateGrantOperationKey(key, time.Now()); err != nil {
		t.Fatal("new grant fixture must use the canonical bounded retry key")
	}
}
