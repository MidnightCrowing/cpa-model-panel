import { useEffect, useRef, useState } from 'react'
import type { SiteView } from '../../api/types'

export type SiteMenuActions = {
  onRefresh: (site: SiteView) => void
  onEnableAll: (site: SiteView) => void
  onDisableAll: (site: SiteView) => void
  onPriority: (site: SiteView, priority: number) => void
  onDelete: (site: SiteView) => void
}

const CHANNEL_LABEL: Record<string, string> = {
  openai: 'openai-compatibility',
  codex: 'codex-api-key',
  claude: 'claude-api-key',
}

/**
 * Everything you can do to one site, in one popover.
 *
 * The column is 120px wide and there are five actions plus a priority; a
 * popover keeps the grid readable and gets rid of the window.prompt() the
 * priority used to need.
 */
export function SiteMenu({
  site,
  priority,
  busy,
  actions,
  onClose,
}: {
  site: SiteView
  priority: number
  busy: boolean
  actions: SiteMenuActions
  onClose: () => void
}) {
  const rootRef = useRef<HTMLDivElement>(null)
  const [draftPriority, setDraftPriority] = useState(priority)

  useEffect(() => setDraftPriority(priority), [priority])

  useEffect(() => {
    const onPointerDown = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) onClose()
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [onClose])

  const commitPriority = (value: number) => {
    setDraftPriority(value)
    actions.onPriority(site, value)
  }

  return (
    <div className="site-menu" ref={rootRef}>
      <div className="site-menu-title">
        <span className="site-menu-name">{site.name}</span>
        {site.temp && <span className="chip chip-egg">鸡蛋</span>}
        <button
          type="button"
          className="btn-icon site-menu-refresh"
          disabled={busy}
          onClick={() => actions.onRefresh(site)}
          title="刷新此站点"
        >
          {busy ? (
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M21 12a9 9 0 11-6.219-8.56" />
            </svg>
          ) : (
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <path d="M21.5 2v6h-6M2.5 22v-6h6M2 11.5a10 10 0 0118.8-4.3M22 12.5a10 10 0 01-18.8 4.2" />
            </svg>
          )}
        </button>
      </div>

      <div className="site-menu-meta mono">{site.base_url}</div>
      <div className="site-menu-meta">
        配置于：{site.channels.map((channel) => CHANNEL_LABEL[channel] ?? channel).join('、') || '—'}
      </div>
      {!site.has_key && <div className="site-menu-warn">CPA 里这条没有 api-key，无法探测（它自己的界面也不显示）</div>}
      {site.last_error && (
        <div className="site-menu-warn" title={site.last_error}>
          上次探测失败{site.failures ? ` ×${site.failures}` : ''}：{site.last_error.slice(0, 60)}
        </div>
      )}
      {site.last_ok_at && <div className="site-menu-meta">最后成功：{site.last_ok_at.replace('T', ' ').slice(0, 19)}</div>}

      <div className="site-menu-priority">
        <span>优先级</span>
        <div className="site-menu-priority-controls">
          <input
            className="input mono"
            type="number"
            value={draftPriority}
            onChange={(event) => setDraftPriority(Number.parseInt(event.target.value, 10) || 0)}
            onBlur={() => actions.onPriority(site, draftPriority)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') event.currentTarget.blur()
            }}
          />
          <button type="button" className="btn btn-secondary btn-xs" onClick={() => commitPriority(draftPriority - 1)}>
            −
          </button>
          <button type="button" className="btn btn-secondary btn-xs" onClick={() => commitPriority(draftPriority + 1)}>
            +
          </button>
        </div>
      </div>

      <div className="site-menu-actions">
        <button type="button" className="btn btn-secondary btn-sm" onClick={() => actions.onEnableAll(site)}>
          整列全开
        </button>
        <button type="button" className="btn btn-secondary btn-sm" onClick={() => actions.onDisableAll(site)}>
          整列全关
        </button>
        {site.source_url && (
          <a className="btn btn-secondary btn-sm" href={site.source_url} target="_blank" rel="noreferrer noopener">
            来源 ↗
          </a>
        )}
      </div>

      <button type="button" className="btn btn-danger btn-sm site-menu-delete" disabled={busy} onClick={() => actions.onDelete(site)}>
        从 CPA 删除站点
      </button>
    </div>
  )
}
