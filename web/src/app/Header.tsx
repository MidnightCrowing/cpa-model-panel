import type { View } from '../api/types'
import type { RefreshState } from '../state/useCatalog'

export type Tab = 'naming' | 'matrix' | 'settings'

const TABS: Array<{ value: Tab; label: string }> = [
  { value: 'naming', label: '命名映射' },
  { value: 'matrix', label: '站点启停' },
  { value: 'settings', label: '设置' },
]

type Props = {
  tab: Tab
  onTab: (tab: Tab) => void
  view: View | null
  dirty: number
  loading: boolean
  saving: boolean
  refreshing: RefreshState
  onRefresh: () => void
  onSave: () => void
  onDiscard: () => void
  onReload: () => void
  onLogout: () => void
}

export function Header({
  tab,
  onTab,
  view,
  dirty,
  loading,
  saving,
  refreshing,
  onRefresh,
  onSave,
  onDiscard,
  onReload,
  onLogout,
}: Props) {
  const busy = loading || saving || refreshing !== null

  return (
    <header className="main-header">
      <div className="brand">CPA Model Panel</div>

      <nav className="segmented" aria-label="主导航">
        {TABS.map((item) => (
          <button
            key={item.value}
            type="button"
            className={`segmented-item ${tab === item.value ? 'is-active' : ''}`}
            onClick={() => onTab(item.value)}
          >
            {item.label}
          </button>
        ))}
      </nav>

      <div className="header-actions">
        {dirty > 0 ? (
          <span className="badge is-warn">未保存 {dirty} 项</span>
        ) : (
          <span className="badge is-ok">已同步</span>
        )}
        {view && (
          <span className="badge mono" title={`可用 ${view.stats.active} · 被排除 ${view.stats.excluded} · 停用 ${view.stats.disabled} · 待写入 ${view.stats.pending}`}>
            {view.stats.active}/{view.stats.models}
          </span>
        )}

        <button className="btn btn-secondary" onClick={onRefresh} disabled={busy} title="让 CPA 逐站拉取上游模型列表并合并进面板缓存；不会直接写回 CPA">
          {refreshing ? `刷新中 ${refreshing.completed}/${refreshing.total}` : '刷新站点模型'}
        </button>
        <button className="btn btn-ghost" onClick={onReload} disabled={busy} title="重新读取 CPA 当前配置">
          重新载入
        </button>
        <button className="btn btn-ghost" onClick={onDiscard} disabled={dirty === 0 || saving}>
          丢弃草稿
        </button>
        <button className="btn btn-primary" onClick={onSave} disabled={dirty === 0 || busy}>
          {saving ? '保存中…' : '保存到 CPA'}
        </button>
        <button className="btn btn-danger" onClick={onLogout}>
          退出
        </button>
      </div>
    </header>
  )
}
