import { useEffect, useRef, useState } from 'react'
import type { Protocol, SiteView } from '../../api/types'

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
 * The column is 120px wide and there are five actions plus a priority, so a
 * popover keeps the grid readable. The priority applies to the protocol the
 * page is showing: CPA ranks a site separately in each of its three lists.
 */
export function SiteMenu({
  site,
  protocol,
  priority,
  busy,
  actions,
  onClose,
}: {
  site: SiteView
  protocol: Protocol
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

  const otherChannels = site.channels.filter((channel) => channel !== protocol)

  return (
    <div className="site-menu" ref={rootRef}>
      <div className="site-menu-title">
        <span className="site-menu-name">
          {site.label || site.name}
          {site.group && <span className="site-menu-group">{site.group}</span>}
        </span>
        {site.temp && <span className="chip chip-egg">鸡蛋</span>}
        <button
          type="button"
          className="btn-icon site-menu-refresh"
          disabled={busy}
          onClick={() => actions.onRefresh(site)}
          title="刷新此站点"
          aria-label="刷新此站点"
        >
          <RefreshIcon spinning={busy} />
        </button>
      </div>

      <div className="site-menu-meta mono">{site.base_url}</div>
      <div className="site-menu-meta">
        配置于：{site.channels.map((channel) => CHANNEL_LABEL[channel] ?? channel).join('、') || '—'}
      </div>
      {site.last_error && (
        <div className="site-menu-warn" title={site.last_error}>
          上次探测失败{site.failures ? ` ×${site.failures}` : ''}：{site.last_error.slice(0, 60)}
        </div>
      )}
      {site.last_ok_at && <div className="site-menu-meta">最后成功：{site.last_ok_at.replace('T', ' ').slice(0, 19)}</div>}

      <div className="site-menu-priority">
        <span className="site-menu-priority-label">
          优先级
          <span className="muted">{CHANNEL_LABEL[protocol]}</span>
        </span>
        <span className="site-menu-stepper">
          <button type="button" className="btn btn-secondary btn-xs" onClick={() => commitPriority(draftPriority - 1)}>
            −
          </button>
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
          <button type="button" className="btn btn-secondary btn-xs" onClick={() => commitPriority(draftPriority + 1)}>
            +
          </button>
        </span>
      </div>
      {otherChannels.length > 0 && (
        <div className="site-menu-meta">
          其它协议：
          {otherChannels
            .map((channel) => `${CHANNEL_LABEL[channel] ?? channel} ${site.priorities[channel] ?? 0}`)
            .join('　')}
        </div>
      )}

      <div className="site-menu-actions">
        <button type="button" className="btn btn-secondary btn-sm" onClick={() => actions.onEnableAll(site)}>
          整列全开
        </button>
        <button type="button" className="btn btn-secondary btn-sm" onClick={() => actions.onDisableAll(site)}>
          整列全关
        </button>
      </div>

      {site.source_url && (
        <a className="btn btn-ghost btn-sm site-menu-source" href={site.source_url} target="_blank" rel="noreferrer noopener">
          来源 ↗
        </a>
      )}

      <button
        type="button"
        className="btn btn-danger btn-sm site-menu-delete"
        disabled={busy}
        onClick={() => actions.onDelete(site)}
      >
        从 CPA 删除站点
      </button>
    </div>
  )
}

function RefreshIcon({ spinning }: { spinning: boolean }) {
  return (
    <svg
      className={spinning ? 'icon-spin' : undefined}
      width="15"
      height="15"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="M21 12a9 9 0 1 1-2.64-6.36" />
      <polyline points="21 3 21 9 15 9" />
    </svg>
  )
}
