import { useReducer } from 'react'
import type { Channel, EntryRef, ModelView, Op, SiteView } from '../api/types'
import { keyOf, parseKey, parseSiteChannelKey, refKey, siteChannelKey } from '../lib/keys'

/**
 * The draft holds *overrides* keyed by entry, never a copy of the data. Every
 * override addresses explicit (site, upstream) pairs, which is what makes an
 * edit made on one row apply to that row only.
 *
 * priorities is keyed by (site, channel): CPA ranks a site separately in each
 * of its three lists, so one number per site would overwrite the other two.
 */
export type Draft = {
  renames: Record<string, string>
  excluded: Record<string, boolean>
  keeps: Record<string, boolean>
  disabled: Record<string, boolean>
  priorities: Record<string, number>
}

export function emptyDraft(): Draft {
  return { renames: {}, excluded: {}, keeps: {}, disabled: {}, priorities: {} }
}

export type DraftAction =
  | { kind: 'rename'; targets: EntryRef[]; to: string }
  | { kind: 'exclude'; targets: EntryRef[]; excluded: boolean }
  | { kind: 'keep'; targets: EntryRef[]; kept: boolean }
  | { kind: 'disable'; targets: EntryRef[]; disabled: boolean }
  | { kind: 'priority'; site: string; channel: Channel; priority: number }
  | { kind: 'reset' }

function reduce(draft: Draft, action: DraftAction): Draft {
  switch (action.kind) {
    case 'rename': {
      const renames = { ...draft.renames }
      for (const target of action.targets) renames[keyOf(target)] = action.to
      return { ...draft, renames }
    }
    case 'exclude': {
      const excluded = { ...draft.excluded }
      for (const target of action.targets) excluded[keyOf(target)] = action.excluded
      return { ...draft, excluded }
    }
    case 'keep': {
      const keeps = { ...draft.keeps }
      for (const target of action.targets) keeps[keyOf(target)] = action.kept
      return { ...draft, keeps }
    }
    case 'disable': {
      const disabled = { ...draft.disabled }
      for (const target of action.targets) disabled[keyOf(target)] = action.disabled
      return { ...draft, disabled }
    }
    case 'priority':
      return {
        ...draft,
        priorities: { ...draft.priorities, [siteChannelKey(action.site, action.channel)]: action.priority },
      }
    case 'reset':
      return emptyDraft()
  }
}

export function useDraft() {
  return useReducer(reduce, undefined, emptyDraft)
}

export type Baseline = {
  models: Map<string, ModelView>
  sites: Map<string, SiteView>
}

export function baselineOf(models: ModelView[], sites: SiteView[]): Baseline {
  return {
    models: new Map(models.map((model) => [refKey(model.site, model.upstream), model])),
    sites: new Map(sites.map((site) => [site.id, site])),
  }
}

/** Overrides that actually differ from what CPA already has. */
export function changedKeys(draft: Draft, baseline: Baseline) {
  const renames: Array<[string, string]> = []
  const excluded: Array<[string, boolean]> = []
  const keeps: Array<[string, boolean]> = []
  const disabled: Array<[string, boolean]> = []
  const priorities: Array<[string, number]> = []

  for (const [key, to] of Object.entries(draft.renames)) {
    const base = baseline.models.get(key)
    if (!base) continue
    const current = base.alias || base.upstream
    if (to.trim() !== current) renames.push([key, to.trim()])
  }
  for (const [key, value] of Object.entries(draft.excluded)) {
    const base = baseline.models.get(key)
    if (!base) continue
    if (value !== (base.excluded === 'manual')) excluded.push([key, value])
  }
  for (const [key, value] of Object.entries(draft.keeps)) {
    const base = baseline.models.get(key)
    if (!base) continue
    if (value !== !!base.kept) keeps.push([key, value])
  }
  for (const [key, value] of Object.entries(draft.disabled)) {
    const base = baseline.models.get(key)
    if (!base) continue
    if (value !== !!base.disabled) disabled.push([key, value])
  }
  for (const [key, value] of Object.entries(draft.priorities)) {
    const { site, channel } = parseSiteChannelKey(key)
    const base = baseline.sites.get(site)
    if (!base) continue
    if (value !== (base.priorities?.[channel as Channel] ?? 0)) priorities.push([key, value])
  }

  return { renames, excluded, keeps, disabled, priorities }
}

export function countChanges(draft: Draft, baseline: Baseline): number {
  const changed = changedKeys(draft, baseline)
  return (
    changed.renames.length +
    changed.excluded.length +
    changed.keeps.length +
    changed.disabled.length +
    changed.priorities.length
  )
}

/** Turns the draft into the op list the backend applies. */
export function buildOps(draft: Draft, baseline: Baseline): Op[] {
  const changed = changedKeys(draft, baseline)
  const ops: Op[] = []

  const byName = new Map<string, EntryRef[]>()
  for (const [key, to] of changed.renames) {
    const list = byName.get(to) ?? []
    list.push(parseKey(key))
    byName.set(to, list)
  }
  for (const [to, targets] of byName) ops.push({ type: 'rename', targets, to })

  pushRefOp(ops, changed.excluded, true, (targets) => ({ type: 'exclude', targets }))
  pushRefOp(ops, changed.excluded, false, (targets) => ({ type: 'include', targets }))
  pushRefOp(ops, changed.keeps, true, (targets) => ({ type: 'keep', targets }))
  pushRefOp(ops, changed.keeps, false, (targets) => ({ type: 'unkeep', targets }))
  pushRefOp(ops, changed.disabled, true, (targets) => ({ type: 'set_disabled', targets, disabled: true }))
  pushRefOp(ops, changed.disabled, false, (targets) => ({ type: 'set_disabled', targets, disabled: false }))

  for (const [key, priority] of changed.priorities) {
    const { site, channel } = parseSiteChannelKey(key)
    ops.push({ type: 'set_priority', site, channel: channel as Channel, priority })
  }

  return ops
}

function pushRefOp(
  ops: Op[],
  entries: Array<[string, boolean]>,
  want: boolean,
  build: (targets: EntryRef[]) => Op,
) {
  const targets = entries.filter(([, value]) => value === want).map(([key]) => parseKey(key))
  if (targets.length > 0) ops.push(build(targets))
}
