import { useEffect, useState } from 'react'
import { fetchSettings, putSettings } from '../../api/catalog'
import type { Settings, View } from '../../api/types'
import { useToasts } from '../../state/useToasts'
import { CleanRulesCard } from './CleanRulesCard'
import { ProtocolRegexCard } from './ProtocolRegexCard'
import { SnapshotsCard } from './SnapshotsCard'
import { VersionFilterCard } from './VersionFilterCard'
import { WhitelistCard } from './WhitelistCard'

type Props = {
  view: View | null
  onView: (view: View) => void
}

/**
 * All settings feed one pipeline, so they are saved as one object and the
 * server answers with the recomputed view. Saving here never touches CPA — it
 * only changes what the panel shows, which makes every card its own preview.
 *
 * The page also works with no view at all: when a stored regex stops
 * compiling the catalog cannot be built, and this is the only place to fix it.
 */
export function SettingsPage({ view, onView }: Props) {
  const { push } = useToasts()
  const [local, setLocal] = useState<Settings | null>(view?.settings ?? null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (view?.settings) setLocal(view.settings)
  }, [view?.settings])

  useEffect(() => {
    if (view) return
    void fetchSettings()
      .then((result) => setLocal(result.settings))
      .catch((error) => push('error', String((error as Error).message)))
  }, [push, view])

  if (!local) return <div className="page-placeholder muted">正在读取设置…</div>

  const save = async () => {
    setBusy(true)
    try {
      const result = await putSettings(local)
      onView(result.view)
      const excluded = result.view.stats.excluded
      push('ok', '设置已保存', {
        detail: excluded > 0 ? [`当前共 ${excluded} 个模型被规则排除，保存到 CPA 后才会真正删除`] : undefined,
      })
    } catch (error) {
      push('error', String((error as Error).message))
    } finally {
      setBusy(false)
    }
  }

  const shared = { settings: local, onChange: setLocal, onSave: () => void save(), busy }

  return (
    <div className="settings-scroll">
      <CleanRulesCard {...shared} />
      <WhitelistCard {...shared} view={view} />
      <VersionFilterCard {...shared} view={view} />
      <ProtocolRegexCard {...shared} view={view} />
      <SnapshotsCard onView={onView} />
    </div>
  )
}
