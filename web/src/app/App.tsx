import { useCallback, useState } from 'react'
import { clearToken, getToken } from '../api/client'
import { SavePreview } from '../components/SavePreview'
import { Toasts } from '../components/Toasts'
import { MatrixPage } from '../features/matrix/MatrixPage'
import { NamingPage } from '../features/naming/NamingPage'
import { SettingsPage } from '../features/settings/SettingsPage'
import { usePersistentState } from '../lib/hooks'
import { useCatalog } from '../state/useCatalog'
import { ToastProvider } from '../state/useToasts'
import { Header, type Tab } from './Header'
import { Login } from './Login'

export default function App() {
  return (
    <ToastProvider>
      <Shell />
    </ToastProvider>
  )
}

function Shell() {
  const [authed, setAuthed] = useState(() => !!getToken())
  const [tab, setTab] = usePersistentState<Tab>('panel.tab', 'naming')

  const onUnauthorized = useCallback(() => setAuthed(false), [])
  const catalog = useCatalog(authed, onUnauthorized)

  if (!authed) {
    return (
      <>
        <Login onSuccess={() => setAuthed(true)} />
        <Toasts />
      </>
    )
  }

  const { view, error, draft, dispatch, dirty, outOfSync, savable, loading, saving, refreshing } = catalog

  return (
    <div className="app-frame">
      <Toasts />
      <Header
        tab={tab}
        onTab={setTab}
        view={view}
        dirty={dirty}
        outOfSync={outOfSync}
        savable={savable}
        loading={loading}
        saving={saving}
        refreshing={refreshing}
        onRefresh={() => void catalog.refresh()}
        onSave={() => void catalog.requestPreview()}
        onDiscard={catalog.discard}
        onReload={() => void catalog.load()}
        onLogout={() => {
          clearToken()
          setAuthed(false)
        }}
      />

      <main className="app-main">
        {!view && tab !== 'settings' ? (
          <div className="page-placeholder">
            {loading ? (
              <span className="muted">正在读取 CPA 配置…</span>
            ) : (
              <div className="load-error">
                <div className="load-error-title">无法读取模型目录</div>
                <div className="mono load-error-message">{error ?? '未知错误'}</div>
                <button type="button" className="btn btn-secondary" onClick={() => setTab('settings')}>
                  去设置页修改
                </button>
                <button type="button" className="btn btn-ghost" onClick={() => void catalog.load()}>
                  重试
                </button>
              </div>
            )}
          </div>
        ) : tab === 'settings' ? (
          <SettingsPage view={view} onView={catalog.setView} />
        ) : tab === 'naming' ? (
          <NamingPage view={view!} draft={draft} dispatch={dispatch} saving={saving} />
        ) : (
          <MatrixPage view={view!} draft={draft} dispatch={dispatch} onView={catalog.setView} />
        )}
      </main>

      {catalog.preview && (
        <SavePreview
          diff={catalog.preview.diff}
          created={catalog.preview.created}
          moved={catalog.preview.moved}
          busy={saving}
          onConfirm={() => void catalog.save()}
          onCancel={catalog.cancelPreview}
        />
      )}
    </div>
  )
}
