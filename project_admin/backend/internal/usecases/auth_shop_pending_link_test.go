package usecases

// Pre-launch scenario verification for Shopify pending_link path:
// merchant arrives via App Store, shop has NO owner email, so we issue
// a shop_pending_link challenge that they consume after signing in.
// See docs/pre_launch_scenarios.md sec 9 (scenarios 57-60).

import (
	"context"
	"errors"
	"testing"

	"keepstar-admin/internal/domain"
)

// mkMagicLinkUCWithRecorder builds a MagicLinkUseCase wired with fakes that
// expose the post-state we assert against (memberships, challenges).
func mkMagicLinkUCWithRecorder() (*MagicLinkUseCase, *fakeAuth, *fakeMemberships, *fakeChallenges, *fakeMailer) {
	auth := newFakeAuth()
	mem := newFakeMemberships()
	ch := newFakeChallenges()
	mail := &fakeMailer{}
	uc, _, _ := newMagicLinkUC(auth, mem, ch, mail)
	return uc, auth, mem, ch, mail
}

// TestScenario_057_IssuePendingShopLink_CreatesChallenge verifies:
// «Я как merchant ставлю Shopify-приложение, но в shop info customerEmail
// пустой → ProvisionShopOwner не запускается, backend issue'ит
// shop_pending_link challenge с tenant_id в meta.» (sec 9, scenario 57)
func TestScenario_057_IssuePendingShopLink_CreatesChallenge(t *testing.T) {
	uc, _, _, ch, _ := mkMagicLinkUCWithRecorder()

	token, err := uc.IssuePendingShopLink(context.Background(), "tenant-no-email", "noemailshop.myshopify.com")
	if err != nil {
		t.Fatalf("IssuePendingShopLink: %v", err)
	}
	if token == "" {
		t.Error("returned token empty")
	}
	if ch.calls != 1 {
		t.Errorf("challenges created = %d, want 1", ch.calls)
	}
	// Meta should contain tenant_id + shop_domain so Consume can hydrate from it.
	var stored *struct {
		tenantID, shopDomain string
	}
	for _, c := range ch.byHash {
		if c.Kind == "shop_pending_link" {
			tid, _ := c.Meta["tenant_id"].(string)
			sd, _ := c.Meta["shop_domain"].(string)
			stored = &struct{ tenantID, shopDomain string }{tid, sd}
		}
	}
	if stored == nil {
		t.Fatal("no shop_pending_link challenge persisted")
	}
	if stored.tenantID != "tenant-no-email" || stored.shopDomain != "noemailshop.myshopify.com" {
		t.Errorf("meta wrong: %+v", stored)
	}
}

// TestScenario_058_IssuePendingShopLink_RejectsEmptyArgs verifies the
// defensive guard that prevents creating an orphan challenge with missing
// tenant_id or shop_domain — which would later panic at Consume with a
// malformed meta error.
func TestScenario_058_IssuePendingShopLink_RejectsEmptyArgs(t *testing.T) {
	uc, _, _, _, _ := mkMagicLinkUCWithRecorder()
	if _, err := uc.IssuePendingShopLink(context.Background(), "", "shop.myshopify.com"); err == nil {
		t.Error("empty tenant_id must err")
	}
	if _, err := uc.IssuePendingShopLink(context.Background(), "tenant-1", ""); err == nil {
		t.Error("empty shop_domain must err")
	}
}

// TestScenario_059_ConsumePendingShopLink_AttachesTenant verifies:
// «Я как merchant выбираю любой метод входа (например Google) → после
// успешного signin frontend ConsumePendingLink → backend линкует мой user_id
// с tenant_id из challenge meta, добавляет membership.» (sec 9, scenario 60)
//
// Note: scenarios 58/59 in the document describe the redirect URL + UI page
// shown to the merchant — those are frontend concerns. Scenario 60's backend
// behavior is what this test exercises; numbered 059 here for sequential
// readability of the test file.
func TestScenario_059_ConsumePendingShopLink_AttachesTenant(t *testing.T) {
	uc, auth, mem, _, _ := mkMagicLinkUCWithRecorder()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{
		Email: "merchant@example.com", TenantID: "t-pre-existing",
	})

	token, err := uc.IssuePendingShopLink(context.Background(), "tenant-shopify-new", "newshop.myshopify.com")
	if err != nil {
		t.Fatalf("IssuePendingShopLink: %v", err)
	}

	gotTenantID, err := uc.ConsumePendingShopLink(context.Background(), token, user.ID)
	if err != nil {
		t.Fatalf("ConsumePendingShopLink: %v", err)
	}
	if gotTenantID != "tenant-shopify-new" {
		t.Errorf("returned tenant = %q, want tenant-shopify-new", gotTenantID)
	}
	if _, ok, _ := mem.HasMembership(context.Background(), user.ID, "tenant-shopify-new"); !ok {
		t.Errorf("owner membership not granted to user %s on tenant-shopify-new", user.ID)
	}

	// Single-use: re-consume must fail.
	if _, err := uc.ConsumePendingShopLink(context.Background(), token, user.ID); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("reused pending_link: err=%v, want ErrInvalidCredentials", err)
	}
}

// TestScenario_060_OrphanPendingShopLink_StaysUnconsumed verifies:
// «Я как merchant отказываюсь входить и закрываю вкладку — tenant остаётся
// orphan, через cleanup-tenant-stale job (если запущен) удаляется.»
// (sec 9, scenario 61)
//
// The cleanup cron is OUT OF SCOPE for backend tests (known gap noted as ⚠
// in the document). What we verify here: the challenge remains FindActive
// until expiry; nobody has membership on the orphan tenant yet.
func TestScenario_060_OrphanPendingShopLink_StaysUnconsumedUntilExpiry(t *testing.T) {
	uc, _, mem, ch, _ := mkMagicLinkUCWithRecorder()

	_, err := uc.IssuePendingShopLink(context.Background(), "tenant-orphan", "orphan.myshopify.com")
	if err != nil {
		t.Fatalf("IssuePendingShopLink: %v", err)
	}
	if ch.calls != 1 {
		t.Fatalf("challenges=%d, want 1", ch.calls)
	}
	// Nobody has membership yet.
	for k := range mem.rows {
		t.Errorf("unexpected membership row: %q", k)
	}
	// Challenge persists.
	var found bool
	for _, c := range ch.byHash {
		if c.Kind == "shop_pending_link" && c.ConsumedAt == nil {
			found = true
		}
	}
	if !found {
		t.Error("shop_pending_link challenge missing or consumed prematurely")
	}
}

// TestScenario_060b_ConsumeWithUnknownToken_Rejects covers the negative
// edge: a token that was never issued (or already consumed in another
// session) must return ErrInvalidCredentials, not silently succeed.
func TestScenario_060b_ConsumeWithUnknownToken_Rejects(t *testing.T) {
	uc, _, _, _, _ := mkMagicLinkUCWithRecorder()
	_, err := uc.ConsumePendingShopLink(context.Background(), "ghost-token", "user-1")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("unknown token: err=%v, want ErrInvalidCredentials", err)
	}
	if _, err := uc.ConsumePendingShopLink(context.Background(), "", "user-1"); err == nil {
		t.Error("empty token must err")
	}
	if _, err := uc.ConsumePendingShopLink(context.Background(), "tok", ""); err == nil {
		t.Error("empty user_id must err")
	}
}
