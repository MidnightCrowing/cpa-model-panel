import { useState } from 'react'
import { addEgg } from '../../api/catalog'
import type { View } from '../../api/types'
import { useToasts } from '../../state/useToasts'

/**
 * Add a 鸡蛋: an endpoint shared somewhere, usable for a while.
 *
 * The key is normally base64 and the URL may or may not carry /v1, so both are
 * normalised server-side and the result is shown back before anything sticks.
 * The endpoint is probed first, so a dead link fails here instead of becoming
 * a provider entry that only errors later.
 */
export function EggDialog({ onClose, onView }: { onClose: () => void; onView: (view: View) => void }) {
  const { push } = useToasts()
  const [url, setUrl] = useState('')
  const [key, setKey] = useState('')
  const [name, setName] = useState('')
  const [source, setSource] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async () => {
    setBusy(true)
    try {
      const result = await addEgg({ url, key, name: name.trim(), source_url: source.trim() })
      onView(result.view)
      push('ok', `已添加 ${result.site}`, {
        detail: [
          `拉到 ${result.models.length} 个模型：${result.models.slice(0, 8).join('、')}${result.models.length > 8 ? ' …' : ''}`,
          result.decoded ? `key 已 base64 解码后使用（${result.key_used}）` : `key 原样使用（${result.key_used}）`,
          '优先级已设为高于所有固定站点',
        ],
        sticky: true,
      })
      onClose()
    } catch (error) {
      push('error', String((error as Error).message))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal egg-modal" onClick={(event) => event.stopPropagation()}>
        <div className="modal-header">
          <h3>添加鸡蛋</h3>
          <button type="button" className="btn btn-secondary btn-sm" onClick={onClose}>
            关闭
          </button>
        </div>

        <label className="field">
          <span className="field-label">接口地址</span>
          <input
            className="input mono"
            autoFocus
            placeholder="https://example.com  或  https://example.com/v1"
            value={url}
            onChange={(event) => setUrl(event.target.value)}
          />
          <span className="muted">带不带 /v1 都行，粘贴 chat/completions 或 models 结尾的也会自动截掉。</span>
        </label>

        <label className="field">
          <span className="field-label">Key</span>
          <input
            className="input mono"
            placeholder="sk-… 或 base64"
            value={key}
            onChange={(event) => setKey(event.target.value)}
          />
          <span className="muted">是 base64 就自动解码，解不出来就原样使用；添加后会告诉你实际用的是哪个。</span>
        </label>

        <div className="settings-grid-2">
          <label className="field">
            <span className="field-label">名字（留空自动编号 鸡蛋N）</span>
            <input className="input" value={name} onChange={(event) => setName(event.target.value)} />
          </label>
          <label className="field">
            <span className="field-label">来源链接（可选）</span>
            <input
              className="input mono"
              placeholder="帖子地址，方便以后回去看"
              value={source}
              onChange={(event) => setSource(event.target.value)}
            />
          </label>
        </div>

        <div className="settings-actions">
          <button type="button" className="btn btn-primary" disabled={busy || !url.trim() || !key.trim()} onClick={() => void submit()}>
            {busy ? '正在探测…' : '探测并添加'}
          </button>
          <span className="muted">会先拉一次 /models 验证，成功才写入 CPA（写入前自动存快照）</span>
        </div>
      </div>
    </div>
  )
}
