import { memo } from 'react'
import type { EntryRef } from '../../api/types'
import { MappingInput } from './MappingInput'
import type { NamingRow as RowData } from './grouping'

const EXCLUDED_LABEL: Record<string, string> = {
  manual: '已手动删除',
  whitelist: '白名单未匹配',
  version: '版本淘汰',
}

export type NamingRowActions = {
  onRename: (targets: EntryRef[], value: string) => void
  onExclude: (targets: EntryRef[]) => void
  onRestore: (row: RowData) => void
  onCancelKeep: (targets: EntryRef[]) => void
  onApplySuggestion: (row: RowData) => void
}

type Props = {
  row: RowData
  saving: boolean
  actions: NamingRowActions
}

export const NamingRow = memo(function NamingRow({ row, saving, actions }: Props) {
  const excludedLabel = row.excluded ? EXCLUDED_LABEL[row.excluded] ?? row.excluded : ''
  const state = row.excluded ? 'is-excluded' : row.pendingCount > 0 ? 'is-pending' : ''

  return (
    <div className={`naming-row ${state} ${row.dirty ? 'is-dirty' : ''}`}>
      <div className="cell cell-sites">
        <NameList values={row.siteNames} />
      </div>

      <div className="cell cell-upstream">
        <NameList values={row.upstreams} mono strike={!!row.excluded} />
      </div>

      <div className="cell cell-remap">
        <MappingInput
          rowKey={row.key}
          value={row.mixed ? '' : row.value}
          placeholder={row.mixed ? `${row.targets.length} 个不同的名字` : undefined}
          dirty={row.dirty}
          disabled={saving}
          onCommit={(value) => actions.onRename(row.targets, value)}
        />

        <div className="cell-actions">
          {row.suggested && (
            <button
              type="button"
              className="chip chip-action"
              title={`应用清洗建议：${row.suggested}`}
              onClick={() => actions.onApplySuggestion(row)}
            >
              建议 <span className="mono">{row.suggested}</span>
            </button>
          )}

          {row.excluded ? (
            <>
              <span className="chip chip-danger" title={row.reason}>
                {excludedLabel}
                {row.excludedCount < row.targets.length ? ` ${row.excludedCount}/${row.targets.length}` : ''}
              </span>
              <button type="button" className="btn btn-secondary btn-sm" onClick={() => actions.onRestore(row)}>
                恢复
              </button>
            </>
          ) : (
            <>
              {row.keptCount > 0 && (
                <button
                  type="button"
                  className="chip chip-keep"
                  title="已强制保留，忽略白名单与版本规则。点击取消"
                  onClick={() => actions.onCancelKeep(row.targets)}
                >
                  强制保留
                </button>
              )}
              {row.pendingCount > 0 && <span className="chip chip-new">待写入</span>}
              {row.disabledCount > 0 && (
                <span className="chip" title="在站点启停页被关掉，不会写入 CPA">
                  已停用 {row.disabledCount}
                </span>
              )}
              <button
                type="button"
                className="btn btn-danger btn-sm"
                title="从 CPA 模型列表中删除；仍可在此恢复"
                onClick={() => actions.onExclude(row.targets)}
              >
                删除
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  )
})

/** Every value gets its own line — collapsing them behind a "+3" made the
 *  common case (a model served by several sites) unreadable. */
function NameList({ values, mono, strike }: { values: string[]; mono?: boolean; strike?: boolean }) {
  return (
    <div className={`name-list ${mono ? 'mono' : ''} ${strike ? 'is-strike' : ''}`}>
      {values.map((value) => (
        <div className="name-list-item" key={value} title={value}>
          {value}
        </div>
      ))}
    </div>
  )
}
