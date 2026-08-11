import { memo } from 'react'
import type { EntryRef, Protocol, SiteView } from '../../api/types'
import { VirtualList } from '../../components/VirtualList'
import { refKey } from '../../lib/keys'
import type { MatrixRow } from './visibility'

export const ROW_HEIGHT = 44
export const COLUMN_WIDTH = 120
export const NAME_WIDTH = 300

export type MatrixActions = {
  onToggle: (targets: EntryRef[], disabled: boolean) => void
  onSetRow: (row: MatrixRow, disabled: boolean) => void
  onEditPriority: (site: SiteView) => void
}

type GridProps = {
  rows: MatrixRow[]
  sites: SiteView[]
  disabledDraft: Record<string, boolean>
  baseDisabled: Set<string>
  actions: MatrixActions
}

export function MatrixGrid({ rows, sites, disabledDraft, baseDisabled, actions }: GridProps) {
  const width = NAME_WIDTH + sites.length * COLUMN_WIDTH

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
            <div className="cell cell-site" key={site.id} title={siteTitle(site)}>
              <span className="site-name">{site.name}</span>
              <button
                type="button"
                className="priority-chip"
                title="修改站点优先级：数值越大越靠前"
                onClick={() => actions.onEditPriority(site)}
              >
                {site.priority}
              </button>
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
          actions={actions}
          width={width}
        />
      )}
    />
  )
}

type RowProps = {
  row: MatrixRow
  sites: SiteView[]
  disabledDraft: Record<string, boolean>
  baseDisabled: Set<string>
  actions: MatrixActions
  width: number
}

const MatrixRowView = memo(function MatrixRowView({ row, sites, disabledDraft, baseDisabled, actions, width }: RowProps) {
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
            <div className="cell cell-site is-absent" key={site.id}>
              ·
            </div>
          )
        }
        const enabled = isEnabled(refs, disabledDraft, baseDisabled)
        const dirty = refs.some((ref) => disabledDraft[refKey(ref.site, ref.upstream)] !== undefined)
        return (
          <div className="cell cell-site" key={site.id}>
            <button
              type="button"
              className={`toggle ${enabled ? 'is-on' : ''} ${dirty ? 'is-dirty' : ''}`}
              title={refs.map((ref) => ref.upstream).join('\n')}
              onClick={() => actions.onToggle(refs, enabled)}
            />
          </div>
        )
      })}
    </div>
  )
})

const CHANNEL_LIST: Record<string, string> = {
  openai: 'openai-compatibility',
  codex: 'codex-api-key',
  claude: 'claude-api-key',
}

/** Which CPA lists this site is configured in. codex-api-key and
 *  claude-api-key entries carry no name at all, so a site that lives only
 *  there is impossible to find in CPA by name — worth saying out loud. */
function siteTitle(site: SiteView): string {
  const lists = site.channels.map((channel) => CHANNEL_LIST[channel] ?? channel).join('、')
  const lines = [`${site.name}（优先级 ${site.priority}）`, `配置于：${lists}`]
  if (!site.channels.includes('openai')) {
    lines.push('该站点在 CPA 里没有 openai-compatibility 条目，只能按 base-url 找')
  }
  return lines.join('\n')
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

/** Which CPA list this row is written to. */
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
