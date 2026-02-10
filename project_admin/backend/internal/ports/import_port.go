package ports

import (
	"context"

	"keepstar-admin/internal/domain"
)

type ImportPort interface {
	CreateImportJob(ctx context.Context, job *domain.ImportJob) (*domain.ImportJob, error)
	GetImportJob(ctx context.Context, tenantID string, jobID string) (*domain.ImportJob, error)
	ListImportJobs(ctx context.Context, tenantID string, limit int, offset int) ([]domain.ImportJob, int, error)
	UpdateImportJobProgress(ctx context.Context, jobID string, processed int, status domain.ImportStatus) error
	AppendImportError(ctx context.Context, jobID string, errMsg string) error
	CompleteImportJob(ctx context.Context, jobID string, status domain.ImportStatus, processed int, errorCount int) error
}
