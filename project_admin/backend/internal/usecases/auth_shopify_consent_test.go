package usecases

// Pre-launch scenario verification for the Shopify install CONSENT flow.
// These tests document the desired behavior of scenarios 52-55 — all of
// which are CURRENTLY UNIMPLEMENTED security gaps. See
// docs/pre_launch_scenarios.md sec 8 (scenarios 52-55).
//
// Status: ALL TESTS EXPECTED TO FAIL until the consent flow is built.
// The document explicitly flags this as a security vulnerability: today's
// code auto-merges an existing user with any Shopify install bearing the
// same email, allowing an attacker to seize admin access by installing the
// Shopify app under a victim's email.

import (
	"context"
	"strings"
	"testing"

	"keepstar-admin/internal/domain"
)

// TestScenario_052_ShopifyAutoMerge_IsBlocked verifies:
// «Сейчас в коде: существующий user re-use'ится, ему добавляется
// membership на новый tenant, отсылается magic-link. Это уязвимость:
// я могу подставить чужой email в Shopify и попасть в чужую админку.
// Надо переделать на consent flow.» (sec 8, scenario 52)
//
// EXPECTED TO FAIL: scenario 52 NOT implemented. ProvisionShopOwner today
// silently adds a membership to the pre-existing user and sends a magic-
// link. This test asserts the SAFE behavior: when the email belongs to an
// existing account, NO membership is auto-added; instead a pending_claim
// challenge is issued.
func TestScenario_052_ShopifyAutoMerge_IsBlocked(t *testing.T) {
	uc, auth, mem, ch, mail := mkMagicLinkUCWithRecorder()
	// Pre-seed an existing user with another tenant.
	existing, _ := auth.CreateUser(context.Background(), &domain.AdminUser{
		Email: "victim@example.com", TenantID: "t-victim-original",
	})
	_ = mem.Add(context.Background(), existing.ID, "t-victim-original", "owner", "")

	// Attacker installs Shopify app under victim's email.
	uc.ProvisionShopOwner(context.Background(), "tenant-attacker-shop", "victim@example.com")

	// SAFE behavior: membership on the new tenant must NOT auto-exist.
	if _, ok, _ := mem.HasMembership(context.Background(), existing.ID, "tenant-attacker-shop"); ok {
		t.Errorf("SECURITY: membership auto-granted on attacker tenant — consent flow not implemented (scenario 52)")
	}
	// A consent-flow challenge should have been issued instead of a
	// straight magic-link.
	var consentIssued bool
	for _, c := range ch.byHash {
		if strings.Contains(c.Kind, "pending_claim") || strings.Contains(c.Kind, "consent") {
			consentIssued = true
		}
	}
	if !consentIssued {
		t.Errorf("scenario 52: no consent/pending_claim challenge created — auto-merge gap")
	}
	// And NO magic-link should be sent to the victim before they consent.
	for _, m := range mail.sent {
		if m.Kind == "magic_link" {
			t.Errorf("scenario 52: magic-link sent before consent (mail=%+v)", m)
		}
	}
}

// TestScenario_053_PendingClaimChallenge_EmailedToOriginalOwner verifies:
// «Должно быть так: создаётся pending_claim challenge → существующему юзеру
// летит письмо "Кто-то поставил Shopify app foo.myshopify.com под email
// который привязан к твоему аккаунту. Подтвердить / отклонить".»
// (sec 8, scenario 53)
//
// EXPECTED TO FAIL: scenario 53 NOT implemented.
func TestScenario_053_PendingClaimChallenge_EmailedToOriginalOwner(t *testing.T) {
	uc, auth, _, _, mail := mkMagicLinkUCWithRecorder()
	existing, _ := auth.CreateUser(context.Background(), &domain.AdminUser{
		Email: "owner@example.com", TenantID: "t-prior",
	})

	uc.ProvisionShopOwner(context.Background(), "tenant-new-shopify", "owner@example.com")

	// A pending_claim email must have been sent.
	var consentEmail bool
	for _, m := range mail.sent {
		if strings.Contains(m.Kind, "pending_claim") || strings.Contains(m.Kind, "consent") {
			consentEmail = true
			// Email must mention the shop being attached (in subject or body data).
			if m.To != "owner@example.com" {
				t.Errorf("consent email To=%q, want owner@example.com", m.To)
			}
		}
	}
	_ = existing
	if !consentEmail {
		t.Errorf("scenario 53: no consent email sent to original account owner")
	}
}

// TestScenario_054_ConsentApprove_AddsMembership verifies:
// «Юзер кликает "подтвердить" → membership добавляется к существующему юзеру,
// в picker'е появляется новый workspace; magic-link не нужен (он залогинится
// обычным способом).» (sec 8, scenario 54)
//
// EXPECTED TO FAIL: there is no ApprovePendingClaim API yet. This test
// demonstrates the desired surface — once implemented, the flow should be:
// 1. ProvisionShopOwner with existing email issues pending_claim
// 2. Owner clicks "approve" in email → frontend POSTs token+approval
// 3. Backend: membership added, challenge consumed, no magic-link sent.
func TestScenario_054_ConsentApprove_AddsMembership(t *testing.T) {
	// Pending implementation. Document the call shape that the gap would unlock:
	// e.g. uc.ApprovePendingClaim(ctx, claimToken, userID) → (tenantID, error)
	t.Errorf("scenario 54: no ApprovePendingClaim API — consent flow not implemented")
}

// TestScenario_055_ConsentReject_MarksTenantOrphaned verifies:
// «Юзер кликает "отклонить" → tenant помечается отказанным, через cleanup-
// job удаляется. Никто туда залогиниться не может.» (sec 8, scenario 55)
//
// EXPECTED TO FAIL: no RejectPendingClaim API + no orphan-tenant cleanup.
func TestScenario_055_ConsentReject_OrphansTenant(t *testing.T) {
	t.Errorf("scenario 55: no RejectPendingClaim API — orphan-cleanup gap")
}
