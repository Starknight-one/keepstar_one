// Package domain — minimal value types for curator. Mirrors a small subset
// of admin/domain (AttributeCandidate, JunkCandidate, AuditEntry) — we only
// need the read-shape for curator UI; admin owns the canonical types.
package domain

import "time"

type CuratorUser struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
}

type AttributeCandidate struct {
	ID               string    `json:"id"`
	Key              string    `json:"key"`
	Vertical         string    `json:"vertical"`
	SeenInTenants    int       `json:"seenInTenants"`
	SampleValues     []string  `json:"sampleValues"`
	ProposedType     string    `json:"proposedType,omitempty"`
	AgentMeta        string    `json:"agentMeta,omitempty"`
	Status           string    `json:"status"`
	PromotedToColumn string    `json:"promotedToColumn,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
}

type CategoryCandidate struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	ProposedParent string    `json:"proposedParent,omitempty"`
	SeenInTenants  int       `json:"seenInTenants"`
	Vertical       string    `json:"vertical"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"createdAt"`
}

type JunkCandidate struct {
	ID             string                 `json:"id"`
	TenantID       string                 `json:"tenantId"`
	ListingID      string                 `json:"listingId"`
	DetectedReason map[string]interface{} `json:"detectedReason"`
	Classification string                 `json:"classification"`
	ClassifiedBy   string                 `json:"classifiedBy,omitempty"`
	CreatedAt      time.Time              `json:"createdAt"`
}

type AuditEntry struct {
	ID            int64                  `json:"id"`
	TenantID      string                 `json:"tenantId,omitempty"`
	ActorKind     string                 `json:"actorKind"`
	ActorID       string                 `json:"actorId,omitempty"`
	EntityKind    string                 `json:"entityKind"`
	EntityID      string                 `json:"entityId"`
	Action        string                 `json:"action"`
	FieldChanges  map[string]interface{} `json:"fieldChanges,omitempty"`
	AggregateMeta map[string]interface{} `json:"aggregateMeta,omitempty"`
	CreatedAt     time.Time              `json:"createdAt"`
}

// IntegrationSummary — read-only view of admin.tenant_integrations for curator.
type IntegrationSummary struct {
	Kind        string     `json:"kind"`
	Status      string     `json:"status"`
	DisplayName string     `json:"displayName,omitempty"`
	ExternalID  string     `json:"externalId,omitempty"`
	LastSyncAt  *time.Time `json:"lastSyncAt,omitempty"`
}

// TenantSummary — row in /curator/tenants list.
type TenantSummary struct {
	ID                  string               `json:"id"`
	Slug                string               `json:"slug"`
	Name                string               `json:"name"`
	Type                string               `json:"type,omitempty"`
	ProductsCount       int                  `json:"productsCount"`
	MasterLinkedCount   int                  `json:"masterLinkedCount"`
	MasterLinkCoverage  float64              `json:"masterLinkCoverage"` // 0..1
	Integrations        []IntegrationSummary `json:"integrations"`
	SchemaStatus        string               `json:"schemaStatus,omitempty"` // tenant_catalog_schema.status
	SchemaVersion       int                  `json:"schemaVersion,omitempty"`
	CreatedAt           time.Time            `json:"createdAt"`
}

// TenantProductRow — light row for tenant catalog browse in curator.
type TenantProductRow struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"` // COALESCE(display_name, original_name, name)
	OriginalName  string  `json:"originalName,omitempty"`
	SKU           string  `json:"sku,omitempty"`
	Brand         string  `json:"brand,omitempty"`
	PriceCents    int     `json:"priceCents"`
	Currency      string  `json:"currency,omitempty"`
	Stock         int     `json:"stock"`
	ImageURL      string  `json:"imageUrl,omitempty"`
	HasMasterLink bool    `json:"hasMasterLink"`
	MasterID      string  `json:"masterId,omitempty"` // master_variant_id OR master_product_id
	SourceSystem  string  `json:"sourceSystem,omitempty"`
}

// MasterProductSummary — row in master catalog browse.
type MasterProductSummary struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Brand           string `json:"brand,omitempty"`
	Vertical        string `json:"vertical"`
	HasEmbedding    bool   `json:"hasEmbedding"`
	HasPIM          bool   `json:"hasPIM"`
	HasDescription  bool   `json:"hasDescription"`
	ListingCount    int    `json:"listingCount"`
	OwnerTenantSlug string `json:"ownerTenantSlug,omitempty"`
	ImageURL        string `json:"imageUrl,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
}

// MasterVariantSummary — variant under a master_product.
type MasterVariantSummary struct {
	ID         string   `json:"id"`
	SKU        string   `json:"sku,omitempty"`
	GTINs      []string `json:"gtins,omitempty"`
	Color      string   `json:"color,omitempty"`
	Size       string   `json:"size,omitempty"`
	Material   string   `json:"material,omitempty"`
	WeightG    int      `json:"weightG,omitempty"`
	VolumeML   int      `json:"volumeMl,omitempty"`
	ImageURL   string   `json:"imageUrl,omitempty"`
}

// MasterProductDetail — full master view + variants + linked listings.
type MasterProductDetail struct {
	MasterProductSummary
	Description    string                 `json:"description,omitempty"`
	PIMFields      map[string]interface{} `json:"pimFields,omitempty"`
	Variants       []MasterVariantSummary `json:"variants"`
	LinkedListings []TenantProductRow     `json:"linkedListings"`
}

// TenantSchemaSummary — read-only view of catalog.tenant_catalog_schema.
type TenantSchemaSummary struct {
	TenantID         string                 `json:"tenantId"`
	Status           string                 `json:"status,omitempty"`
	ArtifactVersion  int                    `json:"artifactVersion,omitempty"`
	DiscoveredAt     *time.Time             `json:"discoveredAt,omitempty"`
	ValidatedAt      *time.Time             `json:"validatedAt,omitempty"`
	MappingArtifact  map[string]interface{} `json:"mappingArtifact,omitempty"`
	ValidationReport map[string]interface{} `json:"validationReport,omitempty"`
}
