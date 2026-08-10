import { useMemo } from 'react'
import type { Settings, View } from '../../api/types'

type Props = {
  settings: Settings
  view: View | null
  onChange: (settings: Settings) => void
  onSave: () => void
  busy: boolean
}

export function WhitelistCard({ settings, view, onChange, onSave, busy }: Props) {
  const removed = useMemo(
    () => (view?.models ?? []).filter((model) => model.excluded === 'whitelist'),
    [view],
  )

  return (
    <section className="card settings-card">
      <h2 className="card-title">模型白名单</h2>
      <p className="card-subtitle">
        正则只匹配<strong>原始模型名</strong>。没匹配上的模型会从 CPA 的模型列表里删除，但仍保留在面板缓存中 ——
        放宽正则或在命名页点「恢复」就能拿回来。留空表示不过滤。
      </p>

      <textarea
        className="textarea"
        rows={3}
        spellCheck={false}
        placeholder="例如：^(claude-|gpt-|deepseek-)"
        value={settings.whitelist}
        onChange={(event) => onChange({ ...settings, whitelist: event.target.value })}
      />

      <div className="settings-actions">
        <button type="button" className="btn btn-primary" onClick={onSave} disabled={busy}>
          保存并重算
        </button>
        <span className="muted">保存只更新面板视图；点顶栏「保存到 CPA」才会真正删除</span>
      </div>

      {view && <ExclusionSummary title="当前被白名单排除" models={removed} />}
    </section>
  )
}

export function ExclusionSummary({
  title,
  models,
}: {
  title: string
  models: Array<{ site: string; upstream: string; reason?: string }>
}) {
  if (models.length === 0) {
    return <div className="muted settings-summary">{title}：0 个模型</div>
  }
  return (
    <details className="settings-summary">
      <summary>
        {title}：<strong>{models.length}</strong> 个模型（点击展开）
      </summary>
      <div className="settings-summary-list mono">
        {models.map((model) => (
          <div key={`${model.site}|${model.upstream}`}>
            {model.site}: {model.upstream}
            {model.reason ? `  (${model.reason})` : ''}
          </div>
        ))}
      </div>
    </details>
  )
}
