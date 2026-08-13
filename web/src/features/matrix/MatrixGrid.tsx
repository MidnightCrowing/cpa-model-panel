import { memo, useState } from 'react'
import type { EntryRef, Protocol, SiteView } from '../../api/types'
import { Tooltip } from '../../components/Tooltip'
import { VirtualList } from '../../components/VirtualList'
import { refKey, siteChannelKey } from '../../lib/keys'
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
  /** The CPA list this page is showing; priorities are read and written per
   *  channel, so every site control needs to know which one. */
  protocol: Protocol
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
  protocol,
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
          {sites.map((site) => {
            const draftKey = siteChannelKey(site.id, protocol)
            const priority = priorityDraft[draftKey] ?? site.priorities[protocol] ?? 0
            return (
              <div className={`cell cell-site ${site.temp ? 'is-temp' : ''}`} key={site.id}>
                <Tooltip content={<SiteTip site={site} protocol={protocol} priority={priority} />}>
                  <button
                    type="button"
                    className="site-head"
                    onClick={() => setOpenMenu(openMenu === site.id ? null : site.id)}
                  >
                    <span className="site-name">{site.label || site.name}</span>
                    {site.group && <span className="site-group">{site.group}</span>}
                    <span className="site-head-line">
                      <span className={`priority-chip ${priorityDraft[draftKey] !== undefined ? 'dirty' : ''}`}>
                        {priority}
                      </span>
                      <SiteHealth site={site} />
                    </span>
                  </button>
                </Tooltip>
                {openMenu === site.id && (
                  <SiteMenu
                    site={site}
                    protocol={protocol}
                    priority={priority}
                    busy={busySite === site.id}
                    actions={actions}
                    onClose={() => setOpenMenu(null)}
                  />
                )}
              </div>
            )
          })}
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

const CHANNEL_LABEL: Record<string, string> = {
  openai: 'openai-compatibility',
  codex: 'codex-api-key',
  claude: 'claude-api-key',
}

/** What the old `title` string said, laid out. */
function SiteTip({ site, protocol, priority }: { site: SiteView; protocol: Protocol; priority: number }) {
  return (
    <>
      <div className="tooltip-title">{site.name}</div>
      <div className="tooltip-line">
        {CHANNEL_LABEL[protocol]} 优先级 {priority}
      </div>
      <div className="tooltip-line">
        配置于 {site.channels.map((channel) => CHANNEL_LABEL[channel] ?? channel).join('、') || '—'}
      </div>
      {site.last_error && <div className="tooltip-line">上次探测失败：{site.last_error.slice(0, 80)}</div>}
      <div className="tooltip-line muted">点击展开站点操作</div>
    </>
  )
}

/** A dot for the last probe: green recently ok, red failing, grey unknown. */
function SiteHealth({ site }: { site: SiteView }) {
  if (site.failures && site.failures > 0) return <span className="health-dot is-bad" />
  if (site.last_ok_at) return <span className="health-dot is-ok" />
  return <span className="health-dot" />
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
            <Tooltip content={<CellTip refs={refs} stats={stats} />}>
              <button
                type="button"
                className={`toggle ${enabled ? 'is-on' : ''} ${dirty ? 'is-dirty' : ''}`}
                onClick={() => actions.onToggle(refs, enabled)}
              />
            </Tooltip>
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
 * A cell often stands for several upstream models and they do not all behave
 * the same, so the badge is the total and the tooltip breaks it down per model.
 */
function CellStats({ refs, stats }: { refs: EntryRef[]; stats: StatsIndex }) {
  if (!stats.configured) return null

  const { ok, failed } = totals(refs, stats)
  if (ok === 0 && failed === 0) return null

  const tone = failed === 0 ? 'is-ok' : ok === 0 ? 'is-bad' : 'is-mixed'
  return (
    <Tooltip content={<CellTip refs={refs} stats={stats} />} className={`cell-stats ${tone}`}>
      <span className="cell-stats-ok">{ok}</span>
      <span className="cell-stats-sep">/</span>
      <span className="cell-stats-bad">{failed}</span>
      {refs.length > 1 && <span className="cell-stats-more">▾</span>}
    </Tooltip>
  )
}

/** Model name on the left, its outcome on the right. */
function CellTip({ refs, stats }: { refs: EntryRef[]; stats: StatsIndex }) {
  const rows = refs
    .map((ref) => ({ ref, cell: stats.byModel.get(refKey(ref.site, ref.upstream)) }))
    .filter((row) => row.cell && (row.cell.ok > 0 || row.cell.failed > 0))

  if (rows.length === 0) {
    return (
      <div className="stat-tip">
        {refs.map((ref) => (
          <div className="stat-tip-row" key={refKey(ref.site, ref.upstream)}>
            <span className="stat-tip-name">{ref.upstream}</span>
            <span className="stat-tip-counts muted">无记录</span>
          </div>
        ))}
      </div>
    )
  }

  const { ok, failed } = totals(refs, stats)
  return (
    <div className="stat-tip">
      {rows.map(({ ref, cell }) => (
        <div className="stat-tip-row" key={refKey(ref.site, ref.upstream)}>
          <span className="stat-tip-name">{ref.upstream}</span>
          <span className="stat-tip-counts">
            <span className="stat-tip-ok">{cell!.ok}</span>
            <span className="stat-tip-sep">/</span>
            <span className="stat-tip-bad">{cell!.failed}</span>
            {cell!.latency_ms > 0 && <span className="stat-tip-latency">{formatLatency(cell!.latency_ms)}</span>}
          </span>
        </div>
      ))}
      {rows.length > 1 && (
        <div className="stat-tip-total">
          <span>合计</span>
          <span className="stat-tip-counts">
            <span className="stat-tip-ok">{ok}</span>
            <span className="stat-tip-sep">/</span>
            <span className="stat-tip-bad">{failed}</span>
          </span>
        </div>
      )}
    </div>
  )
}

/**
 * Latency in the largest unit that stays readable.
 *
 * These are averages over a day and a slow model can average tens of seconds,
 * where a raw millisecond count (40745ms) is something you have to stop and
 * count digits on.
 */
function formatLatency(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)}ms`
  if (ms < 60_000) return `${trimZero((ms / 1000).toFixed(1))}s`
  const minutes = Math.floor(ms / 60_000)
  const seconds = Math.round((ms % 60_000) / 1000)
  return seconds === 0 ? `${minutes}m` : `${minutes}m${seconds}s`
}

function trimZero(value: string): string {
  return value.endsWith('.0') ? value.slice(0, -2) : value
}

function totals(refs: EntryRef[], stats: StatsIndex) {  let ok = 0
  let failed = 0
  for (const ref of refs) {
    const cell = stats.byModel.get(refKey(ref.site, ref.upstream))
    if (!cell) continue
    ok += cell.ok
    failed += cell.failed
  }
  return { ok, failed }
}

function isEnabled(refs: EntryRef[], disabledDraft: Record<string, boolean>, baseDisabled: Set<string>): boolean {
  return refs.some((ref) => {
    const key = refKey(ref.site, ref.upstream)
    return !(disabledDraft[key] ?? baseDisabled.has(key))
  })
}
