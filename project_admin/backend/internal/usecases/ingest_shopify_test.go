package usecases

import (
	"context"
	"encoding/json"
	"testing"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// fakeIntegrations is a minimal IntegrationsPort: lookup by shop domain and
// status flips. Other methods panic so unintended use surfaces loudly.
type fakeIntegrations struct {
	byShopDomain  map[string]*domain.Integration
	statusUpdates []statusUpdateCall
}

type statusUpdateCall struct {
	id        string
	status    domain.IntegrationStatus
	lastError string
}

func newFakeIntegrations() *fakeIntegrations {
	return &fakeIntegrations{byShopDomain: map[string]*domain.Integration{}}
}

func (f *fakeIntegrations) Create(_ context.Context, _ *domain.Integration) (*domain.Integration, error) {
	panic("fakeIntegrations.Create not implemented")
}
func (f *fakeIntegrations) Get(_ context.Context, _, _ string) (*domain.Integration, error) {
	panic("fakeIntegrations.Get not implemented")
}
func (f *fakeIntegrations) GetByKindAndExternalID(_ context.Context, _ string, _ domain.IntegrationKind, _ string) (*domain.Integration, error) {
	panic("fakeIntegrations.GetByKindAndExternalID not implemented")
}
func (f *fakeIntegrations) GetByShopDomain(_ context.Context, shopDomain string) (*domain.Integration, error) {
	if i, ok := f.byShopDomain[shopDomain]; ok {
		return i, nil
	}
	return nil, nil
}
func (f *fakeIntegrations) ListByTenant(_ context.Context, _ string) ([]domain.Integration, error) {
	panic("fakeIntegrations.ListByTenant not implemented")
}
func (f *fakeIntegrations) Update(_ context.Context, _ *domain.Integration) error {
	panic("fakeIntegrations.Update not implemented")
}
func (f *fakeIntegrations) UpdateStatus(_ context.Context, id string, status domain.IntegrationStatus, lastError string) error {
	f.statusUpdates = append(f.statusUpdates, statusUpdateCall{id, status, lastError})
	return nil
}
func (f *fakeIntegrations) UpdateLastSync(_ context.Context, _, _ string) error {
	panic("fakeIntegrations.UpdateLastSync not implemented")
}
func (f *fakeIntegrations) Delete(_ context.Context, _, _ string) error {
	panic("fakeIntegrations.Delete not implemented")
}
func (f *fakeIntegrations) CreateOAuthState(_ context.Context, _ *domain.OAuthState) error {
	panic("fakeIntegrations.CreateOAuthState not implemented")
}
func (f *fakeIntegrations) ConsumeOAuthState(_ context.Context, _ string) (*domain.OAuthState, error) {
	panic("fakeIntegrations.ConsumeOAuthState not implemented")
}
func (f *fakeIntegrations) CleanupExpiredOAuthStates(_ context.Context) (int64, error) {
	panic("fakeIntegrations.CleanupExpiredOAuthStates not implemented")
}

// mkSoftDeleteIngester wires a minimal ShopifyIngester just enough to exercise
// SoftDeleteListing. inbox/orchestrator are nil because the soft-delete path
// only touches the writer + action_log.
func mkSoftDeleteIngester() (*ShopifyIngester, *fakeWriter, *fakeActionLog) {
	w := newFakeWriter()
	log := &fakeActionLog{}
	return NewShopifyIngester(nil, nil, w, log, logger.New("error")), w, log
}

// TestScenario_155_WebhookDelete_DispatchesToSoftDeleteListing verifies:
// «products/delete webhook → handler verify'ит HMAC → вызывает
// inboxIngester.SoftDeleteListing(tenant, source_system, source_id)».
// Unit slice: assert SoftDeleteListing → writer.SoftDeleteListing routes with
// source='shopify' and the expected GID.
func TestScenario_155_WebhookDelete_DispatchesToSoftDeleteListing(t *testing.T) {
	ing, writer, log := mkSoftDeleteIngester()
	if err := ing.SoftDeleteListing(context.Background(), "t-1", "gid://shopify/Product/9"); err != nil {
		t.Fatalf("SoftDeleteListing: %v", err)
	}
	if len(writer.softDeleteCalls) != 1 {
		t.Fatalf("writer.softDeleteCalls = %d, want 1", len(writer.softDeleteCalls))
	}
	c := writer.softDeleteCalls[0]
	if c.tenantID != "t-1" || c.source != "shopify" || c.sourceID != "gid://shopify/Product/9" {
		t.Errorf("call args = %+v, want tenant=t-1 source=shopify gid=...Product/9", c)
	}
	if !hasAction(log.entries, "webhook_received") {
		t.Error("webhook_received entry not written on delete")
	}
}

// TestScenario_156_SoftDelete_StampsDeletedAtOnListing_MasterUntouched verifies:
// «SoftDeleteListing стампит catalog.products.deleted_at для matching listing;
// master_products row НЕ трогается».
// Unit slice: the master row in writer.masters is pre-seeded; after
// SoftDeleteListing it must be unchanged. (The actual deleted_at SQL stamp is
// adapter-level — covered by integration tests.)
func TestScenario_156_SoftDelete_StampsDeletedAtOnListing_MasterUntouched(t *testing.T) {
	ing, writer, _ := mkSoftDeleteIngester()
	// Pre-seed a master that some other tenant might also reference.
	writer.bySKU["SHARED-X"] = "master-shared"
	writer.masters["master-shared"] = &ports.MasterProductUpsert{
		SKU:      "SHARED-X",
		Name:     "Shared Master",
		Brand:    "Shared Brand",
		Vertical: "cosmetics",
	}
	mpBefore := *writer.masters["master-shared"]

	if err := ing.SoftDeleteListing(context.Background(), "t-1", "gid://shopify/Product/1"); err != nil {
		t.Fatalf("SoftDeleteListing: %v", err)
	}
	mpAfter := writer.masters["master-shared"]
	if mpAfter.Name != mpBefore.Name || mpAfter.Brand != mpBefore.Brand || mpAfter.SKU != mpBefore.SKU || mpAfter.Vertical != mpBefore.Vertical {
		t.Errorf("master row mutated on listing soft-delete: before=%+v after=%+v", mpBefore, *mpAfter)
	}
}

// TestScenario_159_WebhookDelete_Idempotent_NoOpOnSecondCall verifies:
// «Delete webhook идемпотентный — повторный вызов на уже-удалённый listing — no-op».
// Unit slice: repeated calls do not error; writer just records each dispatch
// (real adapter UPDATE ... SET deleted_at=NOW() is idempotent at the SQL level).
func TestScenario_159_WebhookDelete_Idempotent_NoOpOnSecondCall(t *testing.T) {
	ing, writer, _ := mkSoftDeleteIngester()
	for i := 0; i < 3; i++ {
		if err := ing.SoftDeleteListing(context.Background(), "t-1", "gid://shopify/Product/1"); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}
	if len(writer.softDeleteCalls) != 3 {
		t.Fatalf("softDeleteCalls = %d, want 3 (all idempotent at usecase level)", len(writer.softDeleteCalls))
	}
}

// TestScenario_174_AppUninstalled_StampsIntegrationDisconnected verifies:
// «Я как merchant disconnect'ю Shopify через app/uninstalled webhook →
// shopify_integrations.disconnected_at стампится».
// Unit slice: ShopifyV2UseCase.HandleWebhook(topic="app/uninstalled") should
// call IntegrationsPort.UpdateStatus(id, Disconnected, "app uninstalled").
func TestScenario_174_AppUninstalled_StampsIntegrationDisconnected(t *testing.T) {
	integrations := newFakeIntegrations()
	integrations.byShopDomain["shop.myshopify.com"] = &domain.Integration{
		ID:       "int-1",
		TenantID: "t-1",
		Kind:     domain.IntegrationKindShopify,
		Status:   domain.IntegrationStatusConnected,
	}

	uc := NewShopifyV2UseCase(nil, integrations, nil, "https://admin.example.com", logger.New("error"))
	// HandleWebhook guards on inboxIngester != nil before the topic switch.
	// We don't exercise the ingester for app/uninstalled, but a non-nil stub
	// is required to reach the switch statement.
	uc.SetInboxIngester(&ShopifyIngester{})

	body, _ := json.Marshal(map[string]any{"shop_domain": "shop.myshopify.com"})
	if err := uc.HandleWebhook(context.Background(), "app/uninstalled", "shop.myshopify.com", body); err != nil {
		t.Fatalf("HandleWebhook: %v", err)
	}
	if len(integrations.statusUpdates) != 1 {
		t.Fatalf("statusUpdates = %d, want 1", len(integrations.statusUpdates))
	}
	upd := integrations.statusUpdates[0]
	if upd.id != "int-1" {
		t.Errorf("id = %q, want int-1", upd.id)
	}
	if upd.status != domain.IntegrationStatusDisconnected {
		t.Errorf("status = %q, want disconnected", upd.status)
	}
	if upd.lastError != "app uninstalled" {
		t.Errorf("lastError = %q, want \"app uninstalled\"", upd.lastError)
	}
}
