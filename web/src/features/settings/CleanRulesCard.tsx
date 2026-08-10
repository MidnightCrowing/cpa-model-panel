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
  return (
    <section className="card settings-card">
      <h2 className="card-title">名称清洗规则</h2>
      <p className="card-subtitle">
        用于生成「建议」名字。每行一条，最长的先匹配。清洗只产生建议，不会自动改名。
      </p>

      <div className="settings-grid-2">
        <label className="field">
          <span className="field-label">前缀（{settings.prefixes.length} 条）</span>
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

      <div className="settings-actions">
        <button type="button" className="btn btn-primary" onClick={onSave} disabled={busy}>
          保存并重算建议
        </button>
      </div>
    </section>
  )
}
