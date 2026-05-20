-- =========================================================================
-- Pending Approval — bundled catalog flow milestone
-- =========================================================================
-- Plan: /Users/starknight/.claude/plans/immutable-sparking-hejlsberg.md
--
-- This SQL is documentation only. Runtime migrations are executed by
-- project_admin/backend/internal/adapters/postgres/catalog_migrations.go
-- (look for the "pending approval scaffolding" section).
--
-- WHAT THIS DOES
--
-- A. Adds catalog.master_pending_changes — field-level pending changes
--    against existing master_products. Each row is one proposed enrichment
--    (array set-union add OR scalar fill-when-empty). 30-day TTL. Curator
--    approves/rejects per-row or in batches.
--
-- B. Adds approval_status + expiry columns to master_products itself —
--    whole new masters created by first apply for a tenant land as
--    'pending_approval'. Existing rows default to 'approved' (non-disruptive).
--
-- INVARIANTS
--   - master_pending_changes never overwrites master_products directly.
--     Approval is a separate atomic step (handler in curator backend).
--   - approval_status='approved' is the default on INSERT for legacy code
--     paths; apply_v2's CREATE path explicitly sets 'pending_approval'.
--   - Expired rows transition to status='expired' (audit-friendly), not
--     DELETEd. A periodic sweeper does the transition.
-- =========================================================================

-- A. Field-level pending changes
CREATE TABLE IF NOT EXISTS catalog.master_pending_changes (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    master_product_id     UUID NOT NULL REFERENCES catalog.master_products(id) ON DELETE CASCADE,
    op_kind               TEXT NOT NULL CHECK (op_kind IN ('enrich_array_union','enrich_scalar_fill')),
    field_name            TEXT NOT NULL,
    pending_value         JSONB NOT NULL,
    source_inbox_item_id  UUID REFERENCES catalog.inbox_items(id) ON DELETE SET NULL,
    tenant_id             UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
    status                TEXT NOT NULL DEFAULT 'pending'
                          CHECK (status IN ('pending','approved','rejected','expired')),
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at            TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days',
    decided_at            TIMESTAMPTZ,
    decided_by            TEXT,
    decided_reason        TEXT
);

CREATE INDEX IF NOT EXISTS idx_master_pending_changes_master_status
    ON catalog.master_pending_changes(master_product_id, status);
CREATE INDEX IF NOT EXISTS idx_master_pending_changes_tenant_status
    ON catalog.master_pending_changes(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_master_pending_changes_expires
    ON catalog.master_pending_changes(expires_at) WHERE status='pending';

-- B. Whole-master staging flags
ALTER TABLE catalog.master_products
    ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'approved'
        CHECK (approval_status IN ('approved','pending_approval','rejected','expired')),
    ADD COLUMN IF NOT EXISTS pending_approval_expires_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS approved_by TEXT;

CREATE INDEX IF NOT EXISTS idx_master_products_pending_approval
    ON catalog.master_products(approval_status, pending_approval_expires_at)
    WHERE approval_status='pending_approval';
