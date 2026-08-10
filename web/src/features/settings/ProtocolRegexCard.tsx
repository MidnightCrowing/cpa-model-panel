import { useMemo } from 'react'
import type { Settings, View } from '../../api/types'

type Props = {
  settings: Settings
  view: View | null
  onChange: (settings: Settings) => void
  onSave: () => void
  busy: boolean
}

const CODEX_EXAMPLE = '(?i)^gpt-\d+(?:[.-]\d+)*(?:-(?:luna|sol|terra|pro|compact))*$'

export function ProtocolRegexCard({ settings, view, onChange, onSave, busy }: Props) {
  const counts = useMemo(() => {
    const tally = { openai: 0, codex: 0, claude: 0 }
    for (const model of view?.models ?? []) tally[model.protocol]++
    return tally
  }, [view])

  const patch = (next: Partial<Settings['protocol']>) =>
    onChange({ ...settings, protocol: { ...settings.protocol, ...next } })

  return (
    <section className="card settings-card">
      <h2 className="card-title">协议标记正则</h2>
      <p className="card-subtitle">
        给每个模型打一个协议标记，用于命名页和启停页的筛选。<strong>只影响展示</strong>：已有模型永远写回它原本所在的
        CPA 列表，不会被搬来搬去。只有「刷新站点模型」发现一个全新模型时，才用这个标记决定它落到该站点的哪张表。
      </p>
      <p className="card-subtitle">
        匹配的是<strong>清洗之后的名字</strong>（有重映射就用重映射名，否则用清洗后的原始名），所以厂商前缀、
        <code>[free]</code>、<code>(xhigh)[1M]</code> 这类噪音不用写进正则。
      </p>
      <p className="card-subtitle">
        正则<strong>原样使用</strong>：留空表示不标记任何模型，写错直接报错，没有任何默认值兜底。Go 用 RE2，
        <strong>不支持 <code>(?!…)</code> 前瞻</strong>，「排除某些词」要正面写出想要的形状。
      </p>

      <label className="field">
        <span className="field-label">Codex 正则</span>
        <input
          className="input mono"
          value={settings.protocol.codex_regex}
          onChange={(event) => patch({ codex_regex: event.target.value })}
        />
        <button
          type="button"
          className="chip chip-action field-hint"
          title="填入一条只匹配正式 gpt 版本、排除 mini/image/nano/chat/audio/oss 的 RE2 正则"
          onClick={() => patch({ codex_regex: CODEX_EXAMPLE })}
        >
          用推荐正则填充
        </button>
      </label>

      <label className="field">
        <span className="field-label">Claude 正则</span>
        <input
          className="input mono"
          value={settings.protocol.claude_regex}
          onChange={(event) => patch({ claude_regex: event.target.value })}
        />
      </label>

      <div className="settings-actions">
        <button type="button" className="btn btn-primary" onClick={onSave} disabled={busy}>
          保存并重算
        </button>
        {view && (
          <span className="muted">
            当前分布：OpenAI {counts.openai} · Codex {counts.codex} · Claude {counts.claude}
          </span>
        )}
      </div>
    </section>
  )
}
