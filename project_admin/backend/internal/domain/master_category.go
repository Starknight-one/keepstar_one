package domain

import "time"

// MasterCategory is one node in the global master taxonomy. Curated by Vlad.
// Tree via parent_id. Slug is stable (used in mapping artifacts).
type MasterCategory struct {
	ID        string    `json:"id"`
	ParentID  string    `json:"parentId,omitempty"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	Vertical  string    `json:"vertical"`
	CreatedAt time.Time `json:"createdAt"`
}

// TenantCategoryKind classifies tenant collections. Showcase ("Best Sellers",
// "Featured") and promo ("On Sale 30%") don't map to master taxonomy.
type TenantCategoryKind string

const (
	TenantCategoryKindCategory TenantCategoryKind = "category"
	TenantCategoryKindShowcase TenantCategoryKind = "showcase"
	TenantCategoryKindPromo    TenantCategoryKind = "promo"
)

// TenantCategory is a per-tenant copy of the tenant's collection tree, with
// their names and hierarchy preserved 1:1 from source (Shopify, etc.).
type TenantCategory struct {
	ID         string             `json:"id"`
	TenantID   string             `json:"tenantId"`
	ParentID   string             `json:"parentId,omitempty"`
	ExternalID string             `json:"externalId,omitempty"` // gid://shopify/Collection/123
	Slug       string             `json:"slug"`
	Name       string             `json:"name"`
	Kind       TenantCategoryKind `json:"kind"`
	CreatedAt  time.Time          `json:"createdAt"`
}

// TenantCategoryWithCount augments TenantCategory with the number of listings
// linked through tenant_listing_categories. Used by the editor UI for the
// per-node count badge.
type TenantCategoryWithCount struct {
	TenantCategory
	ProductCount int `json:"productCount"`
}

// CategoryMapping is the N:1 link from a tenant_category to a master_category.
// NULL master_category_id is valid — it means the tenant category is showcase
// or promo and has no master equivalent.
type CategoryMapping struct {
	TenantCategoryID string `json:"tenantCategoryId"`
	MasterCategoryID string `json:"masterCategoryId,omitempty"`
	Confidence       string `json:"confidence"`
	MappedBy         string `json:"mappedBy,omitempty"` // "auto" | "curator" | "tenant"
}
