import type { EntryRef, ExcludedReason, ModelView, Protocol } from '../../api/types'
import { refKey } from '../../lib/keys'
import type { Draft } from '../../state/useDraft'

export type Aggregation = 'site' | 'original' | 'remapped'

export const AGGREGATION_OPTIONS: Array<{ value: Aggregation; label: string; title: string }> = [
  { value: 'original', label: '原始模型名', title: '同一个原始模型名的所有站点合成一行，改名对这些站点同时生效' },
  { value: 'site', label: '站点', title: '每个站点的每个模型单独一行，改名只影响这一个站点的这一个模型' },
  { value: 'remapped', label: '重映射模型名', title: '当前映射到同一个名字的模型合成一行' },
]

/** Rows show every site and every original name. Past this many the cell
 *  scrolls internally, so one pathological row cannot produce a 40-line row. */
export const MAX_ROW_LINES = 8
export const LINE_HEIGHT = 20
export const ROW_PADDING = 18

export function rowHeightOf(row: NamingRow): number {
  const lines = Math.min(Math.max(row.siteNames.length, row.upstreams.length, 1), MAX_ROW_LINES)
  return ROW_PADDING + lines * LINE_HEIGHT
}

export type NamingRow = {
  key: string
  targets: EntryRef[]
  siteNames: string[]
  upstreams: string[]
  /** Effective name shared by the row; empty when the row's entries disagree. */
  value: string
  mixed: boolean
  suggested: string
  protocol: Protocol
  excluded?: ExcludedReason
  excludedCount: number
  reason?: string
  keptCount: number
  pendingCount: number
  disabledCount: number
  dirty: boolean
}

type BuildInput = {
  models: ModelView[]
  draft: Draft
  aggregation: Aggregation
  siteName: (id: string) => string
}

export function effectiveName(model: ModelView, draft: Draft): string {
  const override = draft.renames[refKey(model.site, model.upstream)]
  if (override !== undefined) return override.trim() || model.upstream
  return model.alias || model.upstream
}

export function effectiveExcluded(model: ModelView, draft: Draft): ExcludedReason | undefined {
  const key = refKey(model.site, model.upstream)
  const manual = draft.excluded[key]
  if (manual === true) return 'manual'
  if (manual === false && model.excluded === 'manual') return undefined
  const kept = draft.keeps[key]
  if (kept === true && model.excluded && model.excluded !== 'manual') return undefined
  if (kept === false && !model.excluded && model.kept) return undefined
  return model.excluded
}

/**
 * Collapses models into editable rows.
 *
 * Every row carries the exact list of (site, upstream) pairs it represents, so
 * editing a row can never reach a model that is not displayed on it — the
 * behaviour the old canonical-name-wide rename got wrong.
 */
export function buildNamingRows({ models, draft, aggregation, siteName }: BuildInput): NamingRow[] {
  const groups = new Map<string, ModelView[]>()

  for (const model of models) {
    const key =
      aggregation === 'site'
        ? refKey(model.site, model.upstream)
        : aggregation === 'remapped'
          ? effectiveName(model, draft)
          : model.upstream
    const bucket = groups.get(key)
    if (bucket) bucket.push(model)
    else groups.set(key, [model])
  }

  const rows: NamingRow[] = []
  for (const [key, entries] of groups) {
    const names = new Set<string>()
    const suggestions = new Set<string>()
    const siteNames: string[] = []
    const upstreams: string[] = []
    let excluded: ExcludedReason | undefined
    let excludedCount = 0
    let reason: string | undefined
    let keptCount = 0
    let pendingCount = 0
    let disabledCount = 0
    let dirty = false

    for (const model of entries) {
      const entryKey = refKey(model.site, model.upstream)
      names.add(effectiveName(model, draft))
      if (model.suggested) suggestions.add(model.suggested)
      if (!siteNames.includes(model.site)) siteNames.push(model.site)
      if (!upstreams.includes(model.upstream)) upstreams.push(model.upstream)

      const state = effectiveExcluded(model, draft)
      if (state) {
        excludedCount++
        if (!excluded) {
          excluded = state
          reason = model.reason
        }
      }
      if (model.kept || draft.keeps[entryKey]) keptCount++
      if (model.pending) pendingCount++
      if (model.disabled) disabledCount++
      if (
        draft.renames[entryKey] !== undefined ||
        draft.excluded[entryKey] !== undefined ||
        draft.keeps[entryKey] !== undefined
      ) {
        dirty = true
      }
    }

    const value = names.size === 1 ? [...names][0] : ''
    const suggested = suggestions.size === 1 ? [...suggestions][0] : ''

    rows.push({
      key,
      targets: entries.map((model) => ({ site: model.site, upstream: model.upstream })),
      siteNames: siteNames.map(siteName),
      upstreams,
      value,
      mixed: names.size > 1,
      suggested: suggested && suggested !== value ? suggested : '',
      protocol: entries[0].protocol,
      excluded,
      excludedCount,
      reason,
      keptCount,
      pendingCount,
      disabledCount,
      dirty,
    })
  }

  rows.sort((a, b) => a.key.localeCompare(b.key, 'zh-Hans-CN'))
  return rows
}
