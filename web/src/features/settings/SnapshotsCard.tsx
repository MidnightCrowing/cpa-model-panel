import { useCallback, useEffect, useState } from 'react'
import { fetchSnapshots, rollbackSnapshot } from '../../api/catalog'
import type { SnapshotMeta, View } from '../../api/types'
import { useToasts } from '../../state/useToasts'

type Props = {
  onView: (view: View) => void
}

export function SnapshotsCard({ onView }: Props) {
  const { push } = useToasts()
  const [snapshots, setSnapshots] = useState<SnapshotMeta[]>([])
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      const result = await fetchSnapshots()
      setSnapshots(result.snapshots || [])
    } catch (error) {
      push('error', String((error as Error).message))
    }
  }, [push])

  useEffect(() => {
    void load()
  }, [load])

  const rollback = async (snapshot: SnapshotMeta) => {
    if (!window.confirm(`回滚到快照 #${snapshot.id}（${snapshot.created_at}）？当前配置会先另存一份快照。`)) return
    setBusy(true)
    try {
      const result = await rollbackSnapshot(snapshot.id)
      if (result.view) onView(result.view)
      push('ok', `已回滚到快照 #${snapshot.id}`, { detail: ['当前配置已另存为 pre-rollback 快照'] })
      await load()
    } catch (error) {
      push('error', String((error as Error).message))
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="card settings-card">
      <h2 className="card-title">配置快照</h2>
      <p className="card-subtitle">每次写回 CPA 前自动保存三张表的完整内容，回滚会把它们原样写回去。</p>

      <div className="snapshot-list">
        {snapshots.map((snapshot) => (
          <div className="snapshot-row" key={snapshot.id}>
            <span className="mono">#{snapshot.id}</span>
            <span className="mono muted">{snapshot.created_at.replace('T', ' ').replace('Z', '')}</span>
            <span>{snapshot.note}</span>
            <button type="button" className="btn btn-secondary btn-sm" disabled={busy} onClick={() => void rollback(snapshot)}>
              回滚
            </button>
          </div>
        ))}
        {snapshots.length === 0 && <div className="muted">暂无快照</div>}
      </div>
    </section>
  )
}
