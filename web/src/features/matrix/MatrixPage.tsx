import { useCallback, useMemo, useState } from 'react'
import { deleteSite as deleteSiteRequest, refreshSite } from '../../api/catalog'
import type { EntryRef, ModelView, Protocol, SiteView, View } from '../../api/types'
import { Checkbox, Segmented } from '../../components/Controls'
import { FilterMenu } from '../../components/FilterMenu'
import { useDebounced, usePersistentState } from '../../lib/hooks'
import { refKey, siteChannelKey } from '../../lib/keys'
import { UNKNOWN_VENDOR, VENDORS, vendorOf } from '../../lib/vendor'
import type { Draft, DraftAction } from '../../state/useDraft'
import { useStats } from '../../state/useStats'
import { useToasts } from '../../state/useToasts'
import { MatrixGrid } from './MatrixGrid'
import { buildMatrix, type MatrixRow } from './visibility'
import { effectiveExcluded } from '../naming/grouping'

type Props = {
  view: View
  draft: Draft
  dispatch: (action: DraftAction) => void
  onView: (view: View) => void
}

export function MatrixPage({ view, draft, dispatch, onView }: Props) {
  const { push } = useToasts()
  const [rawQuery, setRawQuery] = useState('')
  const query = useDebounced(rawQuery)
  // Keyed .v2 because the old setting could hold "all", a value this page no
  // longer has: priorities are per protocol, so one combined view would have
  // to show three different numbers per column.
  const [protocol, setProtocol] = usePersistentState<Protocol>('panel.matrix.protocol.v2', 'openai')
  const [vendors, setVendors] = usePersistentState<string[]>('panel.matrix.vendors', [])
  const [sites, setSites] = usePersistentState<string[]>('panel.matrix.sites', [])
  const [hideEmptyRows, setHideEmptyRows] = usePersistentState('panel.matrix.hideRows', true)
  const [hideEmptyColumns, setHideEmptyColumns] = usePersistentState('panel.matrix.hideCols', true)
  const [busySite, setBusySite] = useState<string | null>(null)

  const stats = useStats(true)

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

  // Only sites CPA actually has an entry for in this list can be shown: the
  // column's priority, toggles and save all address that one entry.
  const protocolSites = useMemo(
    () => view.sites.filter((site) => site.channels.includes(protocol)),
    [protocol, view.sites],
  )

  const siteOptions = useMemo(() => protocolSites.map((site) => site.name), [protocolSites])

  // Excluded models never appear here: they are deleted from CPA's list.
  const models = useMemo(() => {
    const needle = query.trim().toLowerCase()
    const vendorSet = new Set(vendors)
    return view.models.filter((model) => {
      if (effectiveExcluded(model, draft)) return false
      if (model.protocol !== protocol) return false
      if (vendorSet.size > 0 && !vendorSet.has(vendorOfModel(model))) return false
      if (!needle) return true
      const alias = draft.renames[refKey(model.site, model.upstream)] ?? model.alias
      return model.upstream.toLowerCase().includes(needle) || (alias || '').toLowerCase().includes(needle)
    })
  }, [draft, protocol, query, vendorOfModel, vendors, view.models])

  const selectedSites = useMemo(() => {
    if (sites.length === 0) return protocolSites
    const chosen = new Set(sites)
    return protocolSites.filter((site) => chosen.has(site.name))
  }, [protocolSites, sites])

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

  const refsOfSite = useCallback(
    (site: SiteView) => layout.rows.flatMap((row) => row.cells.get(site.id) ?? []),
    [layout.rows],
  )

  const actions = useMemo(
    () => ({
      onToggle: (targets: EntryRef[], currentlyEnabled: boolean) =>
        dispatch({ kind: 'disable', targets, disabled: currentlyEnabled }),
      onSetRow: (row: MatrixRow, disabled: boolean) => {
        const visible = new Set(layout.sites.map((site) => site.id))
        dispatch({ kind: 'disable', targets: row.refs.filter((ref) => visible.has(ref.site)), disabled })
      },
      onEnableAll: (site: SiteView) => dispatch({ kind: 'disable', targets: refsOfSite(site), disabled: false }),
      onDisableAll: (site: SiteView) => dispatch({ kind: 'disable', targets: refsOfSite(site), disabled: true }),
      // The edit names the channel it applies to; the site's other two lists
      // keep whatever CPA has.
      onPriority: (site: SiteView, priority: number) =>
        dispatch({ kind: 'priority', site: site.id, channel: protocol, priority }),
      onRefresh: (site: SiteView) => {
        setBusySite(site.id)
        void refreshSite(site.id)
          .then((result) => {
            onView(result.view)
            push('ok', `${site.name}：拉到 ${result.found} 个模型，新增 ${result.added} 个，删除 ${result.dropped} 个`)
          })
          .catch((error) => push('error', `${site.name}：${(error as Error).message}`, { sticky: true }))
          .finally(() => setBusySite(null))
      },
      onDelete: (site: SiteView) => {
        const what = site.temp ? '鸡蛋' : '站点'
        if (!window.confirm(`确认从 CPA 删除${what}「${site.name}」？会从它所在的每张表里移除，删除前自动存快照。`)) return
        setBusySite(site.id)
        void deleteSiteRequest(site.id)
          .then((result) => {
            onView(result.view)
            push('ok', `已删除 ${site.name}`, { detail: [`从 ${result.removed.join('、')} 移除`] })
          })
          .catch((error) => push('error', `${site.name}：${(error as Error).message}`, { sticky: true }))
          .finally(() => setBusySite(null))
      },
    }),
    [dispatch, layout.sites, onView, protocol, push, refsOfSite],
  )

  // Priority edits reorder the columns immediately, without a round trip, and
  // they order by *this* protocol's number.
  const orderedSites = useMemo(() => {
    const priorityOf = (site: SiteView) =>
      draft.priorities[siteChannelKey(site.id, protocol)] ?? site.priorities[protocol] ?? 0
    return [...layout.sites].sort((a, b) => {
      const pa = priorityOf(a)
      const pb = priorityOf(b)
      return pa !== pb ? pb - pa : a.name.localeCompare(b.name, 'zh-Hans-CN')
    })
  }, [draft.priorities, layout.sites, protocol])

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
            { value: 'openai', label: 'OpenAI', title: 'openai-compatibility 这张表，及它自己的站点优先级' },
            { value: 'codex', label: 'Codex', title: 'codex-api-key 这张表，及它自己的站点优先级' },
            { value: 'claude', label: 'Claude', title: 'claude-api-key 这张表，及它自己的站点优先级' },
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
          {stats.configured ? ` · 统计 ${stats.range}` : ''}
        </span>
      </div>

      <MatrixGrid
        rows={layout.rows}
        sites={orderedSites}
        protocol={protocol}
        disabledDraft={draft.disabled}
        baseDisabled={baseDisabled}
        priorityDraft={draft.priorities}
        stats={stats}
        busySite={busySite}
        actions={actions}
      />
    </>
  )
}
