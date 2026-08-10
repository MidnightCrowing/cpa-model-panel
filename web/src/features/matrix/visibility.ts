import type { EntryRef, ModelView, Protocol, SiteView } from '../../api/types'
import { refKey } from '../../lib/keys'

export type MatrixRow = {
  key: string
  name: string
  protocol: Protocol
  /** site id → the (site, upstream) pairs that produce this name there */
  cells: Map<string, EntryRef[]>
  refs: EntryRef[]
}

export type MatrixLayout = {
  rows: MatrixRow[]
  sites: SiteView[]
  hiddenRows: number
  hiddenColumns: number
}

type BuildInput = {
  models: ModelView[]
  sites: SiteView[]
  renames: Record<string, string>
  hideEmptyRows: boolean
  hideEmptyColumns: boolean
}

/**
 * Builds the matrix and resolves row/column visibility in one pass.
 *
 * Columns are decided from the rows that survived filtering, then rows are
 * filtered once against those columns. Deliberately not iterated to a fixed
 * point: one pass is what the feature means, and it makes the layout a pure
 * function of the filters instead of something that keeps re-settling while
 * you click.
 */
export function buildMatrix({ models, sites, renames, hideEmptyRows, hideEmptyColumns }: BuildInput): MatrixLayout {
  const byName = new Map<string, MatrixRow>()

  for (const model of models) {
    const override = renames[refKey(model.site, model.upstream)]
    const name = (override !== undefined ? override.trim() : model.alias) || model.upstream

    let row = byName.get(name)
    if (!row) {
      row = { key: name, name, protocol: model.protocol, cells: new Map(), refs: [] }
      byName.set(name, row)
    }
    const ref = { site: model.site, upstream: model.upstream }
    const cell = row.cells.get(model.site)
    if (cell) cell.push(ref)
    else row.cells.set(model.site, [ref])
    row.refs.push(ref)
  }

  const rows = [...byName.values()].sort((a, b) => a.name.localeCompare(b.name, 'zh-Hans-CN'))

  let visibleSites = sites
  let hiddenColumns = 0
  if (hideEmptyColumns) {
    visibleSites = sites.filter((site) => rows.some((row) => row.cells.has(site.id)))
    hiddenColumns = sites.length - visibleSites.length
  }

  let visibleRows = rows
  let hiddenRows = 0
  if (hideEmptyRows) {
    const ids = new Set(visibleSites.map((site) => site.id))
    visibleRows = rows.filter((row) => {
      for (const site of row.cells.keys()) if (ids.has(site)) return true
      return false
    })
    hiddenRows = rows.length - visibleRows.length
  }

  return { rows: visibleRows, sites: visibleSites, hiddenRows, hiddenColumns }
}
