// Package domain — read-only views of the master catalog surfaced to the
// discovery_v2 LLM agent (master-side tools). Kept compact on purpose:
// every byte returned eats agent context, so digests must be aggregates,
// not raw rows.
package domain

// MasterCategoryNode is one node in the master taxonomy. Returned by the
// list_master_categories tool. ParentID is empty for root nodes.
type MasterCategoryNode struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	ParentID      string `json:"parent_id,omitempty"`
	Vertical      string `json:"vertical"`
	ProductCount  int    `json:"product_count"`
}

// MasterCategoryDigest is the compact summary of one master category.
// Returned by digest_master_category. Helps the agent understand what
// data shape already lives in this category so it can mirror the structure
// when mapping inbox fields.
type MasterCategoryDigest struct {
	CategoryID    string   `json:"category_id"`
	CategoryName  string   `json:"category_name"`
	Vertical      string   `json:"vertical"`
	ProductCount  int      `json:"product_count"`
	TopBrands     []string `json:"top_brands"`     // up to 10 by frequency
	SampleNames   []string `json:"sample_names"`   // up to 10 most recent
}

// MasterRef is the minimal envelope returned by find_master_by_sku /
// find_master_by_gtin. Carries just enough for the agent to decide
// whether a row in the inbox should bind to an existing master.
type MasterRef struct {
	ID       string `json:"id"`
	SKU      string `json:"sku"`
	Name     string `json:"name"`
	Brand    string `json:"brand,omitempty"`
	Vertical string `json:"vertical"`
	GTIN     string `json:"gtin,omitempty"`
}
