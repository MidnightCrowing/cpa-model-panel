export type Protocol = 'openai' | 'codex' | 'claude'
export type Channel = Protocol
export type ExcludedReason = 'manual' | 'whitelist' | 'version'

/** One (site, upstream model) pair — the unit every edit addresses. */
export type EntryRef = {
  site: string
  upstream: string
}

export type ModelView = EntryRef & {
  /** Current remapped name in CPA; empty means the upstream name is used. */
  alias: string
  /** What the prefix/suffix rules would name it. */
  suggested: string
  protocol: Protocol
  /** Which CPA lists physically hold this model. */
  channels: Channel[]
  excluded?: ExcludedReason
  /** Human explanation for a version-filter exclusion. */
  reason?: string
  /** Explicitly kept despite the whitelist / version rules. */
  kept?: boolean
  /** Turned off at this site. */
  disabled?: boolean
  /** Discovered upstream, not written to CPA yet. */
  pending?: boolean
}

export type SiteView = {
  id: string
  name: string
  priority: number
  channels: Channel[]
  active: number
}

export type Stats = {
  models: number
  active: number
  excluded: number
  disabled: number
  pending: number
  by_exclusion: Record<string, number>
}

export type SeriesThreshold = {
  series: string
  min_version: number
}

export type VersionFilterConfig = {
  enabled: boolean
  thresholds: SeriesThreshold[]
  exempt_pattern: string
}

export type ProtocolConfig = {
  codex_regex: string
  claude_regex: string
}

export type Settings = {
  prefixes: string[]
  suffixes: string[]
  whitelist: string
  version: VersionFilterConfig
  protocol: ProtocolConfig
}

export type View = {
  fingerprint: string
  fetched_at: string
  sites: SiteView[]
  models: ModelView[]
  stats: Stats
  settings: Settings
}

export type Op =
  | { type: 'rename'; targets: EntryRef[]; to: string }
  | { type: 'exclude'; targets: EntryRef[] }
  | { type: 'include'; targets: EntryRef[] }
  | { type: 'keep'; targets: EntryRef[] }
  | { type: 'unkeep'; targets: EntryRef[] }
  | { type: 'set_disabled'; targets: EntryRef[]; disabled: boolean }
  | { type: 'set_priority'; site: string; priority: number }

export type SaveResult = {
  ok: boolean
  view: View
  written: string[] | null
  kept: number
  removed: number
  restored: number
  skipped: number
  snapshot?: number
}

export type RefreshFailure = {
  site: string
  name: string
  error: string
}

export type RefreshResult = {
  ok: boolean
  refreshed: number
  added: number
  failed: RefreshFailure[]
  view: View
}

export type SnapshotMeta = {
  id: number
  created_at: string
  fingerprint: string
  note: string
}
