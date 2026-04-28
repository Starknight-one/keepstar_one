// Package usecases — Merge applier Phase D3 (catalog completion 2026-04-28).
//
// ApplyProposals takes approved proposals from a previously-saved merge_report
// and performs the destructive master_products / master_variants /
// catalog.products writes one transaction per proposal. Revert reverses an
// applied report by walking RollbackData and restoring the prior listing FK
// state. Created masters are intentionally left orphaned on revert — other
// proposals in the same report may have linked to them, and curator can
// clean orphan masters separately.
package usecases

import (
	"context"
	"errors"
	"fmt"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/ports"
)

// ProposalEdit lets the curator override fields on a proposal at apply time
// (edit-and-approve in the UI). All fields are optional; non-nil values
// supersede the proposal as-saved.
type ProposalEdit struct {
	// ProposedMaster, when non-nil, replaces the saved ProposedMaster shape.
	// Used by the curator to fix name/brand/tier2 before creating master.
	ProposedMaster *domain.ProposedMaster `json:"proposedMaster,omitempty"`

	// FieldDecisions, when non-empty, replaces the saved FieldDecisions slice.
	// Used by the curator to flip per-field actions (inherit ↔ propagate ↔ skip).
	FieldDecisions []domain.FieldDecision `json:"fieldDecisions,omitempty"`

	// TargetMasterProductID / TargetMasterVariantID let the curator redirect
	// a link_existing or variant_of_existing proposal to a different master
	// than the one cascade picked.
	TargetMasterProductID string `json:"targetMasterProductId,omitempty"`
	TargetMasterVariantID string `json:"targetMasterVariantId,omitempty"`
}

// ApplyProposalsRequest is the wire shape from the curator endpoint.
type ApplyProposalsRequest struct {
	ReportID    int64                   `json:"reportId"`
	ProposalIDs []string                `json:"proposalIds"`        // subset to approve
	Edits       map[string]ProposalEdit `json:"edits,omitempty"`    // proposal_id → edits
	ActorID     string                  `json:"actorId"`            // curator user id (audit)
}

// ProposalFailure is one row in the apply result. Keeps the proposal id +
// the error message so the curator UI can render a per-row outcome.
type ProposalFailure struct {
	ProposalID string `json:"proposalId"`
	ListingID  string `json:"listingId,omitempty"`
	Reason     string `json:"reason"`
}

// ApplyProposalsResult is what we return to the caller.
type ApplyProposalsResult struct {
	ReportID     int64             `json:"reportId"`
	Status       domain.MergeReportStatus `json:"status"`        // applied / partial / pending
	AppliedCount int               `json:"appliedCount"`
	FailedCount  int               `json:"failedCount"`
	SkippedCount int               `json:"skippedCount"`
	Failures     []ProposalFailure `json:"failures,omitempty"`
}

// ApplyProposals walks the requested proposal subset, applies edits on top
// of the saved proposal, calls the appropriate TxPort method per Action,
// captures RollbackData for revert, audits, and persists the new proposal
// statuses. Per-proposal transactions live inside the TxPort — failures
// here only abort the one proposal.
func (uc *MergeApplyUseCase) ApplyProposals(ctx context.Context, req ApplyProposalsRequest) (*ApplyProposalsResult, error) {
	if uc.tx == nil {
		return nil, errors.New("merge_apply: tx port not wired (call WithApplyTx)")
	}
	if req.ReportID == 0 {
		return nil, errors.New("merge_apply: report_id required")
	}
	if len(req.ProposalIDs) == 0 {
		return nil, errors.New("merge_apply: at least one proposal_id required")
	}

	report, err := uc.reports.GetByID(ctx, req.ReportID)
	if err != nil {
		return nil, fmt.Errorf("load report: %w", err)
	}
	if report == nil {
		return nil, errors.New("merge_apply: report not found")
	}
	if report.Status == domain.MergeReportStatusReverted ||
		report.Status == domain.MergeReportStatusSuperseded {
		return nil, fmt.Errorf("merge_apply: report status=%s — cannot apply", report.Status)
	}

	// Build a quick lookup so we can iterate proposals in saved order while
	// still respecting the requested subset.
	want := make(map[string]struct{}, len(req.ProposalIDs))
	for _, id := range req.ProposalIDs {
		want[id] = struct{}{}
	}

	res := &ApplyProposalsResult{ReportID: req.ReportID}
	var failures []ProposalFailure

	for i := range report.Proposals {
		p := &report.Proposals[i]
		if _, ok := want[p.ID]; !ok {
			continue
		}
		// Already applied (idempotency) or terminally rejected → leave alone.
		if p.Status == domain.MergeProposalStatusApplied ||
			p.Status == domain.MergeProposalStatusRejected {
			continue
		}

		// Layer edits on top of the saved proposal. Edits are scoped to one
		// proposal — a copy avoids mutating the saved blob outside the
		// fields the curator explicitly edited.
		edit, hasEdit := req.Edits[p.ID]
		if hasEdit {
			if edit.ProposedMaster != nil {
				p.ProposedMaster = edit.ProposedMaster
			}
			if len(edit.FieldDecisions) > 0 {
				p.FieldDecisions = edit.FieldDecisions
			}
			if edit.TargetMasterProductID != "" {
				p.TargetMasterProductID = edit.TargetMasterProductID
			}
			if edit.TargetMasterVariantID != "" {
				p.TargetMasterVariantID = edit.TargetMasterVariantID
			}
		}

		// Snapshot pre-state — read listing FKs before any write so we can
		// roll back even if the proposal's own RollbackData was empty when
		// generated.
		listing, err := uc.catalog.GetProduct(ctx, report.TenantID, p.ListingID)
		if err != nil || listing == nil {
			failures = append(failures, ProposalFailure{
				ProposalID: p.ID, ListingID: p.ListingID,
				Reason: fmt.Sprintf("load listing: %v", err),
			})
			p.Status = domain.MergeProposalStatusFailed
			res.FailedCount++
			continue
		}
		rollback := map[string]interface{}{
			"listing_master_product_id_before": listing.MasterProductID,
			"listing_master_variant_id_before": listing.MasterVariantID,
		}

		// Defensive: don't write over a listing that's already linked to a
		// master, even if the proposal said otherwise. Re-running apply
		// after curator merged the listing in another way mid-flight should
		// be a no-op rather than a clobber.
		if (listing.MasterProductID != "" || listing.MasterVariantID != "") &&
			p.Action != domain.MergeActionAlreadyLinked {
			p.Status = domain.MergeProposalStatusApplied
			p.RollbackData = rollback
			res.SkippedCount++
			continue
		}

		switch p.Action {
		case domain.MergeActionNewMaster:
			if p.ProposedMaster == nil {
				failures = append(failures, ProposalFailure{
					ProposalID: p.ID, ListingID: p.ListingID,
					Reason: "proposed_master missing for new_master action",
				})
				p.Status = domain.MergeProposalStatusFailed
				res.FailedCount++
				continue
			}
			mpID, mvID, err := uc.tx.ApplyNewMaster(ctx, p.ListingID, p.ProposedMaster)
			if err != nil {
				failures = append(failures, ProposalFailure{
					ProposalID: p.ID, ListingID: p.ListingID,
					Reason: err.Error(),
				})
				p.Status = domain.MergeProposalStatusFailed
				res.FailedCount++
				continue
			}
			rollback["created_master_product_id"] = mpID
			rollback["created_master_variant_id"] = mvID
			p.TargetMasterProductID = mpID
			p.TargetMasterVariantID = mvID
			p.RollbackData = rollback
			p.Status = domain.MergeProposalStatusApplied
			res.AppliedCount++
			uc.logAuditCurator(ctx, report.TenantID, req.ActorID, domain.EntityKindMasterProduct, mpID, domain.AuditActionMerge, p)

		case domain.MergeActionLinkExisting:
			if p.TargetMasterProductID == "" {
				failures = append(failures, ProposalFailure{
					ProposalID: p.ID, ListingID: p.ListingID,
					Reason: "target_master_product_id missing for link_existing",
				})
				p.Status = domain.MergeProposalStatusFailed
				res.FailedCount++
				continue
			}
			if err := uc.tx.ApplyLinkExisting(ctx, p.ListingID, p.TargetMasterProductID, p.TargetMasterVariantID, p.FieldDecisions); err != nil {
				failures = append(failures, ProposalFailure{
					ProposalID: p.ID, ListingID: p.ListingID,
					Reason: err.Error(),
				})
				p.Status = domain.MergeProposalStatusFailed
				res.FailedCount++
				continue
			}
			p.RollbackData = rollback
			p.Status = domain.MergeProposalStatusApplied
			res.AppliedCount++
			uc.logAuditCurator(ctx, report.TenantID, req.ActorID, domain.EntityKindListing, p.ListingID, domain.AuditActionMerge, p)

		case domain.MergeActionVariantOfExisting:
			if p.TargetMasterProductID == "" || p.ProposedVariant == nil {
				failures = append(failures, ProposalFailure{
					ProposalID: p.ID, ListingID: p.ListingID,
					Reason: "target_master_product_id + proposed_variant required for variant_of_existing",
				})
				p.Status = domain.MergeProposalStatusFailed
				res.FailedCount++
				continue
			}
			mvID, err := uc.tx.ApplyVariantOfExisting(ctx, p.ListingID, p.TargetMasterProductID, p.ProposedVariant)
			if err != nil {
				failures = append(failures, ProposalFailure{
					ProposalID: p.ID, ListingID: p.ListingID,
					Reason: err.Error(),
				})
				p.Status = domain.MergeProposalStatusFailed
				res.FailedCount++
				continue
			}
			rollback["created_master_variant_id"] = mvID
			p.TargetMasterVariantID = mvID
			p.RollbackData = rollback
			p.Status = domain.MergeProposalStatusApplied
			res.AppliedCount++
			uc.logAuditCurator(ctx, report.TenantID, req.ActorID, domain.EntityKindMasterVariant, mvID, domain.AuditActionMerge, p)

		case domain.MergeActionAlreadyLinked, domain.MergeActionSkip, domain.MergeActionNeedsReview:
			// No-op proposals — accept but don't touch anything.
			p.Status = domain.MergeProposalStatusApplied
			res.SkippedCount++

		default:
			failures = append(failures, ProposalFailure{
				ProposalID: p.ID, ListingID: p.ListingID,
				Reason: fmt.Sprintf("unknown action: %s", p.Action),
			})
			p.Status = domain.MergeProposalStatusFailed
			res.FailedCount++
		}
	}

	// Decide final report status from per-proposal counts, ignoring the
	// proposals we didn't touch this call.
	finalStatus := uc.computeReportStatus(report)
	counters := countersFromProposals(report.Proposals)
	if err := uc.reports.UpdateProposals(ctx, report.ID, report.Proposals, counters); err != nil {
		return nil, fmt.Errorf("persist updated proposals: %w", err)
	}
	if err := uc.reports.MarkApplied(ctx, report.ID, req.ActorID, finalStatus); err != nil {
		return nil, fmt.Errorf("mark applied: %w", err)
	}

	res.Status = finalStatus
	res.Failures = failures
	uc.log.Info("merge_apply_done",
		"report_id", report.ID, "tenant_id", report.TenantID,
		"applied", res.AppliedCount, "failed", res.FailedCount, "skipped", res.SkippedCount,
		"final_status", string(finalStatus))
	return res, nil
}

// Revert reverses every applied proposal in the report by walking each
// proposal's RollbackData and restoring the listing FKs to the saved
// pre-state. Created master_products / master_variants are intentionally
// left alone — other proposals in the same report may have linked to them,
// and orphan masters can be cleaned up separately. The report is marked
// reverted; applied_at is preserved for audit but status flips.
func (uc *MergeApplyUseCase) Revert(ctx context.Context, reportID int64, actorID string) error {
	if uc.tx == nil {
		return errors.New("merge_apply: tx port not wired (call WithApplyTx)")
	}
	report, err := uc.reports.GetByID(ctx, reportID)
	if err != nil {
		return fmt.Errorf("load report: %w", err)
	}
	if report == nil {
		return errors.New("merge_apply: report not found")
	}
	if report.Status == domain.MergeReportStatusReverted {
		return errors.New("merge_apply: already reverted")
	}
	if report.AppliedAt == nil {
		return errors.New("merge_apply: report was never applied")
	}

	revertedCount := 0
	failedCount := 0
	stillAppliedCount := 0
	for i := range report.Proposals {
		p := &report.Proposals[i]
		if p.Status != domain.MergeProposalStatusApplied {
			continue
		}
		if p.RollbackData == nil {
			// Skipped proposals (already_linked / skip / needs_review) had no
			// writes — leave their status alone.
			continue
		}
		prevMP, _ := p.RollbackData["listing_master_product_id_before"].(string)
		prevMV, _ := p.RollbackData["listing_master_variant_id_before"].(string)

		if err := uc.tx.RestoreListingLink(ctx, p.ListingID, prevMP, prevMV); err != nil {
			uc.log.Warn("merge_revert_failed_proposal",
				"report_id", report.ID, "proposal_id", p.ID,
				"listing_id", p.ListingID, "error", err.Error())
			// Don't bail — keep going so the rest reverts. The proposal
			// stays "applied" so a subsequent revert can retry.
			failedCount++
			stillAppliedCount++
			continue
		}
		// Clear the link target on the proposal so the report state truly
		// reflects "this proposal isn't applied anymore". We keep the
		// RollbackData around for audit / re-revert if needed.
		fc := map[string]domain.FieldChange{}
		if prevMP != "" || prevMV != "" {
			// restoring to a prior (non-null) state — rare but possible
			// for proposals that were applied on top of an already-merged
			// listing.
			fc["master_product_id"] = domain.FieldChange{Old: p.TargetMasterProductID, New: prevMP}
			fc["master_variant_id"] = domain.FieldChange{Old: p.TargetMasterVariantID, New: prevMV}
		} else {
			fc["master_product_id"] = domain.FieldChange{Old: p.TargetMasterProductID, New: nil}
			fc["master_variant_id"] = domain.FieldChange{Old: p.TargetMasterVariantID, New: nil}
		}
		uc.logAuditCuratorRevert(ctx, report.TenantID, actorID, p.ListingID, fc, p.RollbackData)

		p.Status = domain.MergeProposalStatusPending
		revertedCount++
	}

	counters := countersFromProposals(report.Proposals)
	if err := uc.reports.UpdateProposals(ctx, report.ID, report.Proposals, counters); err != nil {
		return fmt.Errorf("persist reverted proposals: %w", err)
	}

	// Pick the report status that matches what actually happened:
	//   - all targets restored cleanly → reverted (the simple case)
	//   - some restored, some failed → partial (curator must re-try the
	//     failed ones; the report is half-rolled-back)
	//   - none reverted (every Restore* errored) → keep applied so the
	//     curator UI doesn't lie about "report cleaned up" when the FKs
	//     are still pointing at created masters.
	finalStatus := domain.MergeReportStatusReverted
	if revertedCount == 0 && failedCount > 0 {
		finalStatus = domain.MergeReportStatusApplied
	} else if stillAppliedCount > 0 {
		finalStatus = domain.MergeReportStatusPartial
	}
	if err := uc.reports.MarkApplied(ctx, report.ID, actorID, finalStatus); err != nil {
		return fmt.Errorf("mark reverted: %w", err)
	}
	uc.log.Info("merge_apply_reverted",
		"report_id", report.ID, "tenant_id", report.TenantID, "actor_id", actorID,
		"reverted", revertedCount, "failed", failedCount, "final_status", string(finalStatus))
	if failedCount > 0 {
		return fmt.Errorf("revert: %d proposal(s) failed to restore (see logs); report status=%s", failedCount, finalStatus)
	}
	return nil
}

// computeReportStatus picks applied / partial / pending / reviewed based on
// the per-proposal statuses currently in the report. The applier may not
// touch every proposal in one call (curator can split approvals across
// multiple sessions), so "any proposal still pending" → status='partial'.
func (uc *MergeApplyUseCase) computeReportStatus(report *domain.MergeReport) domain.MergeReportStatus {
	hasApplied, hasPending, hasFailed := false, false, false
	for _, p := range report.Proposals {
		switch p.Status {
		case domain.MergeProposalStatusApplied:
			hasApplied = true
		case domain.MergeProposalStatusPending, domain.MergeProposalStatusApproved:
			hasPending = true
		case domain.MergeProposalStatusFailed:
			hasFailed = true
		}
	}
	switch {
	case hasApplied && (hasPending || hasFailed):
		return domain.MergeReportStatusPartial
	case hasApplied:
		return domain.MergeReportStatusApplied
	default:
		return domain.MergeReportStatusReviewed
	}
}

// countersFromProposals re-derives the report header counters after the
// applier has potentially flipped per-proposal statuses (e.g. needs_review
// got approved as link_existing). Action counters reflect the action shape,
// not the apply outcome — they're what the curator UI summarizes.
func countersFromProposals(proposals []domain.MergeProposal) ports.MergeReportCounters {
	c := ports.MergeReportCounters{TotalListings: len(proposals)}
	for _, p := range proposals {
		switch p.Action {
		case domain.MergeActionLinkExisting, domain.MergeActionVariantOfExisting:
			c.AutoLinkCount++
		case domain.MergeActionNewMaster:
			c.NewMasterCount++
		case domain.MergeActionNeedsReview:
			c.NeedsReviewCount++
		case domain.MergeActionSkip:
			c.SkipCount++
		case domain.MergeActionAlreadyLinked:
			c.AlreadyLinkedCount++
		}
	}
	return c
}

// logAuditCurator writes one curator-actioned audit row per applied proposal.
// Silently no-ops when the audit port wasn't wired (test setups).
func (uc *MergeApplyUseCase) logAuditCurator(ctx context.Context, tenantID, actorID string, kind domain.EntityKind, entityID string, action domain.AuditAction, p *domain.MergeProposal) {
	if uc.audit == nil {
		return
	}
	fc := map[string]domain.FieldChange{
		"action":   {Old: nil, New: string(p.Action)},
		"listing":  {Old: nil, New: p.ListingID},
	}
	if p.TargetMasterProductID != "" {
		fc["master_product_id"] = domain.FieldChange{Old: nil, New: p.TargetMasterProductID}
	}
	if p.TargetMasterVariantID != "" {
		fc["master_variant_id"] = domain.FieldChange{Old: nil, New: p.TargetMasterVariantID}
	}
	meta := map[string]interface{}{"proposal_id": p.ID}
	if err := uc.audit.LogCurator(ctx, tenantID, actorID, kind, entityID, action, fc, meta); err != nil {
		uc.log.Warn("merge_apply_audit_failed", "proposal_id", p.ID, "error", err.Error())
	}
}

func (uc *MergeApplyUseCase) logAuditCuratorRevert(ctx context.Context, tenantID, actorID, listingID string, fc map[string]domain.FieldChange, rollback map[string]interface{}) {
	if uc.audit == nil {
		return
	}
	meta := map[string]interface{}{"rollback_data": rollback}
	if err := uc.audit.LogCurator(ctx, tenantID, actorID, domain.EntityKindListing, listingID, domain.AuditActionMergeRevert, fc, meta); err != nil {
		uc.log.Warn("merge_revert_audit_failed", "listing_id", listingID, "error", err.Error())
	}
}
