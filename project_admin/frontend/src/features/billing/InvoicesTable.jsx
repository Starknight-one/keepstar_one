import Table from '../../shared/ui/Table'
import { formatNumber, formatPeriod } from './plans.js'

const STATUS_CLASS = {
  upcoming: 'status-done',
  paid: 'status-active',
  failed: 'status-out',
}

const STATUS_LABEL = {
  upcoming: 'Upcoming',
  paid: 'Paid',
  failed: 'Failed',
}

const COLUMNS = [
  { key: 'id', label: 'Invoice', render: (r) => <span className="inv-id">{r.id}</span> },
  { key: 'period', label: 'Period', render: (r) => formatPeriod(r.periodStart, r.periodEnd) },
  { key: 'tokens', label: 'Tokens used', render: (r) => formatNumber(r.tokensUsed) },
  {
    key: 'status',
    label: 'Status',
    render: (r) => (
      <span className={`status-badge ${STATUS_CLASS[r.status] || 'status-done'}`}>
        {STATUS_LABEL[r.status] || r.status}
      </span>
    ),
  },
  {
    key: 'action',
    label: '',
    render: (r) => {
      if (r.status === 'upcoming') return <a className="inv-action" href="#view">View</a>
      if (r.pdfUrl) return <a className="inv-action" href={r.pdfUrl} target="_blank" rel="noreferrer">PDF</a>
      return <span className="inv-action inv-action-muted">PDF</span>
    },
  },
]

export default function InvoicesTable({ invoices }) {
  return (
    <section className="invoices">
      <header className="invoices-header">
        <h2 className="invoices-title">Recent invoices</h2>
        <a className="invoices-view-all" href="#all">View all →</a>
      </header>
      <Table columns={COLUMNS} data={invoices} />
    </section>
  )
}
