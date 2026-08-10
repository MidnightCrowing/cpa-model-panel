import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react'

export type ToastLevel = 'ok' | 'error' | 'info'

export type Toast = {
  id: number
  level: ToastLevel
  text: string
  detail?: string[]
  /** Sticky toasts wait for the user; they carry information worth reading. */
  sticky: boolean
}

type PushOptions = {
  detail?: string[]
  sticky?: boolean
}

type ToastApi = {
  toasts: Toast[]
  push: (level: ToastLevel, text: string, options?: PushOptions) => void
  dismiss: (id: number) => void
}

const ToastContext = createContext<ToastApi | null>(null)

const AUTO_DISMISS_MS = 3500

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const nextId = useRef(1)
  const timers = useRef(new Map<number, number>())

  const dismiss = useCallback((id: number) => {
    setToasts((current) => current.filter((toast) => toast.id !== id))
    const timer = timers.current.get(id)
    if (timer) {
      window.clearTimeout(timer)
      timers.current.delete(id)
    }
  }, [])

  const push = useCallback(
    (level: ToastLevel, text: string, options: PushOptions = {}) => {
      if (!text) return
      const id = nextId.current++
      // Errors and anything with details stay until dismissed; a bare
      // "已保存" does not deserve a click.
      const sticky = options.sticky ?? (level === 'error' || (options.detail?.length ?? 0) > 0)
      setToasts((current) => [...current, { id, level, text, detail: options.detail, sticky }])
      if (!sticky) {
        timers.current.set(id, window.setTimeout(() => dismiss(id), AUTO_DISMISS_MS))
      }
    },
    [dismiss],
  )

  const value = useMemo(() => ({ toasts, push, dismiss }), [toasts, push, dismiss])
  return <ToastContext.Provider value={value}>{children}</ToastContext.Provider>
}

export function useToasts(): ToastApi {
  const context = useContext(ToastContext)
  if (!context) throw new Error('useToasts must be used inside ToastProvider')
  return context
}
