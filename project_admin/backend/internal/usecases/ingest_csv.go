// Package usecases — CSVIngester reads a CSV (or Google Sheet exported as
// CSV) into the inbox + triggers apply. Header row defines field names;
// each subsequent row becomes one inbox_item with raw JSONB = {col: val}.
package usecases

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
)

type CSVIngester struct {
	inbox        *InboxUseCase
	orchestrator *UpdateOrchestrator
	log          *logger.Logger
}

func NewCSVIngester(inbox *InboxUseCase, orchestrator *UpdateOrchestrator, log *logger.Logger) *CSVIngester {
	return &CSVIngester{inbox: inbox, orchestrator: orchestrator, log: log}
}

// IngestReader parses a CSV (header + rows) and pushes into inbox, then
// runs apply. The first row is treated as headers. Empty headers are
// renamed col_<index>. natural_id_field, when non-empty, names the column
// to use as external_id (e.g. "sku"); when empty, external_id is a
// hash-of-content (see InboxUseCase).
func (i *CSVIngester) IngestReader(ctx context.Context, tenantID string, source domain.InboxSourceKind, r io.Reader, naturalIDField string) (*IngestResult, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("ingest csv: empty tenant_id")
	}
	if source != domain.InboxSourceCSV && source != domain.InboxSourceSheets {
		return nil, fmt.Errorf("ingest csv: source must be csv or sheets, got %s", source)
	}

	rdr := csv.NewReader(r)
	rdr.LazyQuotes = true
	rdr.TrimLeadingSpace = true
	// Allow variable field counts in case sheets have ragged rows.
	rdr.FieldsPerRecord = -1

	header, err := rdr.Read()
	if err != nil {
		return nil, fmt.Errorf("ingest csv: read header: %w", err)
	}
	for idx, h := range header {
		header[idx] = strings.TrimSpace(h)
		if header[idx] == "" {
			header[idx] = fmt.Sprintf("col_%d", idx)
		}
	}

	var rows []CSVRow
	for {
		rec, err := rdr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("ingest csv: read row %d: %w", len(rows)+2, err)
		}
		fields := make(map[string]string, len(header))
		var natural string
		for ci, val := range rec {
			if ci >= len(header) {
				break
			}
			fields[header[ci]] = val
			if header[ci] == naturalIDField {
				natural = strings.TrimSpace(val)
			}
		}
		rows = append(rows, CSVRow{NaturalID: natural, Fields: fields})
	}

	var inboxRes *InboxWriteResult
	switch source {
	case domain.InboxSourceCSV:
		inboxRes, err = i.inbox.WriteFromCSV(ctx, tenantID, rows)
	default:
		inboxRes, err = i.inbox.WriteFromSheets(ctx, tenantID, rows)
	}
	if err != nil {
		return nil, fmt.Errorf("ingest csv: inbox write: %w", err)
	}

	applyRes, err := i.orchestrator.ManualSync(ctx, tenantID)
	if err != nil {
		return &IngestResult{Inbox: inboxRes}, fmt.Errorf("ingest csv: apply: %w", err)
	}
	return &IngestResult{Inbox: inboxRes, Apply: applyRes}, nil
}
