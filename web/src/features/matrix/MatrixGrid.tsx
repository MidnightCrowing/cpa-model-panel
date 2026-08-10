import { memo } from 'react'
import type { EntryRef, SiteView } from '../../api/types'
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
            <div className="cell cell-site" key={site.id} title={`${site.name}（优先级 ${site.priority}）`}>
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

function isEnabled(refs: EntryRef[], draft: Record<string, boolean>, base: Set<string>): boolean {
  return refs.some((ref) => {
    const key = refKey(ref.site, ref.upstream)
    const override = draft[key]
    return override !== undefined ? !override : !base.has(key)
  })
}
