import { useMemo } from 'react'
import type { SeriesThreshold, Settings, View } from '../../api/types'
import { Checkbox } from '../../components/Controls'
import { ExclusionSummary } from './WhitelistCard'

type Props = {
  settings: Settings
  view: View | null
  onChange: (settings: Settings) => void
  onSave: () => void
  busy: boolean
}

export function VersionFilterCard({ settings, view, onChange, onSave, busy }: Props) {
  const removed = useMemo(() => (view?.models ?? []).filter((model) => model.excluded === 'version'), [view])
  const version = settings.version

  const patch = (next: Partial<typeof version>) => onChange({ ...settings, version: { ...version, ...next } })

  const setThreshold = (index: number, next: Partial<SeriesThreshold>) => {
    const thresholds = version.thresholds.map((item, i) => (i === index ? { ...item, ...next } : item))
    patch({ thresholds })
  }

  return (
    <section className="card settings-card">
      <div className="card-head">
        <div>
          <h2 className="card-title">版本淘汰</h2>
          <p className="card-subtitle">
            版本号必须<strong>跟在系列名后面</strong>（最多隔一个词）：<code>gpt-4o</code> → 4、
            <code>claude-3-7-sonnet</code> → 3.7、<code>kimi-k2</code> → 2、<code>doubao-seed-1.6</code> → 1.6；
            而 <code>grok-code-fast-1</code> 不会被读成 grok 1。命中豁免正则的模型直接保留。
          </p>
        </div>
        <Checkbox checked={version.enabled} onChange={(enabled) => patch({ enabled })} label="启用" />
      </div>

      <div className="threshold-list">
        {version.thresholds.map((item, index) => (
          <div className="threshold-row" key={index}>
            <input
              className="input mono"
              placeholder="系列名，如 gpt"
              value={item.series}
              onChange={(event) => setThreshold(index, { series: event.target.value })}
            />
            <span className="muted">最低版本</span>
            <input
              className="input mono threshold-number"
              type="number"
              step="0.1"
              value={item.min_version}
              onChange={(event) => setThreshold(index, { min_version: Number.parseFloat(event.target.value) || 0 })}
            />
            <button
              type="button"
              className="btn btn-secondary btn-sm"
              onClick={() => patch({ thresholds: version.thresholds.filter((_, i) => i !== index) })}
            >
              移除
            </button>
          </div>
        ))}
        <button
          type="button"
          className="btn btn-secondary btn-sm threshold-add"
          onClick={() => patch({ thresholds: [...version.thresholds, { series: '', min_version: 1 }] })}
        >
          + 添加系列
        </button>
      </div>

      <label className="field">
        <span className="field-label">豁免正则（命中即保留）</span>
        <textarea
          className="textarea"
          rows={2}
          spellCheck={false}
          value={version.exempt_pattern}
          onChange={(event) => patch({ exempt_pattern: event.target.value })}
        />
      </label>

      <div className="settings-actions">
        <button type="button" className="btn btn-primary" onClick={onSave} disabled={busy}>
          保存并重算
        </button>
      </div>

      {view && <ExclusionSummary title="当前被版本规则排除" models={removed} />}
    </section>
  )
}
