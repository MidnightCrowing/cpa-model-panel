import { useCallback, useMemo, useState } from 'react'
import type { EntryRef, ModelView, Protocol, View } from '../../api/types'
import { FilterMenu } from '../../components/FilterMenu'
import { Segmented } from '../../components/Controls'
import { VirtualList } from '../../components/VirtualList'
import { useDebounced, usePersistentState } from '../../lib/hooks'
import { UNKNOWN_VENDOR, VENDORS, vendorOf } from '../../lib/vendor'
import type { Draft, DraftAction } from '../../state/useDraft'
import { NamingRow } from './NamingRow'
import {
  AGGREGATION_OPTIONS,
  buildNamingRows,
  effectiveExcluded,
  effectiveName,
  rowHeightOf,
  type Aggregation,
  type NamingRow as RowData,
} from './grouping'

type Props = {
  view: View
  draft: Draft
  dispatch: (action: DraftAction) => void
  saving: boolean
}

type ProtocolFilter = Protocol | 'all'

/** What to do with models the rules removed: show them alongside the rest,
 *  hide them to concentrate on naming, or look at nothing else. */
type ExcludedView = 'all' | 'hide' | 'only'

const EXCLUDED_VIEW_OPTIONS: Array<{ value: ExcludedView; label: string; title: string }> = [
  { value: 'all', label: '全部', title: '被白名单 / 版本规则 / 手动删除排除的模型也一并显示' },
  { value: 'hide', label: '不看已排除', title: '隐藏被白名单、版本规则和手动删除排除的模型' },
  { value: 'only', label: '只看已排除', title: '只看被排除的模型，方便检查规则误杀' },
]

export function NamingPage({ view, draft, dispatch, saving }: Props) {
  const [rawQuery, setRawQuery] = useState('')
  const query = useDebounced(rawQuery)
  const [vendors, setVendors] = usePersistentState<string[]>('panel.naming.vendors', [])
  const [sites, setSites] = usePersistentState<string[]>('panel.naming.sites', [])
  const [protocol, setProtocol] = usePersistentState<ProtocolFilter>('panel.naming.protocol', 'all')
  const [aggregation, setAggregation] = usePersistentState<Aggregation>('panel.naming.aggregation', 'original')
  const [excludedView, setExcludedView] = usePersistentState<ExcludedView>('panel.naming.excludedView', 'all')

  const siteNames = useMemo(() => new Map(view.sites.map((site) => [site.id, site.name])), [view.sites])
  const siteName = useCallback((id: string) => siteNames.get(id) ?? id, [siteNames])

  // Classify on the cleaned name: "glm-5.2-anthropic" and "grok-4.5-claude"
  // name a protocol shape, not a vendor, and matching the raw name filed them
  // under Anthropic.
  const vendorOfModel = useMemo(() => {
    const cache = new Map<string, string>()
    return (model: ModelView) => {
      let vendor = cache.get(model.canonical)
      if (vendor === undefined) {
        vendor = vendorOf(model.canonical, model.upstream)
        cache.set(model.canonical, vendor)
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

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase()
    const vendorSet = new Set(vendors)
    const siteSet = new Set(sites)

    return view.models.filter((model) => {
      if (protocol !== 'all' && model.protocol !== protocol) return false
      if (vendorSet.size > 0 && !vendorSet.has(vendorOfModel(model))) return false
      if (siteSet.size > 0 && !siteSet.has(siteName(model.site))) return false
      if (excludedView !== 'all') {
        const excluded = !!effectiveExcluded(model, draft)
        if (excludedView === 'only' ? !excluded : excluded) return false
      }
      if (!needle) return true
      // Search covers model names only — sites have their own filter.
      return (
        model.upstream.toLowerCase().includes(needle) ||
        effectiveName(model, draft).toLowerCase().includes(needle) ||
        model.suggested.toLowerCase().includes(needle)
      )
    })
  }, [draft, excludedView, protocol, query, siteName, sites, vendorOfModel, vendors, view.models])

  const rows = useMemo(
    () => buildNamingRows({ models: filtered, draft, aggregation, siteName }),
    [aggregation, draft, filtered, siteName],
  )

  const actions = useMemo(
    () => ({
      onRename: (targets: EntryRef[], value: string) => dispatch({ kind: 'rename', targets, to: value }),
      onExclude: (targets: EntryRef[]) => dispatch({ kind: 'exclude', targets, excluded: true }),
      onCancelKeep: (targets: EntryRef[]) => dispatch({ kind: 'keep', targets, kept: false }),
      onApplySuggestion: (row: RowData) => dispatch({ kind: 'rename', targets: row.targets, to: row.suggested }),
      onRestore: (row: RowData) => {
        // A manual deletion is simply undone; a rule-based exclusion needs an
        // explicit override, otherwise the rule would drop it again instantly.
        if (row.excluded === 'manual') dispatch({ kind: 'exclude', targets: row.targets, excluded: false })
        else dispatch({ kind: 'keep', targets: row.targets, kept: true })
      },
    }),
    [dispatch],
  )

  const suggestable = useMemo(() => rows.filter((row) => row.suggested && !row.excluded), [rows])

  const applyAllSuggestions = useCallback(() => {
    for (const row of suggestable) dispatch({ kind: 'rename', targets: row.targets, to: row.suggested })
  }, [dispatch, suggestable])


  return (
    <>
      <div className="toolbar">
        <input
          className="input search"
          type="search"
          placeholder="搜索模型名 / 映射名 / 建议名…"
          value={rawQuery}
          onChange={(event) => setRawQuery(event.target.value)}
        />
        <Segmented
          label="协议"
          value={protocol}
          onChange={setProtocol}
          options={[
            { value: 'all', label: '全部' },
            { value: 'openai', label: 'OpenAI' },
            { value: 'codex', label: 'Codex' },
            { value: 'claude', label: 'Claude' },
          ]}
        />
        <FilterMenu label="供应商" allLabel="所有供应商" options={vendorOptions} selected={vendors} onChange={setVendors} />
        <FilterMenu label="站点" allLabel="所有站点" options={siteOptions} selected={sites} onChange={setSites} />
        <Segmented label="聚合" value={aggregation} onChange={setAggregation} options={AGGREGATION_OPTIONS} />
        <Segmented label="已排除" value={excludedView} onChange={setExcludedView} options={EXCLUDED_VIEW_OPTIONS} />
        <button
          type="button"
          className="btn btn-secondary"
          disabled={suggestable.length === 0 || saving}
          onClick={applyAllSuggestions}
          title="把当前筛选结果里所有清洗建议一次应用"
        >
          应用全部建议 {suggestable.length > 0 ? `(${suggestable.length})` : ''}
        </button>
        <span className="muted toolbar-status">
          {rows.length} 行 · {filtered.length} 条站点模型
        </span>
      </div>

      <VirtualList
        className="card naming-list"
        items={rows}
        rowHeight={rowHeightOf}
        rowKey={(row) => row.key}
        header={
          <div className="naming-row naming-head">
            <div className="cell">站点</div>
            <div className="cell">原始模型名</div>
            <div className="cell">重映射模型名</div>
          </div>
        }
        renderRow={(row) => <NamingRow row={row} saving={saving} actions={actions} />}
      />
    </>
  )
}
