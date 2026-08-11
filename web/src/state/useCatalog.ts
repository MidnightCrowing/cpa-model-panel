import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { fetchView, previewOps, refreshCatalog, saveOps } from '../api/catalog'
import { ApiError, clearToken } from '../api/client'
import type { SavePreview, View } from '../api/types'
import { baselineOf, buildOps, countChanges, emptyDraft, useDraft } from './useDraft'
import { useToasts } from './useToasts'

export type RefreshState = { completed: number; total: number } | null

export function useCatalog(enabled: boolean, onUnauthorized: () => void) {
  const { push } = useToasts()
  const [view, setView] = useState<View | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [refreshing, setRefreshing] = useState<RefreshState>(null)
  const [error, setError] = useState<string | null>(null)
  const [preview, setPreview] = useState<SavePreview | null>(null)
  const [draft, dispatch] = useDraft()

  const fail = useCallback(
    (error: unknown) => {
      if (error instanceof ApiError && error.status === 401) {
        clearToken()
        onUnauthorized()
        return
      }
      push('error', String((error as Error)?.message ?? error))
    },
    [onUnauthorized, push],
  )

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setView(await fetchView())
      setError(null)
      dispatch({ kind: 'reset' })
    } catch (err) {
      // Keep the message on screen: a rejected setting (a regex that does not
      // compile) can only be fixed from the settings page, so the shell has to
      // stay usable.
      setError(String((err as Error)?.message ?? err))
      fail(err)
    } finally {
      setLoading(false)
    }
  }, [dispatch, fail])

  useEffect(() => {
    if (enabled) void load()
  }, [enabled, load])


  const baseline = useMemo(
    () => baselineOf(view?.models ?? [], view?.sites ?? []),
    [view],
  )

  const dirty = useMemo(() => countChanges(draft, baseline), [draft, baseline])

  const savingRef = useRef(false)
  const refreshingRef = useRef(false)
  savingRef.current = saving
  refreshingRef.current = refreshing !== null
  // CPA can be edited elsewhere, so coming back to the tab re-reads it. Skipped
  // while a draft is open: silently replacing the view would throw away edits.
  const hasDraft = dirty > 0
  useEffect(() => {
    if (!enabled) return
    const onFocus = () => {
      if (document.visibilityState !== 'visible') return
      if (hasDraft || savingRef.current || refreshingRef.current) return
      void load()
    }
    window.addEventListener('focus', onFocus)
    document.addEventListener('visibilitychange', onFocus)
    return () => {
      window.removeEventListener('focus', onFocus)
      document.removeEventListener('visibilitychange', onFocus)
    }
  }, [enabled, hasDraft, load])

  // A refresh can leave work to do without any draft edit: newly discovered
  // models reach CPA only on save, and models the rules exclude are still in
  // CPA until then. The save button has to stay live for those.
  const outOfSync = (view?.stats.to_add ?? 0) + (view?.stats.to_remove ?? 0) + (view?.stats.to_move ?? 0)
  const savable = dirty > 0 || outOfSync > 0

  // Saving is two steps: ask the server what it would do, show it, then let
  // the user commit. A routing change can move hundreds of entries.
  const requestPreview = useCallback(async () => {
    if (!view) return
    const ops = buildOps(draft, baseline)
    if (ops.length === 0 && outOfSync === 0) {
      push('info', '没有需要保存的变更')
      return
    }
    setSaving(true)
    try {
      setPreview(await previewOps(view.fingerprint, ops))
    } catch (error) {
      fail(error)
    } finally {
      setSaving(false)
    }
  }, [baseline, draft, fail, outOfSync, push, view])

  const cancelPreview = useCallback(() => setPreview(null), [])

  const save = useCallback(async () => {
    if (!view) return
    const ops = buildOps(draft, baseline)
    if (ops.length === 0 && outOfSync === 0) {
      push('info', '没有需要保存的变更')
      return
    }
    setSaving(true)
    try {
      const result = await saveOps(view.fingerprint, ops)
      setView(result.view)
      dispatch({ kind: 'reset' })

      const detail: string[] = []
      if (result.written?.length) detail.push(`已写回：${result.written.join('、')}`)
      if (result.restored > 0) detail.push(`新增/恢复 ${result.restored} 个模型`)
      else detail.push('CPA 配置无需改动')
      if (result.removed > 0) detail.push(`从 CPA 移除 ${result.removed} 个模型`)
      if (result.moved > 0) detail.push(`按协议归位 ${result.moved} 个模型`)
      if (result.created?.length) detail.push(`新建站点条目：${result.created.join('、')}`)
      if (result.renamed > 0 && !result.written?.length) {
        detail.push(`${result.renamed} 处改名已记录在面板；这些模型当前不写入 CPA，放开规则后会带着新名字回去`)
      }
      if (result.skipped > 0) detail.push(`${result.skipped} 项目标已不存在，已跳过`)
      if (result.snapshot) detail.push(`回滚快照 #${result.snapshot}`)
      push('ok', '已保存到 CPA', { detail })
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) {
        push('error', 'CPA 配置已在面板之外发生变化，请重新载入后再保存')
      } else {
        fail(error)
      }
    } finally {
      setSaving(false)
      setPreview(null)
    }
  }, [baseline, dispatch, draft, fail, outOfSync, push, view])

  const refresh = useCallback(async () => {
    setRefreshing({ completed: 0, total: 0 })
    try {
      const result = await refreshCatalog(
        (total) => setRefreshing({ completed: 0, total }),
        (progress) => setRefreshing({ completed: progress.completed, total: progress.total }),
      )
      setView(result.view)
      dispatch({ kind: 'reset' })

      const detail: string[] = [`成功拉取 ${result.refreshed} 个站点，新增 ${result.added} 个模型`]
      if (result.added > 0) detail.push('新模型标记为「待写入」，点「保存到 CPA」后才会写入')
      for (const failure of result.failed) detail.push(`✕ ${failure.name}：${failure.error}`)
      if (result.failed.length > 0) detail.push('失败的站点保留原有模型列表，不会被清空')
      push(result.failed.length > 0 ? 'info' : 'ok', '站点模型刷新完成', { detail, sticky: true })
    } catch (error) {
      fail(error)
    } finally {
      setRefreshing(null)
    }
  }, [dispatch, fail, push])

  const discard = useCallback(() => dispatch({ kind: 'reset' }), [dispatch])

  return {
    view,
    setView,
    error,
    loading,
    saving,
    refreshing,
    draft,
    dispatch,
    baseline,
    dirty,
    outOfSync,
    savable,
    load,
    save,
    preview,
    requestPreview,
    cancelPreview,
    refresh,
    discard,
    emptyDraft,
  }
}
