package domain

import "time"

type ImportStatus string

const (
	ImportStatusPending    ImportStatus = "pending"
	ImportStatusProcessing ImportStatus = "processing"
	ImportStatusCompleted  ImportStatus = "completed"
	ImportStatusFailed     ImportStatus = "failed"
)

type ImportJob struct {
	ID             string       `json:"id"`
	TenantID       string       `json:"tenantId"`
	FileName       string       `json:"fileName"`
	Status         ImportStatus `json:"status"`
	TotalItems     int          `json:"totalItems"`
	ProcessedItems int          `json:"processedItems"`
	ErrorCount     int          `json:"errorCount"`
	Errors         []string     `json:"errors,omitempty"`
	CreatedAt      time.Time    `json:"createdAt"`
	CompletedAt    *time.Time   `json:"completedAt,omitempty"`
}
