import { authHeaders, request } from './client'
import type {
  AutoSyncConfig,
  AutoSyncPayload,
  EggResult,
  Op,
  RefreshResult,
  SavePreview,
  SaveResult,
  Settings,
  SnapshotMeta,
  StatsResult,
  View,
} from './types'

export function login(token: string) {
  return request<{ ok: boolean; token: string }>('/api/login', {
    method: 'POST',
    body: JSON.stringify({ token }),
  })
}

export function fetchView() {
  return request<View>('/api/catalog')
}

export function saveOps(fingerprint: string, ops: Op[], note = 'ui-save') {
  return request<SaveResult>('/api/save', {
    method: 'POST',
    body: JSON.stringify({ fingerprint, ops, note }),
  })
}

export function fetchSettings() {
  return request<{ settings: Settings }>('/api/settings')
}

export function putSettings(settings: Settings) {
  // The server rejects unknown shapes, and older stored settings may predate
  // these fields.
  settings = { ...settings, protect: settings.protect ?? '', rewrites: settings.rewrites ?? [] }
  return request<{ ok: boolean; view: View }>('/api/settings', {
    method: 'PUT',
    body: JSON.stringify(settings),
  })
}

export function fetchAutoSync() {
  return request<AutoSyncPayload>('/api/auto-sync')
}

export function putAutoSync(config: AutoSyncConfig) {
  return request<AutoSyncPayload>('/api/auto-sync', {
    method: 'PUT',
    body: JSON.stringify(config),
  })
}

/** Same pipeline as a save, but nothing is written. */
export function previewOps(fingerprint: string, ops: Op[]) {
  return request<SavePreview>('/api/save?dry=1', {
    method: 'POST',
    body: JSON.stringify({ fingerprint, ops }),
  })
}

export function fetchStats(range = '24h') {
  return request<StatsResult>(`/api/stats?range=${range}`)
}

export function refreshSite(site: string) {
  return request<{ ok: boolean; site: string; found: number; added: number; dropped: number; view: View }>(
    `/api/sites/${encodeURIComponent(site)}/refresh`,
    { method: 'POST' },
  )
}

export function deleteSite(site: string) {
  return request<{ ok: boolean; removed: string[]; view: View }>(`/api/sites/${encodeURIComponent(site)}`, {
    method: 'DELETE',
  })
}

export function addEgg(payload: { url: string; key: string; name?: string; source_url?: string; priority?: number }) {
  return request<EggResult>('/api/eggs', { method: 'POST', body: JSON.stringify(payload) })
}

export function fetchSnapshots() {
  return request<{ snapshots: SnapshotMeta[] }>('/api/snapshots')
}

export function rollbackSnapshot(id: number) {
  return request<{ ok: boolean; view?: View }>(`/api/snapshots/${id}/rollback`, { method: 'POST' })
}

export type RefreshProgress = {
  completed: number
  total: number
  site: string
  ok: boolean
  found?: number
  error?: string
}

/**
 * Streams the per-site model discovery. Refresh never writes to CPA: new
 * models arrive as pending rows and reach CPA only when the user saves.
 */
export async function refreshCatalog(
  onStart: (total: number) => void,
  onProgress: (progress: RefreshProgress) => void,
): Promise<RefreshResult> {
  const res = await fetch('/api/catalog/refresh', { method: 'POST', headers: authHeaders() })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new Error(data?.error || res.statusText || `HTTP ${res.status}`)
  }
  const reader = res.body?.getReader()
  if (!reader) throw new Error('当前浏览器不支持流式响应')

  const decoder = new TextDecoder()
  let buffer = ''
  let result: RefreshResult | null = null

  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })

    const chunks = buffer.split('\n\n')
    buffer = chunks.pop() || ''
    for (const chunk of chunks) {
      const line = chunk.trim()
      if (!line.startsWith('data:')) continue
      const payload = line.slice(5).trim()
      if (!payload) continue

      const event = JSON.parse(payload)
      if (event.type === 'start') onStart(event.total)
      else if (event.type === 'progress') onProgress(event as RefreshProgress)
      else if (event.type === 'done') {
        if (event.error) throw new Error(event.error)
        result = event as RefreshResult
      }
    }
  }

  if (!result) throw new Error('刷新中断，未收到结果')
  return result
}
