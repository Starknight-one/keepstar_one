package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"keepstar-admin/internal/domain"
)

type ImportAdapter struct {
	client *Client
}

func NewImportAdapter(client *Client) *ImportAdapter {
	return &ImportAdapter{client: client}
}

func (a *ImportAdapter) CreateImportJob(ctx context.Context, job *domain.ImportJob) (*domain.ImportJob, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.admin.create_import_job")
		defer endSpan()
	}
	errorsJSON, _ := json.Marshal(job.Errors)
	query := `INSERT INTO admin.import_jobs (tenant_id, file_name, status, total_items, errors)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`

	err := a.client.pool.QueryRow(ctx, query,
		job.TenantID, job.FileName, job.Status, job.TotalItems, errorsJSON,
	).Scan(&job.ID, &job.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create import job: %w", err)
	}
	return job, nil
}

func (a *ImportAdapter) GetImportJob(ctx context.Context, tenantID string, jobID string) (*domain.ImportJob, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.admin.get_import_job")
		defer endSpan()
	}
	query := `SELECT id, tenant_id, file_name, status, total_items, processed_items, error_count, errors, created_at, completed_at
		FROM admin.import_jobs WHERE id = $1 AND tenant_id = $2`

	var job domain.ImportJob
	var errorsJSON []byte
	err := a.client.pool.QueryRow(ctx, query, jobID, tenantID).Scan(
		&job.ID, &job.TenantID, &job.FileName, &job.Status,
		&job.TotalItems, &job.ProcessedItems, &job.ErrorCount,
		&errorsJSON, &job.CreatedAt, &job.CompletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrImportNotFound
		}
		return nil, fmt.Errorf("get import job: %w", err)
	}
	if len(errorsJSON) > 0 {
		json.Unmarshal(errorsJSON, &job.Errors)
	}
	return &job, nil
}

func (a *ImportAdapter) ListImportJobs(ctx context.Context, tenantID string, limit int, offset int) ([]domain.ImportJob, int, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.admin.list_import_jobs")
		defer endSpan()
	}
	if limit <= 0 {
		limit = 20
	}

	var total int
	if err := a.client.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM admin.import_jobs WHERE tenant_id = $1`, tenantID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count import jobs: %w", err)
	}

	query := `SELECT id, tenant_id, file_name, status, total_items, processed_items, error_count, errors, created_at, completed_at
		FROM admin.import_jobs WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	rows, err := a.client.pool.Query(ctx, query, tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list import jobs: %w", err)
	}
	defer rows.Close()

	var jobs []domain.ImportJob
	for rows.Next() {
		var job domain.ImportJob
		var errorsJSON []byte
		if err := rows.Scan(
			&job.ID, &job.TenantID, &job.FileName, &job.Status,
			&job.TotalItems, &job.ProcessedItems, &job.ErrorCount,
			&errorsJSON, &job.CreatedAt, &job.CompletedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan import job: %w", err)
		}
		if len(errorsJSON) > 0 {
			json.Unmarshal(errorsJSON, &job.Errors)
		}
		jobs = append(jobs, job)
	}
	return jobs, total, nil
}

func (a *ImportAdapter) UpdateImportJobProgress(ctx context.Context, jobID string, processed int, status domain.ImportStatus) error {
	_, err := a.client.pool.Exec(ctx,
		`UPDATE admin.import_jobs SET processed_items = $1, status = $2, updated_at = NOW() WHERE id = $3`,
		processed, status, jobID)
	if err != nil {
		return fmt.Errorf("update import progress: %w", err)
	}
	return nil
}

func (a *ImportAdapter) AppendImportError(ctx context.Context, jobID string, errMsg string) error {
	_, err := a.client.pool.Exec(ctx,
		`UPDATE admin.import_jobs SET errors = errors || $1::jsonb, error_count = error_count + 1, updated_at = NOW() WHERE id = $2`,
		fmt.Sprintf(`[%q]`, errMsg), jobID)
	if err != nil {
		return fmt.Errorf("append import error: %w", err)
	}
	return nil
}

func (a *ImportAdapter) CompleteImportJob(ctx context.Context, jobID string, status domain.ImportStatus, processed int, errorCount int) error {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.admin.complete_import_job")
		defer endSpan()
	}
	_, err := a.client.pool.Exec(ctx,
		`UPDATE admin.import_jobs SET status = $1, processed_items = $2, error_count = $3, completed_at = NOW(), updated_at = NOW() WHERE id = $4`,
		status, processed, errorCount, jobID)
	if err != nil {
		return fmt.Errorf("complete import job: %w", err)
	}
	return nil
}
