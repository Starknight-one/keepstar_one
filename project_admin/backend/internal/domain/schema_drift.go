// Package domain — schema drift finding is one row in
// catalog.schema_drift_findings. Produced by the SchemaDrift usecase
// from the worker pool, consumed by the admin "Drift" tab.
package domain

import (
	"encoding/json"
	"time"
)

// SchemaDriftDecision is the LLM classifier's verdict for one unmapped
// inbox field. Matches the CHECK constraint on the table.
type SchemaDriftDecision string

const (
	DriftDecisionTypo  SchemaDriftDecision = "typo_of_existing"
	DriftDecisionAlias SchemaDriftDecision = "alias_of_attribute"
	DriftDecisionNew   SchemaDriftDecision = "new"
)

// SchemaDriftStatus mirrors the CHECK on status column.
type SchemaDriftStatus string

const (
	DriftStatusPending    SchemaDriftStatus = "pending"
	DriftStatusClassified SchemaDriftStatus = "classified"
	DriftStatusApplied    SchemaDriftStatus = "applied"
	DriftStatusDismissed  SchemaDriftStatus = "dismissed"
)

// SchemaDriftFinding mirrors one row of catalog.schema_drift_findings.
// SuggestedAction is opaque JSON the LLM emits — the schema is loosely:
//
//	{kind: "remove_field_mapping"|"add_field_mapping"|"rename"|...,
//	 vertical?, from?, to?, transform?, default?}
//
// Owner-side handler reads it and triggers discovery_v2 in patch mode.
type SchemaDriftFinding struct {
	ID              string
	TenantID        string
	ApplyRunID      string
	FieldName       string
	TypeGuess       string
	Stats           json.RawMessage
	Decision        SchemaDriftDecision
	Confidence      float64
	SuggestedAction json.RawMessage
	Status          SchemaDriftStatus
	CreatedAt       time.Time
	ClassifiedAt    *time.Time
	DecidedAt       *time.Time
}
