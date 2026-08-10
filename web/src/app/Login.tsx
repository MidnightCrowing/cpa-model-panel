import { useState, type FormEvent } from 'react'
import { login } from '../api/catalog'
import { setToken } from '../api/client'

export function Login({ onSuccess }: { onSuccess: () => void }) {
  const [token, setLocalToken] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setError('')
    setBusy(true)
    try {
      const result = await login(token.trim())
      setToken(result.token)
      onSuccess()
    } catch (err) {
      setError(String((err as Error).message))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="login-shell">
      <form className="card login-card" onSubmit={submit}>
        <p className="login-eyebrow">CPA MODEL PANEL</p>
        <h1>管理员登录</h1>
        <p className="login-desc">管理跨站点的模型命名映射与按站点启停。</p>

        <label htmlFor="admin-token">Admin Token</label>
        <input
          id="admin-token"
          className="input"
          type="password"
          autoFocus
          value={token}
          placeholder="ADMIN_TOKEN"
          onChange={(event) => setLocalToken(event.target.value)}
        />
        {error && <div className="login-error">{error}</div>}
        <button className="btn btn-primary btn-full" type="submit" disabled={busy}>
          {busy ? '登录中…' : '登录'}
        </button>
      </form>
    </div>
  )
}
