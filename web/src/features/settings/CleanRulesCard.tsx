import type { Settings } from '../../api/types'

type Props = {
  settings: Settings
  onChange: (settings: Settings) => void
  onSave: () => void
  busy: boolean
}

const listToText = (list: string[]) => list.join('\n')
const textToList = (text: string) =>
  text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)

export function CleanRulesCard({ settings, onChange, onSave, busy }: Props) {
  const rewrites = settings.rewrites ?? []

  const setRewrite = (index: number, next: Partial<{ pattern: string; replace: string }>) =>
    onChange({ ...settings, rewrites: rewrites.map((item, i) => (i === index ? { ...item, ...next } : item)) })

  return (
    <section className="card settings-card">
      <h2 className="card-title">名称清洗规则</h2>
      <p className="card-subtitle">
        用于生成「建议」名字，也决定协议标记匹配的对象。顺序是：剥前缀 → 取路径末段 → 转小写 → 剥后缀 → 重写。
        清洗只产生建议，不会自动改名。
      </p>

      <div className="settings-grid-2">
        <label className="field">
          <span className="field-label">前缀（{settings.prefixes.length} 条，每行一个，最长的先匹配）</span>
          <textarea
            className="textarea"
            rows={10}
            spellCheck={false}
            value={listToText(settings.prefixes)}
            onChange={(event) => onChange({ ...settings, prefixes: textToList(event.target.value) })}
          />
        </label>

        <label className="field">
          <span className="field-label">后缀（{settings.suffixes.length} 条）</span>
          <textarea
            className="textarea"
            rows={10}
            spellCheck={false}
            value={listToText(settings.suffixes)}
            onChange={(event) => onChange({ ...settings, suffixes: textToList(event.target.value) })}
          />
        </label>
      </div>

      <label className="field">
        <span className="field-label">保护正则：名字一旦匹配就停止剥后缀</span>
        <input
          className="input mono"
          spellCheck={false}
          placeholder="(?i)^qwen.*-max$"
          value={settings.protect ?? ''}
          onChange={(event) => onChange({ ...settings, protect: event.target.value })}
        />
        <span className="muted">
          <code>-max</code> 对 <code>gpt-5.6-luna-max</code> 是噪音，对 <code>qwen3-max</code> 却是官方名字的一部分。
          每剥掉一个后缀都会重新检查，所以 <code>qwen3.8-max-preview</code> 会先掉 <code>-preview</code>，然后停在
          <code>qwen3.8-max</code>。
        </span>
      </label>

      <div className="field">
        <span className="field-label">重写规则：剥完后缀后按顺序做正则替换</span>
        <div className="threshold-list">
          {rewrites.map((rule, index) => (
            <div className="rewrite-row" key={index}>
              <input
                className="input mono"
                spellCheck={false}
                placeholder={String.raw`^gpt-(\d+)-(\d+)`}
                value={rule.pattern}
                onChange={(event) => setRewrite(index, { pattern: event.target.value })}
              />
              <span className="muted">→</span>
              <input
                className="input mono"
                spellCheck={false}
                placeholder="gpt-${1}.${2}"
                value={rule.replace}
                onChange={(event) => setRewrite(index, { replace: event.target.value })}
              />
              <button
                type="button"
                className="btn btn-secondary btn-sm"
                onClick={() => onChange({ ...settings, rewrites: rewrites.filter((_, i) => i !== index) })}
              >
                移除
              </button>
            </div>
          ))}
          <button
            type="button"
            className="btn btn-secondary btn-sm threshold-add"
            onClick={() => onChange({ ...settings, rewrites: [...rewrites, { pattern: '', replace: '' }] })}
          >
            + 添加重写规则
          </button>
        </div>
        <span className="muted">
          点位用 <code>{'${1}'}</code>、<code>{'${2}'}</code>（带花括号，避免 <code>$12</code> 被当成第 12 组）。
          破折号还是点号是各家自己的写法：<code>gpt-5-5</code> 其实是 gpt-5.5，而 <code>claude-haiku-4-5</code>
          官方就是这么写的，所以只写你要改的那一家。
        </span>
      </div>

      <div className="settings-actions">
        <button type="button" className="btn btn-primary" onClick={onSave} disabled={busy}>
          保存并重算建议
        </button>
      </div>
    </section>
  )
}
