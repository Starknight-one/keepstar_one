package ports

import (
	"context"

	"keepstar-admin/internal/domain"
)

// MergeApplyTxPort is the destructive write half of the merge-apply pipeline
// (Phase D3, catalog completion 2026-04-28). Each method performs one
// proposal's worth of writes inside a single database transaction so the
// applier can apply hundreds of proposals one-by-one with natural partial-
// apply on failure.
//
// Why this is its own port rather than methods on AdminCatalogPort: the apply
// operations span master_products + master_variants + catalog.products and
// must be atomic per proposal. AdminCatalogPort exposes single-table writes
// that own their own transactions; composing them at the use-case layer would
// either leak pgx transactions through the port boundary or sacrifice
// atomicity. Keeping the multi-table apply behaviour behind one port method
// preserves hexagonal isolation.
type MergeApplyTxPort interface {
	// ApplyNewMaster creates a fresh master_product + master_variant and links
	// the listing to them. Returns the IDs of the newly created rows so the
	// caller can stash them in RollbackData. Whole thing in one tx — partial
	// failure leaves nothing behind.
	ApplyNewMaster(ctx context.Context, listingID string, pm *domain.ProposedMaster) (masterProductID, masterVariantID string, err error)

	// ApplyLinkExisting updates the listing.master_*_id and optionally
	// propagates listing-side fields onto the master_product (one
	// FieldDecision per propagated key). All in one tx.
	ApplyLinkExisting(ctx context.Context, listingID, masterProductID, masterVariantID string, propagateToMaster []domain.FieldDecision) error

	// ApplyVariantOfExisting attaches a new master_variant to an existing
	// master_product and links the listing to it. Returns the new variant id.
	ApplyVariantOfExisting(ctx context.Context, listingID, parentMasterProductID string, pv *domain.ProposedVariant) (masterVariantID string, err error)

	// RestoreListingLink resets the listing's master_*_id back to the values
	// captured before apply. Used by Revert. masterProductID / masterVariantID
	// can be empty strings to clear the FK (the most common revert case for
	// proposals that started as new_master). Done in a single short tx.
	RestoreListingLink(ctx context.Context, listingID, prevMasterProductID, prevMasterVariantID string) error
}
