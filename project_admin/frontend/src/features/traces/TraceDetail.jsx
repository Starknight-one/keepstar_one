import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { api } from '../../shared/api/apiClient.js'
import Spinner from '../../shared/ui/Spinner.jsx'
import { Section, MetricCard, Tag, JsonBlock, TokensBar, WaterfallBar } from './TraceComponents.jsx'
import './traces.css'

export default function TraceDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [trace, setTrace] = useState(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.get(`/traces/${id}`)
      .then((data) => setTrace(data))
      .catch(() => navigate('/traces'))
      .finally(() => setLoading(false))
  }, [id, navigate])

  if (loading) return <div className="center-spinner"><Spinner /></div>
  if (!trace) return null

  const td = typeof trace.traceData === 'string' ? JSON.parse(trace.traceData) : trace.traceData
  const agent1 = td?.agent1
  const agent2 = td?.agent2
  const stateAfter = td?.stateAfterAgent1
  const formation = td?.formationResult
  const spans = td?.spans

  return (
    <div className="trace-detail">
      <button className="btn btn-secondary trace-back" onClick={() => {
        if (trace.sessionId) {
          navigate(`/traces/session/${trace.sessionId}`)
        } else {
          navigate('/traces')
        }
      }}>
        ← Back
      </button>

      {/* Hero summary */}
      <div className="trace-hero">
        <div className="trace-hero-query">"{trace.query}"</div>
        <div className="trace-hero-metrics">
          <MetricCard label="Total" value={`${trace.totalMs}ms`} />
          <MetricCard label="Cost" value={`$${trace.costUsd?.toFixed(4)}`} />
          <MetricCard label="Agent1" value={agent1 ? `${agent1.totalMs}ms` : '—'} sub={agent1?.toolName} />
          <MetricCard label="Agent2" value={agent2 ? `${agent2.totalMs}ms` : '—'} sub={agent2?.toolName} />
          <MetricCard label="Formation" value={formation?.mode} sub={`${formation?.widgetCount || 0} widgets`} />
        </div>
        {trace.error && <div className="trace-hero-error">{trace.error}</div>}
        <div className="trace-hero-meta">
          <span>Session: <code>{trace.sessionId?.substring(0, 8)}</code></span>
          <span>{new Date(trace.timestamp).toLocaleString()}</span>
        </div>
      </div>

      {/* Waterfall */}
      {spans && spans.length > 0 && (
        <Section title="Waterfall" color="purple" defaultOpen={true}>
          <WaterfallBar spans={spans} totalMs={trace.totalMs} />
        </Section>
      )}

      {/* Agent 1 */}
      <Section
        title="Agent 1 — NLU / Data"
        badge={agent1?.model?.replace('claude-', '').replace('-20251001', '')}
        color="blue"
        defaultOpen={true}
      >
        <div className="trace-metrics-row">
          <MetricCard label="LLM" value={`${agent1?.llmMs}ms`} />
          <MetricCard label="Tool" value={agent1?.toolMs ? `${agent1.toolMs}ms` : '—'} />
          <MetricCard label="Total" value={`${agent1?.totalMs}ms`} />
          <MetricCard label="Prompt" value={agent1?.systemPromptChars ? `${agent1.systemPromptChars}c` : '—'} />
          <MetricCard label="Messages" value={agent1?.messageCount} />
          <MetricCard label="Tools" value={agent1?.toolDefCount} />
        </div>
        <TokensBar agent={agent1} />

        {agent1?.toolName && (
          <div className="trace-tool-call">
            <div className="trace-tool-name">
              <Tag color="blue">{agent1.toolName}</Tag>
              {agent1?.stopReason && <Tag>{agent1.stopReason}</Tag>}
            </div>
          </div>
        )}

        <JsonBlock data={agent1?.toolInput} label="Tool Input" />
        <JsonBlock data={agent1?.toolResult} label="Tool Result" />
        <JsonBlock data={agent1?.toolBreakdown} label="Tool Breakdown" />
        <JsonBlock data={agent1?.enrichedQuery} label="Enriched Query" />
        <JsonBlock data={agent1?.systemPrompt} label="System Prompt" />
      </Section>

      {/* State snapshot */}
      <Section
        title="State after Agent 1"
        badge={`${stateAfter?.productCount || 0}p / ${stateAfter?.serviceCount || 0}s`}
        color="gray"
      >
        <div className="trace-state-info">
          {stateAfter?.fields && stateAfter.fields.length > 0 && (
            <div className="trace-fields-list">
              <span className="trace-fields-label">Fields:</span>
              {stateAfter.fields.map((f, i) => <Tag key={i}>{f}</Tag>)}
            </div>
          )}
        </div>
        {stateAfter?.deltas && stateAfter.deltas.length > 0 && (
          <table className="trace-table">
            <thead>
              <tr>
                <th>Step</th>
                <th>Actor</th>
                <th>Type</th>
                <th>Path</th>
                <th>Tool</th>
                <th>Count</th>
              </tr>
            </thead>
            <tbody>
              {stateAfter.deltas.map((d, i) => (
                <tr key={i}>
                  <td>{d.step}</td>
                  <td><code>{d.actorId}</code></td>
                  <td><Tag color={d.deltaType === 'add' ? 'green' : d.deltaType === 'remove' ? 'red' : 'gray'}>{d.deltaType}</Tag></td>
                  <td><code>{d.path}</code></td>
                  <td>{d.tool ? <code>{d.tool}</code> : '—'}</td>
                  <td>{d.count || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Section>

      {/* Agent 2 */}
      <Section
        title="Agent 2 — Render"
        badge={agent2?.model?.replace('claude-', '').replace('-20251001', '')}
        color="purple"
        defaultOpen={true}
      >
        <div className="trace-metrics-row">
          <MetricCard label="LLM" value={`${agent2?.llmMs}ms`} />
          <MetricCard label="Total" value={`${agent2?.totalMs}ms`} />
          <MetricCard label="Prompt" value={agent2?.systemPromptChars ? `${agent2.systemPromptChars}c` : '—'} />
          <MetricCard label="Messages" value={agent2?.messageCount} />
          <MetricCard label="Tools" value={agent2?.toolDefCount} />
        </div>
        <TokensBar agent={agent2} />

        {agent2?.toolName && (
          <div className="trace-tool-call">
            <Tag color="purple">{agent2.toolName}</Tag>
          </div>
        )}

        <JsonBlock data={agent2?.toolInput} label="Tool Input (visual_assembly params)" defaultExpanded={true} />
        <JsonBlock data={agent2?.toolBreakdown} label="Engine Breakdown" defaultExpanded={true} />
        <JsonBlock data={agent2?.promptSent} label="Prompt Sent to Agent 2" />
        <JsonBlock data={agent2?.rawResponse} label="Raw Response" />
        <JsonBlock data={agent2?.systemPrompt} label="System Prompt" />
      </Section>

      {/* Formation */}
      <Section
        title="Formation Result"
        badge={formation?.mode}
        color="green"
        defaultOpen={true}
      >
        <div className="trace-metrics-row">
          <MetricCard label="Mode" value={formation?.mode} />
          <MetricCard label="Widgets" value={formation?.widgetCount} />
          {formation?.cols > 0 && <MetricCard label="Cols" value={formation?.cols} />}
          <MetricCard label="First" value={formation?.firstWidget?.substring(0, 30)} />
        </div>

        {formation?.widgets && formation.widgets.length > 0 && (
          <table className="trace-table">
            <thead>
              <tr>
                <th>#</th>
                <th>Template</th>
                <th>Size</th>
                <th>Atoms</th>
                <th>Entity</th>
              </tr>
            </thead>
            <tbody>
              {formation.widgets.slice(0, 20).map((w, i) => (
                <tr key={i}>
                  <td className="trace-mono">{i + 1}</td>
                  <td>{w.template || '—'}</td>
                  <td><Tag>{w.size || '—'}</Tag></td>
                  <td>{w.atomCount}</td>
                  <td><code>{w.entityRef ? w.entityRef.substring(0, 12) : '—'}</code></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {formation?.widgets && formation.widgets.length > 20 && (
          <div className="trace-truncated">Showing 20 of {formation.widgets.length} widgets</div>
        )}
        <JsonBlock data={formation?.fullJson} label="Full Formation JSON" />
      </Section>
    </div>
  )
}
