import { useState } from 'react'
import { Section, MetricCard, Tag, JsonBlock, TokensBar, WaterfallBar, CacheStatus, TokenCostTable, StateDetails } from './TraceComponents.jsx'

export default function TraceInline({ trace }) {
  const [open, setOpen] = useState(false)

  const td = typeof trace.traceData === 'string' ? JSON.parse(trace.traceData) : trace.traceData
  const agent1 = td?.agent1
  const agent2 = td?.agent2
  const stateAfter = td?.stateAfterAgent1
  const stateAfter2 = td?.stateAfterAgent2
  const formation = td?.formationResult
  const spans = td?.spans

  const summaryParts = [
    `${trace.totalMs}ms`,
    `$${trace.costUsd?.toFixed(4)}`,
    formation?.mode,
    formation?.widgetCount != null ? `${formation.widgetCount} widgets` : null,
  ].filter(Boolean)

  return (
    <div className="trace-inline">
      <div className="trace-inline-toggle" onClick={() => setOpen(!open)}>
        <span className="trace-inline-arrow">{open ? '▾' : '▸'}</span>
        <span className="trace-inline-label">Trace</span>
        <span className="trace-inline-summary">{summaryParts.join(' · ')}</span>
        {trace.error && <span className="trace-inline-error">ERR</span>}
      </div>

      {open && (
        <div className="trace-inline-body">
          <Section
            title="Агент 1 — Анализ запроса"
            badge={agent1?.model?.replace('claude-', '').replace('-20251001', '')}
            color="blue"
          >
            <div className="trace-metrics-row">
              <MetricCard label="LLM" value={agent1?.llmMs != null ? `${agent1.llmMs}ms` : '—'} />
              <MetricCard label="Tool" value={agent1?.toolMs ? `${agent1.toolMs}ms` : '—'} />
              <MetricCard label="Total" value={agent1?.totalMs != null ? `${agent1.totalMs}ms` : '—'} />
            </div>
            <CacheStatus agent={agent1} />
            <TokensBar agent={agent1} />
            <TokenCostTable agent={agent1} />
            {agent1?.toolName && (
              <div className="trace-tool-call">
                <Tag color="blue">{agent1.toolName}</Tag>
                {agent1.stopReason && <Tag color="gray">{agent1.stopReason}</Tag>}
              </div>
            )}
            <JsonBlock data={agent1?.enrichedQuery} label="Enriched Query" />
            <JsonBlock data={agent1?.toolInput} label="Tool Input" />
            <JsonBlock data={agent1?.toolResult} label="Tool Result" />
            <JsonBlock data={agent1?.toolBreakdown} label="Tool Breakdown" />
          </Section>

          <Section
            title="Стейт после Агента 1"
            badge={`${stateAfter?.productCount || 0}p / ${stateAfter?.serviceCount || 0}s`}
            color="gray"
          >
            <StateDetails snapshot={stateAfter} />
            {stateAfter?.deltas && stateAfter.deltas.length > 0 && (
              <table className="trace-table">
                <thead>
                  <tr><th>Step</th><th>Actor</th><th>Type</th><th>Path</th><th>Tool</th><th>Count</th></tr>
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

          <Section
            title="Агент 2 — Рендеринг"
            badge={agent2?.model?.replace('claude-', '').replace('-20251001', '')}
            color="purple"
          >
            <div className="trace-metrics-row">
              <MetricCard label="LLM" value={agent2?.llmMs != null ? `${agent2.llmMs}ms` : '—'} />
              <MetricCard label="Total" value={agent2?.totalMs != null ? `${agent2.totalMs}ms` : '—'} />
            </div>
            <CacheStatus agent={agent2} />
            <TokensBar agent={agent2} />
            <TokenCostTable agent={agent2} />
            {agent2?.toolName && (
              <div className="trace-tool-call">
                <Tag color="purple">{agent2.toolName}</Tag>
              </div>
            )}
            <JsonBlock data={agent2?.promptSent} label="Prompt Sent" />
            <JsonBlock data={agent2?.toolInput} label="Tool Input" defaultExpanded={true} />
            <JsonBlock data={agent2?.toolBreakdown} label="Engine Breakdown" />
            <JsonBlock data={agent2?.rawResponse} label="Raw Response" />
          </Section>

          {stateAfter2 && (
            <Section
              title="Стейт после Агента 2"
              badge={`${stateAfter2?.productCount || 0}p / ${stateAfter2?.serviceCount || 0}s`}
              color="gray"
            >
              <StateDetails snapshot={stateAfter2} />
              {stateAfter2?.deltas && stateAfter2.deltas.length > 0 && (
                <table className="trace-table">
                  <thead>
                    <tr><th>Step</th><th>Actor</th><th>Type</th><th>Path</th><th>Tool</th><th>Count</th></tr>
                  </thead>
                  <tbody>
                    {stateAfter2.deltas.map((d, i) => (
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
          )}

          <Section
            title="Formation Result"
            badge={formation?.mode}
            color="green"
          >
            <div className="trace-metrics-row">
              <MetricCard label="Mode" value={formation?.mode} />
              <MetricCard label="Widgets" value={formation?.widgetCount} />
              {formation?.cols > 0 && <MetricCard label="Cols" value={formation?.cols} />}
            </div>
            {formation?.widgets && formation.widgets.length > 0 && (
              <table className="trace-table">
                <thead>
                  <tr><th>#</th><th>Template</th><th>Size</th><th>Atoms</th><th>Entity</th></tr>
                </thead>
                <tbody>
                  {formation.widgets.slice(0, 10).map((w, i) => (
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
            <JsonBlock data={formation?.fullJson} label="Full Formation JSON" />
          </Section>

          {spans && spans.length > 0 && (
            <Section title="Waterfall (тайминг)" color="purple">
              <WaterfallBar spans={spans} totalMs={trace.totalMs} />
            </Section>
          )}
        </div>
      )}
    </div>
  )
}
