import { useCallback, useEffect, useState } from 'react'
import { fetchAutoSync, putAutoSync } from '../../api/catalog'
import type { AutoSyncConfig, AutoSyncLog, AutoSyncPayload } from '../../api/types'
import { useToasts } from '../../state/useToasts'

export function AutoSyncCard() {
  const { push } = useToasts()
  const [payload, setPayload] = useState<AutoSyncPayload | null>(null)
  const [local, setLocal] = useState<AutoSyncConfig | null>(null)
  const [dirty, setDirty] = useState(false)
  const [busy, setBusy] = useState(false)

  const load = useCallback(
    async (notify = false) => {
      try {
        const result = await fetchAutoSync()
        setPayload(result)
        if (!dirty) setLocal(result.config)
        if (notify) push('ok', '定时任务状态已刷新')
      } catch (error) {
        push('error', String((error as Error).message))
      }
    },
    [dirty, push],
  )

  useEffect(() => {
    void load()
    const timer = window.setInterval(() => void load(), 15_000)
    return () => window.clearInterval(timer)
  }, [load])

  if (!local || !payload) {
    return (
      <section className="card settings-card">
        <h2 className="card-title">定时同步上游模型</h2>
        <div className="muted">正在读取定时任务设置…</div>
      </section>
    )
  }

  const update = (patch: Partial<AutoSyncConfig>) => {
    setLocal((current) => (current ? { ...current, ...patch } : current))
    setDirty(true)
  }

  const save = async () => {
    setBusy(true)
    try {
      const result = await putAutoSync(local)
      setPayload(result)
      setLocal(result.config)
      setDirty(false)
      push('ok', result.config.enabled ? '定时同步已开启' : '定时同步已关闭')
      // The scheduler publishes a freshly randomized next-run time after it
      // receives the settings wake-up, which is just after this response.
      window.setTimeout(() => void load(), 300)
    } catch (error) {
      push('error', String((error as Error).message))
    } finally {
      setBusy(false)
    }
  }

  const minimum = Math.max(1, local.interval_minutes - local.jitter_minutes)
  const maximum = local.interval_minutes + local.jitter_minutes
  const configuredEnabled = payload.config.enabled
  const status = payload.state.running
    ? '任务正在执行'
    : configuredEnabled && payload.state.next_run_at
      ? `下次执行：${formatTime(payload.state.next_run_at)}`
      : configuredEnabled
        ? '正在安排下次执行时间…'
        : '当前已关闭'

  return (
    <section className="card settings-card">
      <div className="auto-sync-heading">
        <div>
          <h2 className="card-title">定时同步上游模型</h2>
          <p className="card-subtitle">
            到时自动执行“拉取上游模型 → 应用全部命名建议 → 保存到 CPA”。不会新增、取消或修改站点启停项。
          </p>
        </div>
        <span className={`badge ${payload.state.running || configuredEnabled ? 'is-ok' : ''}`}>{status}</span>
      </div>

      <label className="checkbox settings-toggle">
        <input type="checkbox" checked={local.enabled} onChange={(event) => update({ enabled: event.target.checked })} />
        <span>启用定时同步</span>
      </label>

      <div className="settings-grid-2">
        <label className="field">
          <span className="field-label">基础间隔（分钟）</span>
          <input
            className="input"
            type="number"
            min={1}
            max={525600}
            step={1}
            value={local.interval_minutes}
            onChange={(event) => update({ interval_minutes: Number(event.target.value) })}
          />
        </label>
        <label className="field">
          <span className="field-label">随机误差（± 分钟）</span>
          <input
            className="input"
            type="number"
            min={0}
            max={Math.max(0, local.interval_minutes - 1)}
            step={1}
            value={local.jitter_minutes}
            onChange={(event) => update({ jitter_minutes: Number(event.target.value) })}
          />
        </label>
      </div>

      <div className="muted">
        每轮完成后随机等待 <strong>{minimum}</strong>–<strong>{maximum}</strong> 分钟，再执行下一轮；修改设置会重新抽取等待时间。
      </div>

      <div className="settings-actions">
        <button type="button" className="btn btn-primary" disabled={busy || !dirty} onClick={() => void save()}>
          {busy ? '保存中…' : '保存定时设置'}
        </button>
        <button type="button" className="btn btn-ghost" disabled={busy} onClick={() => void load(true)}>
          刷新状态与日志
        </button>
        {dirty && <span className="muted">定时设置尚未保存</span>}
      </div>

      <AutoSyncLogs logs={payload.logs} />
    </section>
  )
}

function AutoSyncLogs({ logs }: { logs: AutoSyncLog[] }) {
  return (
    <details className="settings-summary" open={logs.length > 0}>
      <summary>定时任务日志（最近 {logs.length} 次）</summary>
      {logs.length === 0 ? (
        <div className="muted auto-sync-empty">还没有执行记录</div>
      ) : (
        <div className="auto-sync-logs">
          {logs.map((entry) => (
            <AutoSyncLogEntry entry={entry} key={entry.id} />
          ))}
        </div>
      )}
    </details>
  )
}

function AutoSyncLogEntry({ entry }: { entry: AutoSyncLog }) {
  const failures = (entry.failures ?? []).map(parseFailure)
  const failed = Math.max(entry.failed, failures.length)

  return (
    <details className={`auto-sync-log-row is-${entry.status}`} open={false}>
      <summary className="auto-sync-log-header">
        <span className="auto-sync-log-chevron" aria-hidden="true">›</span>
        <span className="mono auto-sync-log-id">#{entry.id}</span>
        <time dateTime={entry.started_at}>{formatTime(entry.started_at)}</time>
        <LogStatus entry={entry} />
        <LogSummary entry={entry} failed={failed} />
      </summary>

      <div className="auto-sync-log-details">
        {entry.error && (
          <div className="auto-sync-run-error">
            <strong>任务执行失败</strong>
            <div>{entry.error}</div>
          </div>
        )}

        {failures.length > 0 && (
          <ul className="auto-sync-failures">
            {failures.map((failure, index) => (
              <li className="auto-sync-failure" key={`${failure.site}-${index}`}>
                <strong className="auto-sync-failure-site">{failure.site}</strong>
                {failure.status && (
                  <span className={`auto-sync-http-status is-${httpTone(failure.status)}`}>HTTP {failure.status}</span>
                )}
                {failure.endpoint && <span className="mono auto-sync-failure-endpoint">{failure.endpoint}</span>}
                <span className="auto-sync-failure-message">{failure.message}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </details>
  )
}

function LogSummary({ entry, failed }: { entry: AutoSyncLog; failed: number }) {
  return (
    <span className="auto-sync-log-summary">
      拉取 <strong className="is-info">{entry.refreshed}</strong> 站点
      {' · '}新增 <strong className="is-success">+{entry.added}</strong>
      {' · '}下线 <strong className={entry.dropped > 0 ? 'is-danger' : ''}>-{entry.dropped}</strong>
      {' · '}映射 <strong className="is-info">{entry.renamed}</strong>
      {entry.restored > 0 && <><span> · </span>写入 <strong className="is-success">+{entry.restored}</strong></>}
      {entry.removed > 0 && <><span> · </span>移除 <strong className="is-danger">-{entry.removed}</strong></>}
      {failed > 0 && <span className="auto-sync-failure-count"> · 失败 {failed}</span>}
    </span>
  )
}

function LogStatus({ entry }: { entry: AutoSyncLog }) {
  if (entry.status === 'success') return <span className="badge is-ok">成功</span>
  if (entry.status === 'partial') return <span className="badge is-warn">部分成功</span>
  return <span className="chip chip-danger">失败</span>
}

type ParsedFailure = {
  site: string
  status?: number
  endpoint?: string
  message: string
}

function parseFailure(value: string): ParsedFailure {
  const separator = value.indexOf('：')
  const site = separator >= 0 ? value.slice(0, separator).trim() : '未知站点'
  const detail = (separator >= 0 ? value.slice(separator + 1) : value).trim()
  const match = detail.match(/^HTTP\s+(\d{3})\s+from\s+(\S+):\s*([\s\S]*)$/i)
  if (!match) return { site, message: detail }
  return {
    site,
    status: Number(match[1]),
    endpoint: match[2],
    message: match[3] || '上游没有返回错误正文',
  }
}

function httpTone(status: number) {
  if (status >= 500) return 'danger'
  if (status >= 400) return 'warn'
  return 'info'
}

function formatTime(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}
