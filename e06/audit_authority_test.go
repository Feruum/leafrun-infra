package e06

import (
	"testing"

	identity "github.com/leafrun/leafrun-contracts/identity/v1"
)

func TestAuditConsentOriginalAuthorityBinding(t *testing.T) {
	const actor = "fixture-human"
	const providerSession = "22222222-2222-4222-8222-222222222222"
	registered := auditAuthorityBinding{Ref: identity.AuthorityRef{Kind: identity.KindKratosSession, ID: "11111111-1111-4111-8111-111111111111", Version: 1}, ActorID: actor, SessionID: providerSession}
	saved := identity.Identity{Principal: identity.Principal{Domain: "human", Subject: actor}, Authority: registered.Ref}
	t.Run("distinct owned authority and original provider IDs", func(t *testing.T) {
		if !auditConsentAuthorityMatches(saved, actor, providerSession, registered) {
			t.Fatal("separate owned authority ID must match its exact original provider binding")
		}
	})
	for _, change := range []struct {
		name   string
		mutate func(*identity.Identity, *auditAuthorityBinding)
	}{
		{"provider ID substituted for authority", func(id *identity.Identity, _ *auditAuthorityBinding) { id.Authority.ID = providerSession }},
		{"saved authority ID swapped", func(id *identity.Identity, _ *auditAuthorityBinding) {
			id.Authority.ID = "33333333-3333-4333-8333-333333333333"
		}},
		{"saved authority version swapped", func(id *identity.Identity, _ *auditAuthorityBinding) { id.Authority.Version++ }},
		{"registry version swapped", func(_ *identity.Identity, binding *auditAuthorityBinding) { binding.Ref.Version++ }},
		{"different provider session mapping", func(_ *identity.Identity, binding *auditAuthorityBinding) {
			binding.SessionID = "44444444-4444-4444-8444-444444444444"
		}},
		{"different registry actor", func(_ *identity.Identity, binding *auditAuthorityBinding) { binding.ActorID = "foreign-human" }},
		{"different initiating actor", func(id *identity.Identity, _ *auditAuthorityBinding) { id.Principal.Subject = "foreign-human" }},
	} {
		t.Run(change.name, func(t *testing.T) {
			id, binding := saved, registered
			change.mutate(&id, &binding)
			if auditConsentAuthorityMatches(id, actor, providerSession, binding) {
				t.Fatal("swapped original authority binding must be rejected")
			}
		})
	}
}
