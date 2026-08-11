import { memo, useState } from 'react'
import type { EntryRef, Protocol, SiteView } from '../../api/types'
import { VirtualList } from '../../components/VirtualList'
import { refKey } from '../../lib/keys'
import type { StatsIndex } from '../../state/useStats'
import { SiteMenu, type SiteMenuActions } from './SiteMenu'
import type { MatrixRow } from './visibility'

export const ROW_HEIGHT = 44
export const COLUMN_WIDTH = 120
export const NAME_WIDTH = 300

export type MatrixActions = SiteMenuActions & {
  onToggle: (targets: EntryRef[], currentlyEnabled: boolean) => void
  onSetRow: (row: MatrixRow, disabled: boolean) => void
}

type GridProps = {
  rows: MatrixRow[]
  sites: SiteView[]
  disabledDraft: Record<string, boolean>
  baseDisabled: Set<string>
  priorityDraft: Record<string, number>
  stats: StatsIndex
  busySite: string | null
  actions: MatrixActions
}

export function MatrixGrid({
  rows,
  sites,
  disabledDraft,
  baseDisabled,
  priorityDraft,
  stats,
  busySite,
  actions,
}: GridProps) {
  const width = NAME_WIDTH + sites.length * COLUMN_WIDTH
  const [openMenu, setOpenMenu] = useState<string | null>(null)

  return (
    <VirtualList
      className="card matrix-list"
      items={rows}
      rowHeight={ROW_HEIGHT}
      contentWidth={width}
      rowKey={(row) => row.key}
      header={
        <div className="matrix-row matrix-head" style={{ width }}>
          <div className="cell cell-name">模型（映射后）</div>
          {sites.map((site) => (
            <div className={`cell cell-site ${site.temp ? 'is-temp' : ''}`} key={site.id}>
              <button
                type="button"
                className="site-head"
                title={siteTitle(site)}
                onClick={() => setOpenMenu(openMenu === site.id ? null : site.id)}
              >
                <span className="site-name">{site.name}</span>
                <span className="site-head-line">
                  <span className={`priority-chip ${priorityDraft[site.id] !== undefined ? 'dirty' : ''}`}>
                    {priorityDraft[site.id] ?? site.priority}
                  </span>
                  <SiteHealth site={site} />
                </span>
              </button>
              {openMenu === site.id && (
                <SiteMenu
                  site={site}
                  priority={priorityDraft[site.id] ?? site.priority}
                  busy={busySite === site.id}
                  actions={actions}
                  onClose={() => setOpenMenu(null)}
                />
              )}
            </div>
          ))}
        </div>
      }
      renderRow={(row) => (
        <MatrixRowView
          row={row}
          sites={sites}
          disabledDraft={disabledDraft}
          baseDisabled={baseDisabled}
          stats={stats}
          actions={actions}
          width={width}
        />
      )}
    />
  )
}

/** A dot for the last probe: green recently ok, red failing, grey unknown. */
function SiteHealth({ site }: { site: SiteView }) {
  if (!site.has_key) return <span className="health-dot is-bad" title="CPA 里没有 api-key" />
  if (site.failures && site.failures > 0) {
    return <span className="health-dot is-bad" title={`连续失败 ${site.failures} 次：${site.last_error ?? ''}`} />
  }
  if (site.last_ok_at) return <span className="health-dot is-ok" title={`最后成功 ${site.last_ok_at}`} />
  return <span className="health-dot" title="尚未探测" />
}

const CHANNEL_LABEL: Record<string, string> = {
  openai: 'openai-compatibility',
  codex: 'codex-api-key',
  claude: 'claude-api-key',
}

function siteTitle(site: SiteView): string {
  const lines = [`${site.name}（优先级 ${site.priority}）`, `配置于：${site.channels.map((c) => CHANNEL_LABEL[c] ?? c).join('、')}`]
  if (!site.channels.includes('openai')) {
    lines.push('该站点在 CPA 里没有 openai-compatibility 条目，只能按 base-url 找')
  }
  if (!site.has_key) lines.push('CPA 配置里 api-key 为空')
  lines.push('点击展开站点操作')
  return lines.join('\n')
}

type RowProps = {
  row: MatrixRow
  sites: SiteView[]
  disabledDraft: Record<string, boolean>
  baseDisabled: Set<string>
  stats: StatsIndex
  actions: MatrixActions
  width: number
}

const MatrixRowView = memo(function MatrixRowView({
  row,
  sites,
  disabledDraft,
  baseDisabled,
  stats,
  actions,
  width,
}: RowProps) {
  let on = 0
  let off = 0
  for (const site of sites) {
    const refs = row.cells.get(site.id)
    if (!refs) continue
    if (isEnabled(refs, disabledDraft, baseDisabled)) on++
    else off++
  }

  return (
    <div className={`matrix-row ${on === 0 ? 'is-all-off' : ''}`} style={{ width }}>
      <div className="cell cell-name">
        <ProtocolMark protocols={row.protocols} />
        <span className="mono matrix-name" title={row.name}>
          {row.name}
        </span>
        <span className="matrix-counts muted">
          {on}/{on + off}
        </span>
        <span className="matrix-row-actions">
          <button type="button" className="btn btn-secondary btn-xs" onClick={() => actions.onSetRow(row, false)}>
            全开
          </button>
          <button type="button" className="btn btn-secondary btn-xs" onClick={() => actions.onSetRow(row, true)}>
            全关
          </button>
        </span>
      </div>

      {sites.map((site) => {
        const refs = row.cells.get(site.id)
        if (!refs) {
          return (
            <div className={`cell cell-site is-absent ${site.temp ? 'is-temp' : ''}`} key={site.id}>
              ·
            </div>
          )
        }
        const enabled = isEnabled(refs, disabledDraft, baseDisabled)
        const dirty = refs.some((ref) => disabledDraft[refKey(ref.site, ref.upstream)] !== undefined)
        return (
          <div className={`cell cell-site ${site.temp ? 'is-temp' : ''}`} key={site.id}>
            <button
              type="button"
              className={`toggle ${enabled ? 'is-on' : ''} ${dirty ? 'is-dirty' : ''}`}
              title={refs.map((ref) => ref.upstream).join('\n')}
              onClick={() => actions.onToggle(refs, enabled)}
            />
            <CellStats refs={refs} stats={stats} />
          </div>
        )
      })}
    </div>
  )
})

/**
 * Request outcomes for the models behind this cell.
 *
 * A cell often stands for several upstream models, and they do not all behave
 * the same, so the summary is the total and the tooltip breaks it down per
 * model.
 */
function CellStats({ refs, stats }: { refs: EntryRef[]; stats: StatsIndex }) {
  if (!stats.configured) return null

  let ok = 0
  let failed = 0
  const lines: string[] = []
  for (const ref of refs) {
    const cell = stats.byModel.get(refKey(ref.site, ref.upstream))
    if (!cell) continue
    ok += cell.ok
    failed += cell.failed
    lines.push(`${ref.upstream}：成功 ${cell.ok} / 失败 ${cell.failed}${cell.latency_ms ? ` · ${cell.latency_ms}ms` : ''}`)
  }
  if (ok === 0 && failed === 0) return null

  const tone = failed === 0 ? 'is-ok' : ok === 0 ? 'is-bad' : 'is-mixed'
  return (
    <span className={`cell-stats ${tone}`} title={lines.join('\n')}>
      <span className="cell-stats-ok">{ok}</span>
      <span className="cell-stats-sep">/</span>
      <span className="cell-stats-bad">{failed}</span>
      {refs.length > 1 && <span className="cell-stats-more">▾</span>}
    </span>
  )
}

const PROTOCOL_LABEL: Record<Protocol, string> = {
  openai: 'OA',
  codex: 'CX',
  claude: 'CL',
}

const PROTOCOL_TARGET: Record<Protocol, string> = {
  openai: 'openai-compatibility',
  codex: 'codex-api-key',
  claude: 'claude-api-key',
}

function ProtocolMark({ protocols }: { protocols: Protocol[] }) {
  if (protocols.length === 1) {
    const protocol = protocols[0]
    return (
      <span className={`proto-mark is-${protocol}`} title={`写入 ${PROTOCOL_TARGET[protocol]}`}>
        {PROTOCOL_LABEL[protocol]}
      </span>
    )
  }
  return (
    <span
      className="proto-mark is-mixed"
      title={`这一行的模型分属不同协议，会被写进多张表：${protocols.map((p) => PROTOCOL_TARGET[p]).join('、')}`}
    >
      混
    </span>
  )
}

function isEnabled(refs: EntryRef[], draft: Record<string, boolean>, base: Set<string>): boolean {
  return refs.some((ref) => {
    const key = refKey(ref.site, ref.upstream)
    const override = draft[key]
    return override !== undefined ? !override : !base.has(key)
  })
}
