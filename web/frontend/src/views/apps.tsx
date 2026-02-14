import { useState, useEffect, useCallback } from 'react'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { api } from '../api'
import type { AppSummary, AppDetail, AppInput, AppStatusResponse, GitHubStatus, Install } from '../types'
import { Center, BackLink, Badge, RibbonStack, InfoCard, InfoRow } from '../components/ui'
import { TestInstallModal } from '../components/LoginModal'
import { InstallWizard } from './install-wizard'
import { groupInputs } from '../lib/format'

export function AppList() {
  const [apps, setApps] = useState<AppSummary[]>([])
  const [categories, setCategories] = useState<string[]>([])
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState('')
  const [loading, setLoading] = useState(true)

  const fetchApps = useCallback(async () => {
    setLoading(true)
    try {
      const params: { q?: string; category?: string } = {}
      if (search) params.q = search
      if (category) params.category = category
      const data = await api.apps(params)
      setApps(data.apps || [])
    } catch { setApps([]) }
    setLoading(false)
  }, [search, category])

  useEffect(() => { fetchApps() }, [fetchApps])
  useEffect(() => { api.categories().then(d => setCategories(d.categories || [])) }, [])

  return (
    <div>
      <div className="flex gap-3 mb-6 flex-wrap">
        <input type="text" placeholder="Search apps..." value={search} onChange={e => setSearch(e.target.value)}
          className="flex-1 min-w-[200px] px-4 py-2.5 bg-bg-secondary border border-border rounded-lg text-text-primary text-sm outline-none focus:border-primary focus:ring-1 focus:ring-primary transition-colors font-mono placeholder:text-text-muted" />
        <select value={category} onChange={e => setCategory(e.target.value)}
          className="px-4 py-2.5 bg-bg-secondary border border-border rounded-lg text-text-primary text-sm outline-none focus:border-primary font-mono cursor-pointer">
          <option value="">All Categories</option>
          {categories.map(c => <option key={c} value={c}>{c}</option>)}
        </select>
      </div>
      {loading ? <Center>Loading...</Center> : apps.length === 0 ? <Center>No apps found</Center> : (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(300px,1fr))] gap-4">
          {apps.map(app => <AppCard key={app.id} app={app} />)}
        </div>
      )}
    </div>
  )
}

export function AppCard({ app }: { app: AppSummary }) {
  return (
    <a href={`#/app/${app.id}`} className="relative block bg-bg-card border border-border rounded-lg p-5 no-underline text-inherit transition-all duration-200 hover:border-border-hover hover:shadow-[0_0_20px_rgba(0,255,157,0.15)] hover:-translate-y-0.5 group overflow-hidden">
      <RibbonStack ribbons={[
        ...(app.featured ? [{ label: 'featured', color: 'bg-status-featured' }] : []),
        ...(app.source === 'developer' ? [{ label: 'DEV', color: 'bg-yellow-400' }] : app.official ? [{ label: 'official', color: 'bg-primary' }] : []),
      ]} />
      <div className="flex items-start gap-4">
        <div className="w-12 h-12 rounded-lg bg-bg-secondary flex items-center justify-center text-xl shrink-0 overflow-hidden">
          {app.has_icon ? <img src={`/api/apps/${app.id}/icon`} alt="" className="w-10 h-10 rounded-lg" /> : <span className="text-primary font-mono">{app.name[0]}</span>}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="text-base font-semibold text-text-primary group-hover:text-primary transition-colors">{app.name}</h3>
            <span className="text-xs text-text-muted font-mono">v{app.version}</span>
          </div>
          <p className="text-sm text-text-secondary mt-1 overflow-hidden text-ellipsis line-clamp-2">{app.description}</p>
          <div className="flex gap-1.5 mt-2 flex-wrap">
            {app.categories.map(c => <Badge key={c} className="bg-bg-secondary text-text-secondary">{c}</Badge>)}
            {app.gpu_support && app.gpu_support.length > 0 && <Badge className="bg-primary/10 text-primary">GPU {app.gpu_required ? 'Required' : 'Optional'}</Badge>}
          </div>
        </div>
      </div>
    </a>
  )
}

export function BranchDialog({ sourceId, sourceName, onClose }: { sourceId: string; sourceName: string; onClose: () => void }) {
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const handleBranch = async () => {
    setLoading(true); setError('')
    try {
      await api.devBranchApp(sourceId)
      window.location.hash = `#/dev/${sourceId}`
      onClose()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Branch failed'
      if (msg.includes('already exists')) {
        setError(`Dev copy already exists.`)
      } else {
        setError(msg)
      }
    } finally { setLoading(false) }
  }

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-[200]">
      <div className="bg-bg-card border border-border rounded-xl p-8 w-full max-w-[460px]">
        <h2 className="text-lg font-bold text-text-primary mb-2 font-mono">Branch {sourceName}</h2>
        <p className="text-sm text-text-secondary mb-4">Create a development branch for <span className="font-mono text-text-primary">{sourceName}</span>? This will copy the app into Developer Mode for editing. Your changes will be pushed to branch <span className="font-mono text-primary">app/{sourceId}</span> on your GitHub fork when you submit.</p>
        {error && (
          <div className="text-xs text-status-stopped mt-2">
            {error}
            {error.includes('already exists') && (
              <a href={`#/dev/${sourceId}`} className="ml-2 text-primary hover:underline" onClick={onClose}>Open it</a>
            )}
          </div>
        )}
        <div className="flex gap-3 mt-5 justify-end">
          <button onClick={onClose} className="px-5 py-2.5 text-sm font-semibold border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-text-secondary transition-colors font-mono">Cancel</button>
          <button onClick={handleBranch} disabled={loading} className="px-5 py-2.5 text-sm font-semibold border-none rounded-lg cursor-pointer bg-yellow-400 text-bg-primary hover:shadow-[0_0_20px_rgba(255,200,0,0.3)] transition-all disabled:opacity-50 font-mono">
            {loading ? 'Branching...' : 'Branch to Dev'}
          </button>
        </div>
      </div>
    </div>
  )
}

export function AppDetailView({ id, requireAuth, devMode }: { id: string; requireAuth: (cb: () => void) => void; devMode: boolean }) {
  const [app, setApp] = useState<AppDetail | null>(null)
  const [readme, setReadme] = useState('')
  const [error, setError] = useState('')
  const [showInstall, setShowInstall] = useState(false)
  const [appStatus, setAppStatus] = useState<AppStatusResponse | null>(null)
  const [showBranch, setShowBranch] = useState(false)
  const [replaceExisting, setReplaceExisting] = useState(false)
  const [showTestConfirm, setShowTestConfirm] = useState(false)
  const [existingInstall, setExistingInstall] = useState<Install | null>(null)
  const [ghStatus, setGhStatus] = useState<GitHubStatus | null>(null)

  useEffect(() => {
    setApp(null); setError(''); setAppStatus(null); setExistingInstall(null)
    api.app(id).then(setApp).catch(e => setError(e.message))
    api.appReadme(id).then(setReadme)
    api.appStatus(id).then(s => {
      setAppStatus(s)
      // Fetch existing install details for pre-filling test install wizard
      if (s.installed && s.install_id) {
        api.installDetail(s.install_id).then(setExistingInstall).catch(() => {})
      }
    }).catch(() => {})
  }, [id])

  useEffect(() => {
    if (devMode) {
      api.devGitHubStatus().then(setGhStatus).catch(() => {})
    }
  }, [devMode])

  if (error) return <div><BackLink /><Center className="text-status-stopped">{error}</Center></div>
  if (!app) return <Center>Loading...</Center>

  const inputGroups = app.inputs && app.inputs.length > 0 ? groupInputs(app.inputs) : null

  // Dev app viewing an official install — allow "Test Install" to replace
  const isDevApp = app.source === 'developer'
  const hasOfficialInstall = appStatus?.installed && appStatus?.app_source !== 'developer'
  const canTestInstall = isDevApp && hasOfficialInstall

  return (
    <div>
      <BackLink />
      <div className="mt-4 flex items-start gap-5">
        <div className="w-16 h-16 rounded-xl bg-bg-secondary flex items-center justify-center text-3xl shrink-0 overflow-hidden">
          {app.icon_path ? <img src={`/api/apps/${app.id}/icon`} alt="" className="w-14 h-14 rounded-lg" /> : <span className="text-primary font-mono">{app.name[0]}</span>}
        </div>
        <div className="flex-1">
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-bold text-text-primary">{app.name}</h1>
            {app.featured && <Badge className="bg-status-featured/10 text-status-featured">featured</Badge>}
            {app.official && <Badge className="bg-primary/10 text-primary">official</Badge>}
            {isDevApp && <Badge className="bg-yellow-400/10 text-yellow-400">dev</Badge>}
          </div>
          <p className="text-sm text-text-secondary mt-1">{app.description}</p>
          <div className="flex gap-3 mt-2 text-sm text-text-muted items-center font-mono">
            <span>v{app.version}</span>
            {app.license && <span>{app.license}</span>}
            {app.homepage && <a href={app.homepage} target="_blank" rel="noreferrer" className="text-primary hover:underline">{'>'}homepage</a>}
          </div>
        </div>
        <div className="flex gap-2 items-center shrink-0">
          {canTestInstall ? (
            <button onClick={() => requireAuth(() => setShowTestConfirm(true))} className="px-6 py-2.5 bg-yellow-400 text-bg-primary font-semibold font-mono uppercase text-sm rounded-lg hover:shadow-[0_0_20px_rgba(250,204,21,0.3)] transition-all cursor-pointer border-none">Test Install</button>
          ) : appStatus?.installed ? (
            <a href={`#/install/${appStatus.install_id}`} className="px-6 py-2.5 bg-bg-secondary border border-primary text-primary font-semibold font-mono uppercase text-sm rounded-lg hover:bg-primary/10 transition-all no-underline">Installed</a>
          ) : appStatus?.job_active ? (
            <a href={`#/job/${appStatus.job_id}`} className="px-6 py-2.5 bg-bg-secondary border border-status-warning text-status-warning font-semibold font-mono uppercase text-sm rounded-lg hover:bg-status-warning/10 transition-all no-underline">Installing...</a>
          ) : (
            <button onClick={() => requireAuth(() => setShowInstall(true))} className="px-6 py-2.5 bg-primary text-bg-primary font-semibold font-mono uppercase text-sm rounded-lg hover:shadow-[0_0_20px_rgba(0,255,157,0.3)] transition-all cursor-pointer border-none">Install</button>
          )}
          {devMode && !isDevApp && ghStatus?.connected && ghStatus?.fork && <button onClick={() => requireAuth(() => setShowBranch(true))} className="px-4 py-2.5 bg-bg-secondary border border-yellow-400 text-yellow-400 font-semibold font-mono uppercase text-sm rounded-lg hover:bg-yellow-400/10 transition-all cursor-pointer">Branch</button>}
        </div>
      </div>

      {showBranch && <BranchDialog sourceId={app.id} sourceName={app.name} onClose={() => setShowBranch(false)} />}

      {showTestConfirm && (
        <TestInstallModal
          app={app}
          ctid={appStatus?.ctid}
          onConfirm={() => { setShowTestConfirm(false); setReplaceExisting(true); setShowInstall(true) }}
          onClose={() => setShowTestConfirm(false)}
        />
      )}

      {app.overview && (
        <div className="mt-5 bg-bg-card border border-border rounded-lg p-6">
          <h3 className="text-xs font-semibold text-text-muted mb-3 uppercase tracking-wider font-mono">Overview</h3>
          {app.overview.split('\n\n').map((p, i) => (
            <p key={i} className={`text-sm text-text-secondary leading-7 ${i > 0 ? 'mt-3' : ''}`}>{p}</p>
          ))}
        </div>
      )}

      {showInstall && <InstallWizard app={app} onClose={() => { setShowInstall(false); setReplaceExisting(false) }} replaceExisting={replaceExisting} existingInstall={replaceExisting ? existingInstall : undefined} />}

      <div className="grid grid-cols-[repeat(auto-fit,minmax(280px,1fr))] gap-4 mt-6">
        <InfoCard title="Default Resources">
          <InfoRow label="OS Template" value={app.lxc.ostemplate} />
          <InfoRow label="CPU Cores" value={String(app.lxc.defaults.cores)} />
          <InfoRow label="Memory" value={`${app.lxc.defaults.memory_mb} MB`} />
          <InfoRow label="Disk" value={`${app.lxc.defaults.disk_gb} GB`} />
          <InfoRow label="Unprivileged" value={app.lxc.defaults.unprivileged ? 'Yes' : 'No'} />
          {app.lxc.defaults.features && app.lxc.defaults.features.length > 0 && <InfoRow label="Features" value={app.lxc.defaults.features.join(', ')} />}
        </InfoCard>
        {app.volumes && app.volumes.length > 0 && (
          <InfoCard title="Mounts">
            {app.volumes.map(vol => (
              <div key={vol.name} className="py-1.5 border-b border-border last:border-b-0">
                <div className="flex justify-between items-center text-sm">
                  <div className="flex items-center gap-2">
                    <span className="text-text-primary">{vol.label || vol.name}</span>
                    <Badge className={vol.type === 'bind' ? 'bg-status-warning/10 text-status-warning' : 'bg-primary/10 text-primary'}>
                      {vol.type || 'volume'}
                    </Badge>
                    {!vol.required && <Badge className="bg-bg-secondary text-text-muted">optional</Badge>}
                  </div>
                  {vol.size_gb ? <span className="text-text-muted font-mono">{vol.size_gb} GB</span> : null}
                </div>
                <div className="text-xs text-text-muted font-mono mt-0.5">{vol.mount_path}</div>
                {vol.default_host_path && <div className="text-xs text-text-muted font-mono">default: {vol.default_host_path}</div>}
                {vol.description && <div className="text-xs text-text-secondary mt-0.5">{vol.description}</div>}
              </div>
            ))}
            <p className="text-xs text-text-muted mt-2 italic">Managed volumes survive uninstall; bind mounts reference existing host paths.</p>
          </InfoCard>
        )}
        {app.gpu.supported && app.gpu.supported.length > 0 && (
          <InfoCard title="GPU Support">
            <InfoRow label="Required" value={app.gpu.required ? 'Yes' : 'No'} />
            <InfoRow label="Supported" value={app.gpu.supported.join(', ')} />
            {app.gpu.profiles && <InfoRow label="Profiles" value={app.gpu.profiles.join(', ')} />}
            {app.gpu.notes && <p className="text-sm text-text-secondary mt-2">{app.gpu.notes}</p>}
          </InfoCard>
        )}
        {app.outputs && app.outputs.length > 0 && (
          <InfoCard title="Outputs">
            {app.outputs.map(out => {
              const display = out.value
                .replace(/\{\{IP\}\}/gi, '<container-ip>')
                .replace(/\{\{(\w+)\}\}/g, (_, k) => {
                  const inp = app.inputs?.find((i: AppInput) => i.key === k)
                  return inp ? `<${inp.label.toLowerCase()}>` : `<${k}>`
                })
              return <InfoRow key={out.key} label={out.label} value={display} />
            })}
          </InfoCard>
        )}
      </div>

      {inputGroups && (
        <div className="mt-6">
          <h3 className="text-xs font-semibold text-text-muted mb-4 uppercase tracking-wider font-mono">Configuration Reference</h3>
          {Object.entries(inputGroups).map(([group, inputs]) => (
            <div key={group} className="bg-bg-card border border-border rounded-lg p-5 mb-3">
              <h4 className="text-xs text-primary uppercase mb-3 tracking-wider font-mono">{group}</h4>
              {inputs.map(inp => (
                <div key={inp.key} className="py-2.5 border-b border-border last:border-b-0">
                  <div className="flex justify-between items-center">
                    <div className="flex items-center gap-2">
                      <span className="text-sm text-text-primary font-medium">{inp.label}</span>
                      <Badge className="bg-bg-secondary text-text-secondary">{inp.type}</Badge>
                      {inp.required && <Badge className="bg-status-stopped/10 text-status-stopped">required</Badge>}
                    </div>
                    {inp.default !== undefined && (
                      <span className="text-sm text-text-muted font-mono">default: <span className="text-text-secondary">{String(inp.default)}</span></span>
                    )}
                  </div>
                  {inp.description && <p className="text-sm text-text-secondary mt-1.5 leading-relaxed">{inp.description}</p>}
                  {inp.help && <p className="text-xs text-text-muted mt-1 italic">{inp.help}</p>}
                </div>
              ))}
            </div>
          ))}
        </div>
      )}

      {readme && (
        <div className="mt-6 bg-bg-card border border-border rounded-lg p-6">
          <h3 className="text-base font-semibold text-text-primary mb-3">README</h3>
          <div className="prose prose-invert prose-sm max-w-none
            prose-headings:text-text-primary prose-headings:font-semibold prose-headings:mt-4 prose-headings:mb-2
            prose-p:text-text-secondary prose-p:leading-relaxed
            prose-a:text-accent prose-a:no-underline hover:prose-a:underline
            prose-strong:text-text-primary
            prose-code:text-accent prose-code:bg-bg-secondary prose-code:px-1.5 prose-code:py-0.5 prose-code:rounded prose-code:text-xs prose-code:font-mono
            prose-pre:bg-bg-secondary prose-pre:border prose-pre:border-border prose-pre:rounded-lg
            prose-table:border-collapse
            prose-th:border prose-th:border-border prose-th:px-3 prose-th:py-2 prose-th:bg-bg-secondary prose-th:text-text-primary prose-th:text-left prose-th:text-sm
            prose-td:border prose-td:border-border prose-td:px-3 prose-td:py-2 prose-td:text-text-secondary prose-td:text-sm
            prose-li:text-text-secondary prose-li:marker:text-text-muted
            prose-hr:border-border
          ">
            <Markdown remarkPlugins={[remarkGfm]}>{readme}</Markdown>
          </div>
        </div>
      )}
    </div>
  )
}
