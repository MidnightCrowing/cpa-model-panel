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
        协议标记决定模型属于 CPA 的哪张表。开启下面的归位开关后，<strong>启停页显示什么，CPA 对应的表就是什么</strong>
        ——不多写一个，也不少写一个：gpt 进 codex-api-key，claude 进 claude-api-key，其余进 openai-compatibility。
      </p>

      <label className="checkbox settings-toggle" title="关闭后模型一律留在它原本所在的表里，保存不会跨表搬运">
        <input
          type="checkbox"
          checked={settings.route_by_protocol ?? true}
          onChange={(event) => onChange({ ...settings, route_by_protocol: event.target.checked })}
        />
        <span>按协议归位（CPA 的三张表 = 启停页的投影）</span>
      </label>
      <p className="card-subtitle">
        站点在目标表里还没有条目时会<strong>自动新建</strong>一条，沿用该站点的 base-url 与密钥
        （claude 表用裸域名，另外两张用 <code>/v1</code>），不会因为「目标表没有这个站」就把模型丢掉。
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
