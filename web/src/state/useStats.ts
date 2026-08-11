import { useEffect, useRef, useState } from 'react'
import { fetchStats } from '../api/catalog'
import type { StatCell, StatsResult } from '../api/types'
import { refKey } from '../lib/keys'

const POLL_MS = 30_000

export type StatsIndex = {
  configured: boolean
  reason?: string
  updatedAt?: string
  range: string
  /** Keyed by (site, upstream model). */
  byModel: Map<string, StatCell>
  loading: boolean
}

const EMPTY: StatsIndex = { configured: false, range: '24h', byModel: new Map(), loading: false }

/**
 * Request outcomes from Keeper, refreshed while the page is open.
 *
 * Polling stops when the tab is hidden — the numbers are only worth anything
 * while somebody is looking at them.
 */
export function useStats(active: boolean, range = '24h'): StatsIndex {
  const [state, setState] = useState<StatsIndex>(EMPTY)
  const inFlight = useRef(false)

  useEffect(() => {
    if (!active) return
    let cancelled = false

    const pull = async () => {
      if (inFlight.current || document.visibilityState !== 'visible') return
      inFlight.current = true
      setState((prev) => ({ ...prev, loading: true }))
      try {
        const result: StatsResult = await fetchStats(range)
        if (cancelled) return
        const byModel = new Map<string, StatCell>()
        for (const cell of result.cells ?? []) byModel.set(refKey(cell.site, cell.model), cell)
        setState({
          configured: result.configured,
          reason: result.reason,
          updatedAt: result.updated_at,
          range,
          byModel,
          loading: false,
        })
      } catch {
        // A statistics outage must not disturb the page it decorates.
        if (!cancelled) setState((prev) => ({ ...prev, loading: false }))
      } finally {
        inFlight.current = false
      }
    }

    void pull()
    const timer = window.setInterval(() => void pull(), POLL_MS)
    document.addEventListener('visibilitychange', pull)
    return () => {
      cancelled = true
      window.clearInterval(timer)
      document.removeEventListener('visibilitychange', pull)
    }
  }, [active, range])

  return state
}
