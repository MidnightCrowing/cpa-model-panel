import type { ChannelDiff, Conflict } from '../api/types'

const CHANNEL_LABEL: Record<string, string> = {
  openai: 'openai-compatibility',
  codex: 'codex-api-key',
  claude: 'claude-api-key',
}

type Props = {
  diff: ChannelDiff[]
  conflicts: Conflict[]
  created: string[] | null
  moved: number
  busy: boolean
  onConfirm: () => void
  onCancel: () => void
}

/**
 * What the save is about to do, produced by the same code that will do it.
 *
 * A routing change can move hundreds of entries between CPA's lists; a count
 * alone is not enough to press the button with confidence.
 */
export function SavePreview({ diff, conflicts, created, moved, busy, onConfirm, onCancel }: Props) {
  const total = diff.reduce((sum, item) => sum + item.added.length + item.removed.length + item.renamed.length, 0)

  return (
    <div className="modal-backdrop" onClick={onCancel}>
      <div className="modal preview-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <h3>保存前确认</h3>
          <button type="button" className="btn btn-secondary btn-sm" onClick={onCancel}>
            取消
          </button>
        </div>

        {total === 0 && conflicts.length === 0 ? (
          <p className="muted">CPA 配置无需改动。</p>
        ) : (
          <p className="muted">
            共 {total} 处改动
            {moved > 0 ? ` · 其中 ${moved} 个模型按协议归位到另一张表` : ''}
            {created?.length ? ` · 新建 ${created.length} 个站点条目` : ''}
          </p>
        )}

        {conflicts.length > 0 && (
          <section className="preview-conflicts">
            <h4>⚠ {conflicts.length} 处命名冲突</h4>
            <p className="muted">同一个站点里多个原始模型会被写成同一个名字，CPA 两条都会留着，实际用哪条不确定。</p>
            <div className="preview-list mono">
              {conflicts.map((conflict) => (
                <div key={`${conflict.site}|${conflict.channel}|${conflict.name}`}>
                  {conflict.site} · {CHANNEL_LABEL[conflict.channel] ?? conflict.channel} · {conflict.name} ←{' '}
                  {conflict.upstreams.join(' , ')}
                </div>
              ))}
            </div>
          </section>
        )}

        {diff.map((item) => {
          const count = item.added.length + item.removed.length + item.renamed.length
          if (count === 0) return null
          return (
            <details className="preview-channel" key={item.channel} open={count <= 40}>
              <summary>
                {CHANNEL_LABEL[item.channel] ?? item.channel}
                <span className="muted">
                  {' '}
                  +{item.added.length} −{item.removed.length}
                  {item.renamed.length > 0 ? ` ✎${item.renamed.length}` : ''}
                  {item.new_providers.length > 0 ? ` · 新建 ${item.new_providers.length} 个条目` : ''}
                </span>
              </summary>
              <div className="preview-list mono">
                {item.new_providers.map((label) => (
                  <div className="is-new" key={`p:${label}`}>
                    ＋条目 {label}
                  </div>
                ))}
                {item.added.map((label) => (
                  <div className="is-add" key={`a:${label}`}>
                    + {label}
                  </div>
                ))}
                {item.removed.map((label) => (
                  <div className="is-remove" key={`r:${label}`}>
                    − {label}
                  </div>
                ))}
                {item.renamed.map((label) => (
                  <div className="is-rename" key={`n:${label}`}>
                    ✎ {label}
                  </div>
                ))}
              </div>
            </details>
          )
        })}

        <div className="settings-actions">
          <button type="button" className="btn btn-primary" disabled={busy} onClick={onConfirm}>
            {busy ? '写入中…' : '确认写入 CPA'}
          </button>
          <button type="button" className="btn btn-ghost" onClick={onCancel}>
            再看看
          </button>
          <span className="muted">写入前会自动存快照，可在设置页回滚</span>
        </div>
      </div>
    </div>
  )
}
