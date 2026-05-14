// Package domain — inbox is the universal raw-data buffer for tenant catalogs.
// Any source (Shopify, CSV, Sheets, manual) lands here as JSONB; the discovery
// agent reads it via narrow tools, and apply_v2 consumes applied=false rows.
package domain

import (
	"encoding/json"
	"time"
)

// InboxSourceKind matches catalog.inbox_items.source_kind CHECK constraint.
type InboxSourceKind string

const (
	InboxSourceShopify InboxSourceKind = "shopify"
	InboxSourceCSV     InboxSourceKind = "csv"
	InboxSourceSheets  InboxSourceKind = "sheets"
	InboxSourceManual  InboxSourceKind = "manual"
)

// InboxItem mirrors one row of catalog.inbox_items.
type InboxItem struct {
	ID          string
	TenantID    string
	SourceKind  InboxSourceKind
	ExternalID  string
	Raw         json.RawMessage
	PayloadHash string
	FetchedAt   time.Time
	AppliedAt   *time.Time
}

// IsApplied reports whether the item has been processed by apply_v2.
func (i *InboxItem) IsApplied() bool { return i.AppliedAt != nil }

// InboxFieldStats is the compact summary discovery agent receives from the
// FieldStats tool. Keeps token cost low: distinct count, range, a few samples.
type InboxFieldStats struct {
	Field         string    `json:"field"`
	NonNullCount  int       `json:"non_null_count"`
	DistinctCount int       `json:"distinct_count"`
	SampleValues  []string  `json:"sample_values"` // up to N unique stringified values
	MinNumeric    *float64  `json:"min_numeric,omitempty"`
	MaxNumeric    *float64  `json:"max_numeric,omitempty"`
	MinLength     *int      `json:"min_length,omitempty"`
	MaxLength     *int      `json:"max_length,omitempty"`
}
