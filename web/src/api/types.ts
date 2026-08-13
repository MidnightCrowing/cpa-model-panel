export type Protocol = 'openai' | 'codex' | 'claude'
export type Channel = Protocol
export type ExcludedReason = 'manual' | 'gone' | 'whitelist' | 'version'

/** One (site, upstream model) pair — the unit every edit addresses. */
export type EntryRef = {
  site: string
  upstream: string
}

export type ModelView = EntryRef & {
  /** Current remapped name in CPA; empty means the upstream name is used. */
  alias: string
  /** The name after cleaning: the remap when set, else the cleaned upstream
   *  name. Protocol and vendor classification both run on this. */
  canonical: string
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
  /** Whether CPA currently holds this model. */
  present: boolean
  /** False when the site has no provider the model could be written to. */
  writable: boolean
  /** The CPA list this model belongs in. */
  target: Channel
}

export type SiteView = {
  id: string
  /** The full CPA provider name, "<站点> / <分组>" by convention. */
  name: string
  /** name without the group suffix. */
  label: string
  /** The group half of name, when it has one. Two entries on one host usually
   *  differ only by group and api-key. */
  group?: string
  /** CPA ranks a site separately in each list, so priority is per channel. */
  priorities: Partial<Record<Channel, number>>
  channels: Channel[]
  active: number
  base_url: string
  /** False for entries CPA holds with an empty api-key. */
  has_key: boolean
  /** A short-lived endpoint added from a shared link. */
  temp?: boolean
  source_url?: string
  last_ok_at?: string
  last_error?: string
  failures?: number
}

export type ChannelDiff = {
  channel: Channel
  added: string[]
  removed: string[]
  renamed: string[]
  new_providers: string[]
}

export type SavePreview = {
  ok: boolean
  dry: true
  diff: ChannelDiff[]
  moved: number
  created: string[] | null
}

export type StatCell = {
  site: string
  model: string
  ok: number
  failed: number
  latency_ms: number
  last_at: string
}

export type StatsResult = {
  configured: boolean
  reason?: string
  range?: string
  updated_at?: string
  events?: number
  cells?: StatCell[]
}

export type EggResult = {
  ok: boolean
  site: string
  models: string[]
  key_used: string
  decoded: boolean
  view: View
}

export type Stats = {
  models: number
  active: number
  excluded: number
  disabled: number
  pending: number
  by_exclusion: Record<string, number>
  /** What a save would change even with an empty draft. */
  to_add: number
  to_remove: number
  /** Models sitting in a CPA list their protocol does not name. */
  to_move: number
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

export type Rewrite = {
  pattern: string
  replace: string
}

export type Settings = {
  prefixes: string[]
  suffixes: string[]
  /** Names matching this keep their suffix (qwen3-max is not qwen3). */
  protect: string
  /** Regex substitutions applied after stripping. */
  rewrites: Rewrite[]
  whitelist: string
  version: VersionFilterConfig
  protocol: ProtocolConfig
  /** Write each model to the CPA list its protocol names. */
  route_by_protocol: boolean
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
  | { type: 'set_priority'; site: string; channel: Channel; priority: number }

export type SaveResult = {
  ok: boolean
  view: View
  written: string[] | null
  renamed: number
  kept: number
  removed: number
  restored: number
  moved: number
  created: string[] | null
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
