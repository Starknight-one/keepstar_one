package usecases

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/operations/seed"
	"keepstar_v5/internal/ports"
)

// seed_demo_data — the demo-data law (V2_SPEC.md L6 + ruling R3): the
// workspace is seeded with a realistic, business-class starter pack AT
// ASSEMBLY, because without it both surfaces render empty and the L4
// demonstration (book on the storefront → the lead appears in the CRM) has
// nothing to run on.
//
// Two planes, each through the door real data uses:
//
//   - Listings → AdminGatewayPort.StartImport + ImportStatus, byte-identical
//     to what the uploader posts (R7's honest status contract included). The
//     pack renders a CSV with a stable SKU per row, so a retry converges on
//     the same catalog instead of doubling it (R22).
//   - Leads / contacts → EntityPort.CreateRecord, mapped onto whatever
//     entities the conversation actually defined. Re-runs skip records whose
//     demo key is already present.
//
// Everything seeded carries the demo flag (seed.DemoFlagKey) — L6's "purged
// when real data arrives". The purge is NOT built yet; the flag is what
// makes it buildable.
//
// Because the purge does not exist, the step REFUSES to seed a workspace
// whose real data already arrived or is arriving (seedNotNeededReason): the
// upload path auto-applies the manifest (ResolveIngestToken), so without the
// guard a visitor's own listings CSV would trigger 17 fake Rio apartments
// into their catalog seconds before their real rows land — permanently,
// since admin's ingest only adds and nothing reads the demo flag yet. Every
// door that means "real data is on this session" (upload accepted, import
// completed, import failed → re-upload coming) marks a not-yet-run seed step
// skipped: markSeedSkipped, called from the ingest-step mutations and from
// ResolveIngestToken before it auto-applies.
//
// The step applies AFTER define_entity / define_value_set (orderedStepIndices
// puts it in the late group) and BEFORE issue_surface_urls, which refuses
// until every other step has applied (skipped counts as done, so a guarded
// seed never blocks the handover).

// Import polling. Vars, not consts: the timeout test shrinks them so the
// fail-loud path is covered without a 90s test.
var (
	seedImportPollInterval = 500 * time.Millisecond
	seedImportPollTimeout  = 90 * time.Second
)

// applySeedDemoData is the seed_demo_data step executor. Zero-input: the
// pack follows the tenant's own vertical. An unknown business class is NOT
// a failure — no pack ships for it yet, and seeding the wrong world is worse
// than seeding nothing; the step applies with an honest note.
func (ap *ManifestApplier) applySeedDemoData(ctx context.Context, m *domain.OnboardingManifest, st *domain.ManifestStep) (map[string]any, string, error) {
	if err := requireTenant(m); err != nil {
		return nil, "", err
	}
	// Real data beats demo data: never mix fake stock into a catalog the
	// business already filled (there is no purge to undo it).
	if reason := seedNotNeededReason(m); reason != "" {
		ap.log.Info("seed_demo_data skipped — real data on this session",
			"tenant", m.Tenant.Slug, "reason", reason)
		return map[string]any{
			"pack":     "none",
			"listings": 0,
			"records":  0,
			"notes":    []string{reason},
		}, domain.ManifestStepSkipped, nil
	}
	pack, ok := seed.PackForVertical(m.Tenant.Vertical)
	if !ok {
		ap.log.Warn("seed_demo_data: no demo pack for this business class — nothing seeded",
			"tenant", m.Tenant.Slug, "vertical", m.Tenant.Vertical)
		return map[string]any{
			"pack":     "none",
			"listings": 0,
			"records":  0,
			"notes":    []string{"no demo pack ships for business class " + m.Tenant.Vertical},
		}, domain.ManifestStepApplied, nil
	}

	imported, err := ap.importDemoListings(ctx, m.Tenant.Slug, pack)
	if err != nil {
		return nil, "", err
	}
	records, notes, err := ap.seedDemoRecords(ctx, m.Tenant.ID, m.Tenant.Slug, pack)
	if err != nil {
		return nil, "", err
	}

	ap.log.Info("seed_demo_data applied",
		"tenant", m.Tenant.Slug, "pack", pack.Name,
		"listings", imported.Processed, "projectionRows", imported.ProjectionRows,
		"records", records)
	// R7's honest contract, whole: what admin processed, what the projection
	// rebuild actually wrote and what it complained about — the same fields
	// the real upload path records (CompleteIngestStep). "17 processed" with
	// 0 projection rows is a storefront that renders nothing; the step must
	// not hide that behind a listings count.
	out := map[string]any{
		"pack":           pack.Name,
		"listings":       imported.Processed,
		"projectionRows": imported.ProjectionRows,
		"invalidated":    imported.Invalidated,
		"records":        records,
	}
	if len(imported.Errors) > 0 {
		out["errors"] = imported.Errors
		notes = append(notes, fmt.Sprintf("admin reported %d row error(s) on the demo import", len(imported.Errors)))
	}
	if imported.Processed < len(pack.Listings) {
		notes = append(notes, fmt.Sprintf("admin imported %d of the pack's %d listings", imported.Processed, len(pack.Listings)))
	}
	if imported.ProjectionRows == 0 {
		notes = append(notes, "the search projection was not rebuilt for the demo listings — the storefront may render empty")
	}
	if len(notes) > 0 {
		out["notes"] = notes
	}
	return out, domain.ManifestStepApplied, nil
}

// seedNotNeededReason answers "does this workspace still need demo data?" —
// "" means yes, any other string is the human reason it must NOT be seeded,
// recorded on the step. The signal is the session's own ingest door: a
// completed real import (the step is applied), or an upload admin already
// accepted (jobId stamped by RecordUploadJob) whose rows are landing now.
func seedNotNeededReason(m *domain.OnboardingManifest) string {
	for i := range m.Steps {
		st := &m.Steps[i]
		if st.Op != opIssueIngestDoor {
			continue
		}
		if st.Status == domain.ManifestStepApplied {
			if n := intFromResult(st.Result, "processed"); n > 0 {
				return fmt.Sprintf("this workspace already holds real uploaded data (%d rows) — demo data not seeded", n)
			}
			return "this workspace already holds real uploaded data — demo data not seeded"
		}
		if job, _ := st.Result["jobId"].(string); job != "" {
			return fmt.Sprintf("a real data upload is already importing (job %s) — demo data not seeded", job)
		}
	}
	return ""
}

// markSeedSkipped stops a not-yet-run seed_demo_data step from ever firing
// and records WHY on the step. Called from the paths that learn real data is
// on this session BEFORE the applier could reach the seed — an already
// applied or skipped step is left alone (nothing left to prevent), a FAILED
// one is marked skipped too, so a demo-data hiccup stops gating the surface
// URLs the visitor actually came for. Returns whether anything changed.
func markSeedSkipped(m *domain.OnboardingManifest, reason string) bool {
	changed := false
	for i := range m.Steps {
		st := &m.Steps[i]
		if st.Op != opSeedDemoData {
			continue
		}
		if st.Status == domain.ManifestStepApplied || st.Status == domain.ManifestStepSkipped {
			continue
		}
		st.Status = domain.ManifestStepSkipped
		st.Error = ""
		st.Result = map[string]any{
			"pack":     "none",
			"listings": 0,
			"records":  0,
			"notes":    []string{reason},
		}
		changed = true
	}
	return changed
}

// intFromResult reads a number off a step result. Step results round-trip
// through JSONB, so an int written here comes back as a float64.
func intFromResult(result map[string]any, key string) int {
	switch v := result[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

// importDemoListings pushes the pack's catalog rows through the admin import
// — the same route the visitor's own upload takes — and waits for the honest
// terminal status (R7), which it returns whole.
func (ap *ManifestApplier) importDemoListings(ctx context.Context, tenantSlug string, pack seed.DemoPack) (*ports.AdminImportStatus, error) {
	if ap.cfg.Gateway == nil {
		return nil, errors.New("seed_demo_data: admin gateway not wired")
	}
	table, err := pack.ListingsCSV()
	if err != nil {
		return nil, fmt.Errorf("seed_demo_data: render %s pack: %w", pack.Name, err)
	}
	job, err := ap.cfg.Gateway.StartImport(ctx, tenantSlug, pack.FileName, bytes.NewReader(table))
	if err != nil {
		return nil, fmt.Errorf("seed_demo_data: start import: %w", err)
	}
	status, err := ap.awaitImport(ctx, tenantSlug, job.JobID)
	if err != nil {
		return nil, err
	}
	if status.Status == "failed" {
		return nil, fmt.Errorf("seed_demo_data: import failed: %s", strings.Join(status.Errors, "; "))
	}
	if status.Processed == 0 {
		// A completed import that landed nothing leaves the storefront dead —
		// exactly the state L6 exists to prevent. Fail loud; the retry
		// re-imports the same stable SKUs.
		return nil, fmt.Errorf("seed_demo_data: import completed with 0 of %d rows processed (job %s)",
			len(pack.Listings), job.JobID)
	}
	if status.Processed < len(pack.Listings) {
		ap.log.Warn("seed_demo_data: import processed fewer rows than the pack holds",
			"tenant", tenantSlug, "processed", status.Processed, "pack", len(pack.Listings))
	}
	if status.ProjectionRows == 0 {
		ap.log.Warn("seed_demo_data: import wrote no projection rows — the storefront may render empty",
			"tenant", tenantSlug, "job", job.JobID, "processed", status.Processed)
	}
	return status, nil
}

// awaitImport polls the admin job until it reaches a terminal status. A
// deadline (or a cancelled context) is an error, never a silent "probably
// fine" — the applier must not report demo data that may not exist.
func (ap *ManifestApplier) awaitImport(ctx context.Context, tenantSlug, jobID string) (*ports.AdminImportStatus, error) {
	deadline := time.Now().Add(seedImportPollTimeout)
	for {
		status, err := ap.cfg.Gateway.ImportStatus(ctx, tenantSlug, jobID)
		if err != nil {
			return nil, fmt.Errorf("seed_demo_data: import status: %w", err)
		}
		if status.Status == "completed" || status.Status == "failed" {
			return status, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("seed_demo_data: import %s still %s after %s",
				jobID, status.Status, seedImportPollTimeout)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("seed_demo_data: import %s: %w", jobID, ctx.Err())
		case <-time.After(seedImportPollInterval):
		}
	}
}

// seedDemoRecords writes the pack's leads and contacts into the entities the
// conversation defined. Returns how many records were written, honest notes
// about what the tenant does not model, and an error only for real write
// failures (a missing entity is a note, not a halt).
func (ap *ManifestApplier) seedDemoRecords(ctx context.Context, tenantID, tenantSlug string, pack seed.DemoPack) (int, []string, error) {
	if ap.cfg.Entities == nil {
		return 0, nil, errors.New("seed_demo_data: entity port not wired")
	}
	defs, err := ap.cfg.Entities.ListEntityDefinitions(ctx, tenantID)
	if err != nil {
		return 0, nil, fmt.Errorf("seed_demo_data: list entities: %w", err)
	}

	var notes []string
	written := 0

	leadDef := seed.PickEntity(defs, seed.LeadEntityHints)
	if leadDef == nil {
		notes = append(notes, fmt.Sprintf("no lead-like entity defined — %d demo leads not seeded", len(pack.Leads)))
	} else {
		vs := ap.leadValueSet(ctx, tenantID, leadDef)
		now := time.Now()
		rows := make([]map[string]any, 0, len(pack.Leads))
		for _, lead := range pack.Leads {
			rows = append(rows, pack.LeadRecordData(leadDef, vs, lead, now))
		}
		n, err := ap.writeDemoRecords(ctx, tenantID, tenantSlug, leadDef, pack.Name, rows)
		if err != nil {
			return written, notes, err
		}
		written += n
	}

	contactDef := seed.PickEntity(defs, seed.ContactEntityHints)
	if contactDef == nil {
		notes = append(notes, fmt.Sprintf("no contact-like entity defined — %d demo contacts not seeded", len(pack.Contacts)))
	} else if leadDef != nil && contactDef.Slug == leadDef.Slug {
		// One entity answers both vocabularies (a "client" entity holding
		// leads): seeding contacts into it would blur the pipeline.
		notes = append(notes, fmt.Sprintf("contacts share the %q entity with leads — %d demo contacts not seeded", leadDef.Slug, len(pack.Contacts)))
	} else {
		rows := make([]map[string]any, 0, len(pack.Contacts))
		for _, contact := range pack.Contacts {
			rows = append(rows, pack.ContactRecordData(contactDef, contact))
		}
		n, err := ap.writeDemoRecords(ctx, tenantID, tenantSlug, contactDef, pack.Name, rows)
		if err != nil {
			return written, notes, err
		}
		written += n
	}
	return written, notes, nil
}

// leadValueSet resolves the value set the tenant's status field validates
// against; nil when the entity has no status or the set is unreadable (the
// pack then writes its own status vocabulary).
func (ap *ManifestApplier) leadValueSet(ctx context.Context, tenantID string, def *domain.EntityDefinition) *domain.ValueSet {
	_, ref := seed.StatusFieldOf(def)
	if ref == "" {
		return nil
	}
	vs, err := ap.cfg.Entities.GetValueSet(ctx, tenantID, ref)
	if err != nil || vs == nil {
		ap.log.Warn("seed_demo_data: status value set unreadable — seeding the pack's own statuses",
			"entity", def.Slug, "valueSet", ref, "err", err)
		return nil
	}
	return vs
}

// writeDemoRecords creates the rows that are not already there. Idempotency
// (R22): every row carries seed.DemoRecordKey, and the tenant is asked once
// which of those keys already exist — a re-run after a halt writes only the
// remainder, never a second copy.
func (ap *ManifestApplier) writeDemoRecords(ctx context.Context, tenantID, tenantSlug string, def *domain.EntityDefinition, packName string, rows []map[string]any) (int, error) {
	existing, err := ap.existingDemoKeys(ctx, tenantID, def.Slug, packName)
	if err != nil {
		return 0, err
	}
	written := 0
	for _, data := range rows {
		key, _ := data[seed.DemoRecordKey].(string)
		if key != "" && existing[key] {
			continue
		}
		rec := &domain.EntityRecord{
			TenantID:   tenantID,
			EntitySlug: def.Slug,
			Data:       data,
			CreatedBy:  applierActor,
		}
		if _, _, err := ap.cfg.Entities.CreateRecord(ctx, rec); err != nil {
			return written, fmt.Errorf("seed_demo_data: create %s record %s: %w", def.Slug, key, err)
		}
		written++
	}
	ap.log.Info("seed_demo_data: records seeded",
		"tenant", tenantSlug, "entity", def.Slug, "written", written, "skipped", len(rows)-written)
	return written, nil
}

// existingDemoKeys reads the demo keys this pack already wrote into an
// entity (data->>'demoPack' = pack, one flat query — the entity plane has no
// aggregation).
func (ap *ManifestApplier) existingDemoKeys(ctx context.Context, tenantID, entitySlug, packName string) (map[string]bool, error) {
	recs, _, err := ap.cfg.Entities.QueryRecords(ctx, tenantID, entitySlug, domain.RecordFilter{
		Attrs: map[string]string{seed.DemoPackKey: packName},
		Limit: 200,
	})
	if err != nil {
		return nil, fmt.Errorf("seed_demo_data: read seeded %s records: %w", entitySlug, err)
	}
	keys := make(map[string]bool, len(recs))
	for i := range recs {
		if key, _ := recs[i].Data[seed.DemoRecordKey].(string); key != "" {
			keys[key] = true
		}
	}
	return keys, nil
}
