import { useToasts } from '../state/useToasts'

/** Floating toast stack. Never occupies layout space, so nothing jumps. */
export function Toasts() {
  const { toasts, dismiss } = useToasts()
  if (toasts.length === 0) return null

  return (
    <div className="toast-container">
      {toasts.map((toast) => (
        <div key={toast.id} className={`toast is-${toast.level}`}>
          <div className="toast-body">
            <div className="toast-text">{toast.text}</div>
            {toast.detail && toast.detail.length > 0 && (
              <ul className="toast-detail">
                {toast.detail.map((line, index) => (
                  <li key={index}>{line}</li>
                ))}
              </ul>
            )}
          </div>
          <button type="button" className="toast-close" onClick={() => dismiss(toast.id)} aria-label="关闭通知">
            ✕
          </button>
        </div>
      ))}
    </div>
  )
}
