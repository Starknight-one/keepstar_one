import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams, Link } from 'react-router-dom'
import { mergeApi } from '../api.js'

// MergeReportPage — proposal review + approve/edit-and-approve/revert flow.
// Phase 4 UI for the D3 backend shipped in commit 6f31393.
//
// Layout follows the Pencil mockup: header with counters, filter toolbar,
// per-proposal expandable cards with action chip + listing name + agent
// confidence + per-field decision table inside expanded view.
export default function MergeReportPage() {
  const { reportId, tenantId } = useParams()
  const navigate = useNavigate()
  const [report, setReport] = useState(null)
  const [error, setError] = useState(null)
  const [loading, setLoading] = useState(true)
  const [filterAction, setFilterAction] = useState('all')
  const [filterConfidence, setFilterConfidence] = useState('all')
  const [search, setSearch] = useState('')
  const [selected, setSelected] = useState({}) // proposalId → bool
  const [edits, setEdits] = useState({})       // proposalId → ProposalEdit
  const [expanded, setExpanded] = useState({}) // proposalId → bool
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    mergeApi.get(reportId)
      .then((r) => { if (!cancelled) { setReport(r); setError(null) } })
      .catch((e) => { if (!cancelled) setError(e.message) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [reportId])

  const proposals = report?.proposals || []
  const visible = useMemo(() => filterProposals(proposals, { filterAction, filterConfidence, search }), [proposals, filterAction, filterConfidence, search])
  const selectedIds = useMemo(() => Object.keys(selected).filter((k) => selected[k]), [selected])

  async function handleApply() {
    if (selectedIds.length === 0) return
    setBusy(true)
    try {
      const res = await mergeApi.apply(reportId, {
        proposalIds: selectedIds,
        edits,
        actorId: '', // backend pulls from X-Curator-User header
      })
      // refresh
      const r = await mergeApi.get(reportId)
      setReport(r)
      setSelected({})
      setEdits({})
      alert(`Applied ${res.appliedCount} · Failed ${res.failedCount} · Skipped ${res.skippedCount}`)
    } catch (e) {
      alert(`Apply failed: ${e.message}`)
    } finally {
      setBusy(false)
    }
  }

  async function handleRevert() {
    if (!confirm('Revert the entire report? Listing FKs will be reset to their pre-apply state. Created masters will stay (orphan-safe).')) return
    setBusy(true)
    try {
      await mergeApi.revert(reportId)
      const r = await mergeApi.get(reportId)
      setReport(r)
      alert('Reverted.')
    } catch (e) {
      alert(`Revert failed: ${e.message}`)
    } finally {
      setBusy(false)
    }
  }

  function selectAllConfident() {
    const next = { ...selected }
    for (const p of proposals) {
      if (p.status === 'pending' && p.matchEvidence?.confidence === 'high') {
        next[p.id] = true
      }
    }
    setSelected(next)
  }

  if (loading) return <div className="merge-page"><div className="loading">Loading…</div></div>
  if (error) return <div className="merge-page"><div className="error">Error: {error}</div></div>
  if (!report) return <div className="merge-page"><div className="error">Report not found</div></div>

  const isApplied = report.status === 'applied' || report.status === 'partial'
  const isReverted = report.status === 'reverted'

  return (
    <div className="merge-page">
      <header className="merge-header">
        <div className="merge-bread">
          <Link to={`/tenants/${report.tenantId}`}>← Back to tenant</Link>
        </div>
        <div className="merge-title-row">
          <h1>Merge report #{report.id}</h1>
          <StatusBadge status={report.status} />
        </div>
        <div className="merge-meta">
          Generated {fmtDate(report.generatedAt)} · {report.totalListings} listings ·
          {' '}<span className="meta-good">{report.autoLinkCount} auto-link</span> ·
          {' '}<span className="meta-accent">{report.newMasterCount} new master</span> ·
          {' '}<span className="meta-review">{report.needsReviewCount} needs review</span> ·
          {' '}<span className="meta-blocked">{report.skipCount} skip</span> ·
          {' '}<span className="meta-muted">{report.alreadyLinkedCount} already linked</span>
        </div>
        {report.agentMetaSummary && <p className="merge-agent-note">{report.agentMetaSummary}</p>}
      </header>

      <div className="merge-toolbar">
        <select value={filterAction} onChange={(e) => setFilterAction(e.target.value)}>
          <option value="all">All actions</option>
          <option value="new_master">New master</option>
          <option value="link_existing">Link existing</option>
          <option value="variant_of_existing">Variant of existing</option>
          <option value="needs_review">Needs review</option>
          <option value="skip">Skip</option>
          <option value="already_linked">Already linked</option>
        </select>
        <select value={filterConfidence} onChange={(e) => setFilterConfidence(e.target.value)}>
          <option value="all">All confidence</option>
          <option value="high">High</option>
          <option value="medium">Medium</option>
          <option value="low">Low</option>
        </select>
        <input
          type="text"
          placeholder="Search by listing name or vendor…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <div className="spacer" />
        {!isApplied && !isReverted && (
          <>
            <button className="btn-secondary" onClick={selectAllConfident} disabled={busy}>Select all high-confidence</button>
            <button className="btn-primary" onClick={handleApply} disabled={busy || selectedIds.length === 0}>
              Apply selected ({selectedIds.length})
            </button>
          </>
        )}
        {isApplied && (
          <button className="btn-danger" onClick={handleRevert} disabled={busy}>Revert report</button>
        )}
      </div>

      <div className="merge-cards">
        {visible.length === 0 && <div className="empty">No proposals match the filter.</div>}
        {visible.map((p) => (
          <ProposalCard
            key={p.id}
            proposal={p}
            edit={edits[p.id]}
            selected={!!selected[p.id]}
            expanded={!!expanded[p.id]}
            disabled={isApplied || isReverted || busy}
            onSelect={(v) => setSelected((s) => ({ ...s, [p.id]: v }))}
            onToggle={() => setExpanded((e) => ({ ...e, [p.id]: !e[p.id] }))}
            onEdit={(e) => setEdits((m) => ({ ...m, [p.id]: e }))}
          />
        ))}
      </div>
    </div>
  )
}

function ProposalCard({ proposal, edit, selected, expanded, disabled, onSelect, onToggle, onEdit }) {
  const action = proposal.action
  const confidence = proposal.matchEvidence?.confidence || (action === 'new_master' ? 'high' : 'low')
  const isApplied = proposal.status === 'applied'
  const isFailed = proposal.status === 'failed'
  return (
    <div className={`proposal-card status-${proposal.status}`}>
      <div className="proposal-row" onClick={onToggle}>
        {!disabled && action !== 'already_linked' && action !== 'skip' && (
          <input
            type="checkbox"
            checked={selected}
            disabled={isApplied}
            onClick={(e) => e.stopPropagation()}
            onChange={(e) => onSelect(e.target.checked)}
          />
        )}
        <ActionChip action={action} />
        <div className="listing-info">
          <div className="listing-name">{proposal.listingName || '(unnamed)'}</div>
          <div className="listing-meta">
            {proposal.listingVendor && <span>{proposal.listingVendor} · </span>}
            {targetSummary(proposal)}
            {proposal.skipReason && <span className="skip-reason"> · {proposal.skipReason}</span>}
          </div>
        </div>
        <div className="spacer" />
        {action !== 'already_linked' && action !== 'skip' && <ConfidenceDot confidence={confidence} />}
        {isApplied && <span className="status-pill status-pill-applied">applied</span>}
        {isFailed && <span className="status-pill status-pill-failed">failed</span>}
        <span className="chev">{expanded ? '▼' : '▶'}</span>
      </div>
      {expanded && (
        <ProposalDetail
          proposal={proposal}
          edit={edit}
          disabled={disabled || isApplied}
          onEdit={onEdit}
        />
      )}
    </div>
  )
}

function ProposalDetail({ proposal, edit, disabled, onEdit }) {
  const merged = mergeProposalAndEdit(proposal, edit)
  function setProposedMasterField(key, value) {
    const pm = { ...(merged.proposedMaster || {}), [key]: value }
    onEdit({ ...edit, proposedMaster: pm })
  }
  function setFieldDecision(idx, patch) {
    const fds = [...(merged.fieldDecisions || [])]
    fds[idx] = { ...fds[idx], ...patch }
    onEdit({ ...edit, fieldDecisions: fds })
  }
  return (
    <div className="proposal-detail">
      {proposal.matchEvidence?.reasoning && (
        <div className="evidence">
          <span className="evidence-label">EVIDENCE</span>
          <span>{proposal.matchEvidence.strategy} · {proposal.matchEvidence.reasoning}</span>
        </div>
      )}

      {proposal.action === 'new_master' && merged.proposedMaster && (
        <div className="proposed-master">
          <div className="section-title">PROPOSED MASTER PRODUCT</div>
          <div className="form-grid">
            <Field label="Name" value={merged.proposedMaster.name} disabled={disabled}
              onChange={(v) => setProposedMasterField('name', v)} />
            <Field label="Brand" value={merged.proposedMaster.brand} disabled={disabled}
              onChange={(v) => setProposedMasterField('brand', v)} />
            <Field label="Vertical" value={merged.proposedMaster.vertical} disabled={disabled}
              onChange={(v) => setProposedMasterField('vertical', v)} />
            <Field label="Description" value={merged.proposedMaster.description} disabled={disabled} multiline
              onChange={(v) => setProposedMasterField('description', v)} />
          </div>
          {merged.proposedMaster.tier2 && Object.keys(merged.proposedMaster.tier2).length > 0 && (
            <div className="tier2">
              <div className="section-title">TIER-2 FIELDS</div>
              <table>
                <tbody>
                  {Object.entries(merged.proposedMaster.tier2).map(([k, v]) => (
                    <tr key={k}><td className="key">{k}</td><td>{String(v)}</td></tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {(proposal.action === 'link_existing' || proposal.action === 'variant_of_existing') && (
        <div className="link-target">
          <div className="section-title">LINK TARGET</div>
          <div className="ids">
            <span className="key">master_product_id</span>
            <code>{merged.targetMasterProductId || '(none)'}</code>
            <span className="key">master_variant_id</span>
            <code>{merged.targetMasterVariantId || '(none)'}</code>
          </div>
        </div>
      )}

      {merged.fieldDecisions && merged.fieldDecisions.length > 0 && (
        <div className="field-decisions">
          <div className="section-title">PER-FIELD DECISIONS</div>
          <table>
            <thead>
              <tr>
                <th>Field</th>
                <th>Master value</th>
                <th>Listing value</th>
                <th>Action</th>
              </tr>
            </thead>
            <tbody>
              {merged.fieldDecisions.map((fd, i) => (
                <tr key={i}>
                  <td className="key">{fd.field}</td>
                  <td>{stringify(fd.masterValue)}</td>
                  <td>{stringify(fd.listingValue)}</td>
                  <td>
                    <select
                      value={fd.action || 'inherit'}
                      disabled={disabled}
                      onChange={(e) => setFieldDecision(i, { action: e.target.value })}
                    >
                      <option value="inherit">inherit master</option>
                      <option value="propagate_to_master">propagate to master</option>
                      <option value="keep_master">keep master</option>
                      <option value="override_listing">override listing</option>
                      <option value="skip">skip</option>
                    </select>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {proposal.rollbackData && (
        <details className="rollback">
          <summary>Rollback snapshot</summary>
          <pre>{JSON.stringify(proposal.rollbackData, null, 2)}</pre>
        </details>
      )}
    </div>
  )
}

function ActionChip({ action }) {
  const map = {
    new_master:           { label: 'NEW MASTER',  cls: 'chip-accent' },
    link_existing:        { label: 'LINK',        cls: 'chip-good' },
    variant_of_existing:  { label: 'VARIANT',     cls: 'chip-good' },
    needs_review:         { label: 'REVIEW',      cls: 'chip-review' },
    skip:                 { label: 'SKIP',        cls: 'chip-blocked' },
    already_linked:       { label: 'LINKED',      cls: 'chip-muted' },
  }
  const m = map[action] || { label: action, cls: 'chip-muted' }
  return <span className={`chip ${m.cls}`}>{m.label}</span>
}

function ConfidenceDot({ confidence }) {
  const map = {
    high:   { label: 'high',   cls: 'good' },
    medium: { label: 'medium', cls: 'review' },
    low:    { label: 'low',    cls: 'blocked' },
  }
  const m = map[confidence] || { label: confidence, cls: 'muted' }
  return (
    <span className={`confidence confidence-${m.cls}`}>
      <span className="dot" /> {m.label}
    </span>
  )
}

function StatusBadge({ status }) {
  const map = {
    pending:    'pending',
    reviewed:   'reviewed',
    applied:    'applied',
    partial:    'partial',
    reverted:   'reverted',
    superseded: 'superseded',
  }
  return <span className={`status-badge status-${status}`}>{map[status] || status}</span>
}

function Field({ label, value, onChange, disabled, multiline }) {
  return (
    <label className="field">
      <span>{label}</span>
      {multiline
        ? <textarea value={value || ''} disabled={disabled} onChange={(e) => onChange(e.target.value)} rows={2} />
        : <input type="text" value={value || ''} disabled={disabled} onChange={(e) => onChange(e.target.value)} />}
    </label>
  )
}

function targetSummary(p) {
  if (p.action === 'new_master' && p.proposedMaster) {
    return `→ ${p.proposedMaster.name} · ${p.proposedMaster.brand || 'unbranded'}`
  }
  if (p.action === 'link_existing' || p.action === 'variant_of_existing') {
    return `→ ${shortenId(p.targetMasterProductId)}${p.targetMasterVariantId ? ' / ' + shortenId(p.targetMasterVariantId) : ''}`
  }
  return ''
}

function shortenId(id) {
  if (!id) return ''
  return id.length > 8 ? id.slice(0, 8) + '…' : id
}

function stringify(v) {
  if (v == null) return ''
  if (typeof v === 'object') return JSON.stringify(v)
  return String(v)
}

function fmtDate(d) {
  if (!d) return ''
  try {
    return new Date(d).toLocaleString()
  } catch {
    return d
  }
}

function mergeProposalAndEdit(p, edit) {
  if (!edit) return p
  return {
    ...p,
    proposedMaster: edit.proposedMaster ?? p.proposedMaster,
    fieldDecisions: edit.fieldDecisions ?? p.fieldDecisions,
    targetMasterProductId: edit.targetMasterProductId ?? p.targetMasterProductId,
    targetMasterVariantId: edit.targetMasterVariantId ?? p.targetMasterVariantId,
  }
}

function filterProposals(proposals, { filterAction, filterConfidence, search }) {
  const q = (search || '').toLowerCase().trim()
  return proposals.filter((p) => {
    if (filterAction !== 'all' && p.action !== filterAction) return false
    if (filterConfidence !== 'all' && (p.matchEvidence?.confidence || '') !== filterConfidence) return false
    if (q) {
      const hay = [p.listingName, p.listingVendor, p.proposedMaster?.name, p.proposedMaster?.brand].filter(Boolean).join(' ').toLowerCase()
      if (!hay.includes(q)) return false
    }
    return true
  })
}
