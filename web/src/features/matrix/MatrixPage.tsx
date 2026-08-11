import { useMemo, useState } from 'react'
import type { EntryRef, ModelView, Protocol, SiteView, View } from '../../api/types'
import { Checkbox, Segmented } from '../../components/Controls'
import { FilterMenu } from '../../components/FilterMenu'
import { useDebounced, usePersistentState } from '../../lib/hooks'
import { refKey } from '../../lib/keys'
import { UNKNOWN_VENDOR, VENDORS, vendorOf } from '../../lib/vendor'
import type { Draft, DraftAction } from '../../state/useDraft'
import { MatrixGrid } from './MatrixGrid'
import { buildMatrix, type MatrixRow } from './visibility'
import { effectiveExcluded } from '../naming/grouping'

type Props = {
  view: View
  draft: Draft
  dispatch: (action: DraftAction) => void
}

type ProtocolFilter = Protocol | 'all'

export function MatrixPage({ view, draft, dispatch }: Props) {
  const [rawQuery, setRawQuery] = useState('')
  const query = useDebounced(rawQuery)
  const [protocol, setProtocol] = usePersistentState<ProtocolFilter>('panel.matrix.protocol', 'all')
  const [vendors, setVendors] = usePersistentState<string[]>('panel.matrix.vendors', [])
  const [sites, setSites] = usePersistentState<string[]>('panel.matrix.sites', [])
  const [hideEmptyRows, setHideEmptyRows] = usePersistentState('panel.matrix.hideRows', true)
  const [hideEmptyColumns, setHideEmptyColumns] = usePersistentState('panel.matrix.hideCols', true)

  const vendorOfModel = useMemo(() => {
    const cache = new Map<string, string>()
    return (model: ModelView) => {
      let vendor = cache.get(model.upstream)
      if (vendor === undefined) {
        vendor = vendorOf(model.upstream, model.alias)
        cache.set(model.upstream, vendor)
      }
      return vendor
    }
  }, [])

  const vendorOptions = useMemo(() => {
    const present = new Set(view.models.map(vendorOfModel))
    const known = VENDORS.filter((vendor) => present.has(vendor))
    return present.has(UNKNOWN_VENDOR) ? [...known, UNKNOWN_VENDOR] : known
  }, [view.models, vendorOfModel])

  const siteOptions = useMemo(() => view.sites.map((site) => site.name), [view.sites])

  // Excluded models never appear here: they are deleted from CPA's list.
  const models = useMemo(() => {
    const needle = query.trim().toLowerCase()
    const vendorSet = new Set(vendors)
    return view.models.filter((model) => {
      if (effectiveExcluded(model, draft)) return false
      if (protocol !== 'all' && model.protocol !== protocol) return false
      if (vendorSet.size > 0 && !vendorSet.has(vendorOfModel(model))) return false
      if (!needle) return true
      const alias = draft.renames[refKey(model.site, model.upstream)] ?? model.alias
      return model.upstream.toLowerCase().includes(needle) || (alias || '').toLowerCase().includes(needle)
    })
  }, [draft, protocol, query, vendorOfModel, vendors, view.models])

  const selectedSites = useMemo(() => {
    if (sites.length === 0) return view.sites
    const chosen = new Set(sites)
    return view.sites.filter((site) => chosen.has(site.name))
  }, [sites, view.sites])

  const layout = useMemo(
    () =>
      buildMatrix({
        models,
        sites: selectedSites,
        renames: draft.renames,
        hideEmptyRows,
        hideEmptyColumns,
      }),
    [draft.renames, hideEmptyColumns, hideEmptyRows, models, selectedSites],
  )

  const baseDisabled = useMemo(() => {
    const set = new Set<string>()
    for (const model of view.models) {
      if (model.disabled) set.add(refKey(model.site, model.upstream))
    }
    return set
  }, [view.models])

  const actions = useMemo(
    () => ({
      onToggle: (targets: EntryRef[], enabled: boolean) =>
        dispatch({ kind: 'disable', targets, disabled: enabled }),
      onSetRow: (row: MatrixRow, disabled: boolean) => {
        const visible = new Set(layout.sites.map((site) => site.id))
        const targets = row.refs.filter((ref) => visible.has(ref.site))
        dispatch({ kind: 'disable', targets, disabled })
      },
      onEditPriority: (site: SiteView) => {
        const current = draft.priorities[site.id] ?? site.priority
        const input = window.prompt(`设置「${site.name}」的优先级（整数，越大越靠前）：`, String(current))
        if (input === null) return
        const value = Number.parseInt(input.trim(), 10)
        if (Number.isNaN(value)) return
        dispatch({ kind: 'priority', site: site.id, priority: value })
      },
    }),
    [dispatch, draft.priorities, layout.sites],
  )

  // Priority edits reorder the columns immediately, without a round trip.
  const orderedSites = useMemo(() => {
    const withDraft = layout.sites.map((site) => ({
      ...site,
      priority: draft.priorities[site.id] ?? site.priority,
    }))
    return withDraft.sort((a, b) =>
      a.priority !== b.priority ? b.priority - a.priority : a.name.localeCompare(b.name, 'zh-Hans-CN'),
    )
  }, [draft.priorities, layout.sites])

  return (
    <>
      <div className="toolbar">
        <input
          className="input search"
          type="search"
          placeholder="搜索模型名…"
          value={rawQuery}
          onChange={(event) => setRawQuery(event.target.value)}
        />
        <Segmented
          label="协议"
          value={protocol}
          onChange={setProtocol}
          options={[
            { value: 'all', label: '全部', title: '显示全部，每行左侧的角标标明它属于哪张 CPA 表' },
            { value: 'openai', label: 'OpenAI', title: '只看写入 openai-compatibility 的模型' },
            { value: 'codex', label: 'Codex', title: '只看写入 codex-api-key 的模型' },
            { value: 'claude', label: 'Claude', title: '只看写入 claude-api-key 的模型' },
          ]}
        />
        <FilterMenu label="供应商" allLabel="所有供应商" options={vendorOptions} selected={vendors} onChange={setVendors} />
        <FilterMenu label="站点" allLabel="所有站点" options={siteOptions} selected={sites} onChange={setSites} />
        <Checkbox
          checked={hideEmptyRows}
          onChange={setHideEmptyRows}
          label="隐藏空行"
          title="当前可见站点都不提供的模型不显示"
        />
        <Checkbox
          checked={hideEmptyColumns}
          onChange={setHideEmptyColumns}
          label="隐藏空列"
          title="不提供任何当前可见模型的站点不显示"
        />
        <span className="muted toolbar-status">
          {layout.rows.length} 行 · {orderedSites.length} 站点
          {layout.hiddenRows > 0 ? ` · 隐藏 ${layout.hiddenRows} 行` : ''}
          {layout.hiddenColumns > 0 ? ` · 隐藏 ${layout.hiddenColumns} 列` : ''}
        </span>
      </div>

      <MatrixGrid
        rows={layout.rows}
        sites={orderedSites}
        disabledDraft={draft.disabled}
        baseDisabled={baseDisabled}
        actions={actions}
      />
    </>
  )
}
