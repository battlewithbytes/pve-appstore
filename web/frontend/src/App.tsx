import { useState, useEffect, useCallback, useRef } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'
import { api } from './api'
import type { AppSummary, AppDetail, AppInput, HealthResponse, Job, LogEntry, InstallDetail, InstallListItem, ConfigDefaultsResponse, MountPoint, MountInfo, BrowseEntry, ExportResponse, ExportRecipe, InstallRequest, DevicePassthrough, AppStatusResponse, StackListItem, StackDetail, StackCreateRequest, StackValidateResponse, StackApp } from './types'

function useHash() {
  const [hash, setHash] = useState(window.location.hash)
  useEffect(() => {
    const handler = () => setHash(window.location.hash)
    window.addEventListener('hashchange', handler)
    return () => window.removeEventListener('hashchange', handler)
  }, [])
  return hash
}

function App() {
  const hash = useHash()
  const [health, setHealth] = useState<HealthResponse | null>(null)
  const [authed, setAuthed] = useState(false)
  const [authRequired, setAuthRequired] = useState(false)
  const [showLogin, setShowLogin] = useState(false)
  const [loginCallback, setLoginCallback] = useState<(() => void) | null>(null)

  useEffect(() => { api.health().then(setHealth).catch(() => {}) }, [])
  useEffect(() => {
    api.authCheck().then(d => {
      setAuthed(d.authenticated)
      setAuthRequired(d.auth_required)
    }).catch(() => {})
  }, [])

  const requireAuth = useCallback((onSuccess: () => void) => {
    if (authed || !authRequired) { onSuccess(); return }
    setLoginCallback(() => onSuccess)
    setShowLogin(true)
  }, [authed, authRequired])

  const handleLoginSuccess = useCallback(() => {
    setAuthed(true)
    setShowLogin(false)
    if (loginCallback) { loginCallback(); setLoginCallback(null) }
  }, [loginCallback])

  const handleLogout = useCallback(async () => {
    await api.logout().catch(() => {})
    setAuthed(false)
  }, [])

  const appMatch = hash.match(/^#\/app\/(.+)$/)
  const jobMatch = hash.match(/^#\/job\/(.+)$/)
  const installMatch = hash.match(/^#\/install\/(.+)$/)
  const stackMatch = hash.match(/^#\/stack\/(.+)$/)
  const isInstalls = hash === '#/installs'
  const isStacks = hash === '#/stacks'
  const isCreateStack = hash === '#/create-stack'
  const isJobs = hash === '#/jobs'
  const isConfig = hash === '#/config'

  let content
  if (jobMatch) content = <JobView id={jobMatch[1]} />
  else if (installMatch) content = <InstallDetailView id={installMatch[1]} requireAuth={requireAuth} />
  else if (stackMatch) content = <StackDetailView id={stackMatch[1]} requireAuth={requireAuth} />
  else if (appMatch) content = <AppDetailView id={appMatch[1]} requireAuth={requireAuth} />
  else if (isInstalls) content = <InstallsList requireAuth={requireAuth} />
  else if (isStacks) content = <StacksList requireAuth={requireAuth} />
  else if (isCreateStack) content = <StackCreateWizard requireAuth={requireAuth} />
  else if (isJobs) content = <JobsList />
  else if (isConfig) content = <ConfigView requireAuth={requireAuth} />
  else content = <AppList />

  return (
    <div className="min-h-screen flex flex-col bg-bg-primary">
      <Header health={health} authed={authed} authRequired={authRequired} onLogout={handleLogout} onLogin={() => setShowLogin(true)} />
      <main className="flex-1 max-w-[1200px] mx-auto px-4 py-6 w-full">
        {content}
      </main>
      <Footer />
      {showLogin && <LoginModal onSuccess={handleLoginSuccess} onClose={() => { setShowLogin(false); setLoginCallback(null) }} />}
    </div>
  )
}

function Header({ health, authed, authRequired, onLogout, onLogin }: { health: HealthResponse | null; authed: boolean; authRequired: boolean; onLogout: () => void; onLogin: () => void }) {
  return (
    <header className="bg-bg-primary border-b border-border px-6 py-3 flex items-center justify-between">
      <div className="flex items-center gap-6">
        <a href="#/" className="no-underline text-inherit flex items-center gap-3">
          <span className="text-primary text-2xl font-mono font-bold">&gt;_</span>
          <span className="text-lg font-bold text-text-primary font-mono tracking-tight">PVE App Store</span>
        </a>
        <nav className="flex gap-4">
          <a href="#/" className="text-text-secondary hover:text-primary no-underline text-sm font-mono uppercase tracking-wider transition-colors">Apps</a>
          <a href="#/installs" className="text-text-secondary hover:text-primary no-underline text-sm font-mono uppercase tracking-wider transition-colors">Installed</a>
          <a href="#/stacks" className="text-text-secondary hover:text-primary no-underline text-sm font-mono uppercase tracking-wider transition-colors">Stacks</a>
          <a href="#/jobs" className="text-text-secondary hover:text-primary no-underline text-sm font-mono uppercase tracking-wider transition-colors">Jobs</a>
          <a href="#/config" className="text-text-secondary hover:text-primary no-underline text-sm font-mono uppercase tracking-wider transition-colors">Config</a>
        </nav>
      </div>
      <div className="flex items-center gap-4 text-xs text-text-muted font-mono">
        {health && <>
          <span>node:{health.node}</span>
          <span>v{health.version}</span>
        </>}
        {authRequired && (authed ? (
          <button onClick={onLogout} className="bg-transparent border border-border rounded px-3 py-1 text-text-muted text-xs font-mono cursor-pointer hover:border-primary hover:text-primary transition-colors">logout</button>
        ) : (
          <button onClick={onLogin} className="bg-transparent border border-primary rounded px-3 py-1 text-primary text-xs font-mono cursor-pointer hover:bg-primary hover:text-bg-primary transition-colors">login</button>
        ))}
      </div>
    </header>
  )
}

function Footer() {
  return (
    <footer className="border-t border-border px-6 py-4 mt-8">
      <div className="max-w-[1200px] mx-auto flex flex-col sm:flex-row items-center justify-between gap-2 text-xs text-text-muted font-mono">
        <span>&copy; {new Date().getFullYear()} BattleWithBytes.io</span>
        <div className="flex items-center gap-4">
          <a href="https://github.com/battlewithbytes/pve-appstore-catalog" target="_blank" rel="noreferrer" className="text-text-muted hover:text-primary transition-colors">App Catalog</a>
          <a href="https://github.com/battlewithbytes/pve-appstore" target="_blank" rel="noreferrer" className="text-text-muted hover:text-primary transition-colors">GitHub</a>
        </div>
      </div>
    </footer>
  )
}

// --- App List ---

function AppList() {
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

function AppCard({ app }: { app: AppSummary }) {
  return (
    <a href={`#/app/${app.id}`} className="block bg-bg-card border border-border rounded-lg p-5 no-underline text-inherit transition-all duration-200 hover:border-border-hover hover:shadow-[0_0_20px_rgba(0,255,157,0.15)] hover:-translate-y-0.5 group">
      <div className="flex items-start gap-4">
        <div className="w-12 h-12 rounded-lg bg-bg-secondary flex items-center justify-center text-xl shrink-0 overflow-hidden">
          {app.has_icon ? <img src={`/api/apps/${app.id}/icon`} alt="" className="w-10 h-10 rounded-lg" /> : <span className="text-primary font-mono">{app.name[0]}</span>}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="text-base font-semibold text-text-primary group-hover:text-primary transition-colors">{app.name}</h3>
            <span className="text-xs text-text-muted font-mono">v{app.version}</span>
            {app.official && <Badge className="bg-primary/10 text-primary">official</Badge>}
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

// --- App Detail + Install ---

function groupInputs(inputs: AppInput[]): Record<string, AppInput[]> {
  const groups: Record<string, AppInput[]> = {}
  for (const inp of inputs) {
    const g = inp.group || 'General'
    if (!groups[g]) groups[g] = []
    groups[g].push(inp)
  }
  return groups
}

function AppDetailView({ id, requireAuth }: { id: string; requireAuth: (cb: () => void) => void }) {
  const [app, setApp] = useState<AppDetail | null>(null)
  const [readme, setReadme] = useState('')
  const [error, setError] = useState('')
  const [showInstall, setShowInstall] = useState(false)
  const [appStatus, setAppStatus] = useState<AppStatusResponse | null>(null)

  useEffect(() => {
    setApp(null); setError(''); setAppStatus(null)
    api.app(id).then(setApp).catch(e => setError(e.message))
    api.appReadme(id).then(setReadme)
    api.appStatus(id).then(setAppStatus).catch(() => {})
  }, [id])

  if (error) return <div><BackLink /><Center className="text-status-stopped">{error}</Center></div>
  if (!app) return <Center>Loading...</Center>

  const inputGroups = app.inputs && app.inputs.length > 0 ? groupInputs(app.inputs) : null

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
            {app.official && <Badge className="bg-primary/10 text-primary">official</Badge>}
          </div>
          <p className="text-sm text-text-secondary mt-1">{app.description}</p>
          <div className="flex gap-3 mt-2 text-sm text-text-muted items-center font-mono">
            <span>v{app.version}</span>
            {app.license && <span>{app.license}</span>}
            {app.homepage && <a href={app.homepage} target="_blank" rel="noreferrer" className="text-primary hover:underline">{'>'}homepage</a>}
          </div>
        </div>
        {appStatus?.installed ? (
          <a href={`#/install/${appStatus.install_id}`} className="px-6 py-2.5 bg-bg-secondary border border-primary text-primary font-semibold font-mono uppercase text-sm rounded-lg hover:bg-primary/10 transition-all no-underline">Installed</a>
        ) : appStatus?.job_active ? (
          <a href={`#/job/${appStatus.job_id}`} className="px-6 py-2.5 bg-bg-secondary border border-status-warning text-status-warning font-semibold font-mono uppercase text-sm rounded-lg hover:bg-status-warning/10 transition-all no-underline">Installing...</a>
        ) : (
          <button onClick={() => requireAuth(() => setShowInstall(true))} className="px-6 py-2.5 bg-primary text-bg-primary font-semibold font-mono uppercase text-sm rounded-lg hover:shadow-[0_0_20px_rgba(0,255,157,0.3)] transition-all cursor-pointer border-none">Install</button>
        )}
      </div>

      {app.overview && (
        <div className="mt-5 bg-bg-card border border-border rounded-lg p-6">
          <h3 className="text-xs font-semibold text-text-muted mb-3 uppercase tracking-wider font-mono">Overview</h3>
          {app.overview.split('\n\n').map((p, i) => (
            <p key={i} className={`text-sm text-text-secondary leading-7 ${i > 0 ? 'mt-3' : ''}`}>{p}</p>
          ))}
        </div>
      )}

      {showInstall && <InstallWizard app={app} onClose={() => setShowInstall(false)} />}

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
            {app.outputs.map(out => <InfoRow key={out.key} label={out.label} value={out.value} />)}
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
          <pre className="text-sm text-text-secondary whitespace-pre-wrap leading-relaxed font-mono">{readme}</pre>
        </div>
      )}
    </div>
  )
}

function InstallWizard({ app, onClose }: { app: AppDetail; onClose: () => void }) {
  const [inputs, setInputs] = useState<Record<string, string>>(() => {
    const d: Record<string, string> = {}
    app.inputs?.forEach(i => { if (i.default !== undefined) d[i.key] = String(i.default) })
    return d
  })
  const [cores, setCores] = useState(String(app.lxc.defaults.cores))
  const [memory, setMemory] = useState(String(app.lxc.defaults.memory_mb))
  const [disk, setDisk] = useState(String(app.lxc.defaults.disk_gb))
  const [storage, setStorage] = useState('')
  const [bridge, setBridge] = useState('')
  const [hostname, setHostname] = useState('')
  const [ipAddress, setIpAddress] = useState('')
  const [onboot, setOnboot] = useState(app.lxc.defaults.onboot ?? true)
  const [unprivileged, setUnprivileged] = useState(app.lxc.defaults.unprivileged ?? true)
  const [installing, setInstalling] = useState(false)
  const [error, setError] = useState('')
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [defaults, setDefaults] = useState<ConfigDefaultsResponse | null>(null)
  const [bindMounts, setBindMounts] = useState<Record<string, string>>(() => {
    const d: Record<string, string> = {}
    app.volumes?.filter(v => v.type === 'bind' && v.default_host_path).forEach(v => { d[v.name] = v.default_host_path! })
    return d
  })
  const [extraMounts, setExtraMounts] = useState<{ host_path: string; mount_path: string; read_only: boolean }[]>([])
  const [storageInputMounts, setStorageInputMounts] = useState<Record<string, string>>({})
  const [volumeStorages, setVolumeStorages] = useState<Record<string, string>>({})
  const [volumeBindOverrides, setVolumeBindOverrides] = useState<Record<string, string>>({})
  const [customVars, setCustomVars] = useState<{key: string; value: string}[]>([])
  const [devices, setDevices] = useState<DevicePassthrough[]>([])
  const [envVars] = useState<Record<string, string>>({})
  const [envVarList, setEnvVarList] = useState<{key: string; value: string}[]>([])
  const [browseTarget, setBrowseTarget] = useState<string | null>(null)
  const [browseInitPath, setBrowseInitPath] = useState('/')

  useEffect(() => {
    api.configDefaults().then(d => {
      setDefaults(d)
      setStorage(d.storages[0] || '')
      setBridge(d.bridges[0] || '')
    }).catch(() => {})
  }, [])

  const volumeVolumes = (app.volumes || []).filter(v => (v.type || 'volume') === 'volume')
  const bindVolumes = (app.volumes || []).filter(v => v.type === 'bind')
  const hasMounts = volumeVolumes.length > 0 || bindVolumes.length > 0

  const openBrowser = (target: string, currentPath?: string) => {
    setBrowseTarget(target)
    setBrowseInitPath(currentPath || '')
  }

  const handleBrowseSelect = (path: string) => {
    if (!browseTarget) return
    if (browseTarget.startsWith('extra-')) {
      const idx = parseInt(browseTarget.replace('extra-', ''))
      setExtraMounts(p => p.map((em, i) => i === idx ? { ...em, host_path: path } : em))
    } else if (browseTarget.startsWith('storage-')) {
      const key = browseTarget.replace('storage-', '')
      setStorageInputMounts(p => ({ ...p, [key]: path }))
    } else if (browseTarget.startsWith('volbind-')) {
      const name = browseTarget.replace('volbind-', '')
      setVolumeBindOverrides(p => ({ ...p, [name]: path }))
    } else {
      setBindMounts(p => ({ ...p, [browseTarget]: path }))
    }
    setBrowseTarget(null)
  }

  const handleInstall = async () => {
    setInstalling(true); setError('')
    try {
      // Merge custom variables into inputs
      const allInputs = { ...inputs }
      for (const cv of customVars) {
        if (cv.key.trim()) allInputs[cv.key.trim()] = cv.value
      }
      const req: Record<string, unknown> = {
        cores: parseInt(cores) || undefined,
        memory_mb: parseInt(memory) || undefined,
        disk_gb: parseInt(disk) || undefined,
        storage: storage || undefined,
        bridge: bridge || undefined,
        hostname: hostname || undefined,
        ip_address: ipAddress || undefined,
        onboot,
        unprivileged,
        inputs: allInputs,
      }
      // Merge bind mounts: manifest bind volumes + volume-to-bind overrides
      const allBindMounts = { ...bindMounts }
      for (const [name, hp] of Object.entries(volumeBindOverrides)) {
        if (hp) allBindMounts[name] = hp
      }
      if (Object.keys(allBindMounts).length > 0) req.bind_mounts = allBindMounts
      // Per-volume storage overrides (only for volumes NOT overridden to bind)
      const vs: Record<string, string> = {}
      for (const [name, st] of Object.entries(volumeStorages)) {
        if (st && st !== storage && !volumeBindOverrides[name]) vs[name] = st
      }
      if (Object.keys(vs).length > 0) req.volume_storages = vs
      // Merge storage input mounts into extra mounts
      const allExtras = [...extraMounts.filter(em => em.host_path && em.mount_path)]
      for (const [key, hostPath] of Object.entries(storageInputMounts)) {
        if (hostPath) {
          const inp = app.inputs?.find(i => i.key === key)
          allExtras.push({
            host_path: hostPath,
            mount_path: inputs[key] || String(inp?.default || ''),
            read_only: false,
          })
        }
      }
      if (allExtras.length > 0) req.extra_mounts = allExtras
      // Device passthrough
      const validDevices = devices.filter(d => d.path.trim())
      if (validDevices.length > 0) req.devices = validDevices
      // Environment variables (merge overrides + custom)
      const allEnv: Record<string, string> = { ...envVars }
      for (const ev of envVarList) {
        if (ev.key.trim()) allEnv[ev.key.trim()] = ev.value
      }
      if (Object.keys(allEnv).length > 0) req.env_vars = allEnv
      const job = await api.installApp(app.id, req as InstallRequest)
      window.location.hash = `#/job/${job.id}`
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Install failed')
      setInstalling(false)
    }
  }

  // Split inputs into storage-group (with path defaults) and the rest
  // Fix 2: Skip storage inputs whose default path is under a managed volume
  const managedMountPaths = volumeVolumes.map(v => v.mount_path)
  const storagePathInputs: AppInput[] = []
  const otherInputGroups: Record<string, AppInput[]> = {}
  if (app.inputs) {
    for (const inp of app.inputs) {
      if (inp.group === 'Storage' && typeof inp.default === 'string' && inp.default.startsWith('/')) {
        const isUnderManagedVolume = managedMountPaths.some(mp =>
          inp.default === mp || (inp.default as string).startsWith(mp + '/')
        )
        if (!isUnderManagedVolume) {
          storagePathInputs.push(inp)
        }
      } else {
        const g = inp.group || 'General'
        if (!otherInputGroups[g]) otherInputGroups[g] = []
        otherInputGroups[g].push(inp)
      }
    }
  }
  const hasOtherInputs = Object.keys(otherInputGroups).length > 0

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-[100]">
      <div className="bg-bg-card border border-border rounded-xl p-8 w-full max-w-[560px] max-h-[90vh] overflow-auto">
        <h2 className="text-xl font-bold text-text-primary mb-5 font-mono">Install {app.name}</h2>

        <SectionTitle>Resources</SectionTitle>
        <FormRow label="CPU Cores"><FormInput value={cores} onChange={setCores} type="number" /></FormRow>
        <FormRow label="Memory (MB)"><FormInput value={memory} onChange={setMemory} type="number" /></FormRow>
        <FormRow label="Disk (GB)" help="Root filesystem only — app data lives on separate volumes"><FormInput value={disk} onChange={setDisk} type="number" /></FormRow>

        <SectionTitle>Networking & Storage</SectionTitle>
        <FormRow label="Storage Pool" description="Proxmox storage where the container's virtual disk will be created." help={`Disk size: ${disk} GB`}>
          {defaults && defaults.storages.length > 1 ? (
            <select value={storage} onChange={e => setStorage(e.target.value)} className="w-full px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono">
              {defaults.storages.map(s => <option key={s} value={s}>{s}</option>)}
            </select>
          ) : (
            <span className="block px-3 py-2 bg-bg-primary border border-border rounded-md text-text-secondary text-sm font-mono">{storage}</span>
          )}
        </FormRow>
        <FormRow label="Network Bridge" description="Proxmox virtual bridge that connects the container to your network." help="Container gets its own IP via DHCP on this bridge">
          {defaults && defaults.bridges.length > 1 ? (
            <select value={bridge} onChange={e => setBridge(e.target.value)} className="w-full px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono">
              {defaults.bridges.map(b => <option key={b} value={b}>{b}</option>)}
            </select>
          ) : (
            <span className="block px-3 py-2 bg-bg-primary border border-border rounded-md text-text-secondary text-sm font-mono">{bridge}</span>
          )}
        </FormRow>

        {/* Unified Mounts section */}
        {(hasMounts || true) && (
          <>
            <SectionTitle>Mounts</SectionTitle>
            <div className="text-[11px] text-text-muted font-mono mb-2 border-l-2 border-primary/30 pl-2">
              Managed volumes survive container recreation. Bind mounts reference existing host paths.
            </div>

            {/* Managed volumes with toggle: PVE Volume vs Host Path */}
            {volumeVolumes.map(vol => {
              const isBind = volumeBindOverrides[vol.name] !== undefined
              return (
                <div key={vol.name} className="bg-bg-secondary rounded-lg p-3 mb-1.5">
                  <div className="flex justify-between items-center">
                    <div className="flex items-center gap-2">
                      <span className="text-sm text-text-primary">{vol.label || vol.name}</span>
                      <span className="text-xs text-text-muted font-mono">{vol.mount_path}</span>
                    </div>
                    {!isBind && <span className="text-sm text-text-muted font-mono">{vol.size_gb} GB</span>}
                  </div>
                  <div className="flex items-center gap-2 mt-1.5">
                    <button type="button" onClick={() => {
                      if (isBind) {
                        setVolumeBindOverrides(p => { const n = { ...p }; delete n[vol.name]; return n })
                      } else {
                        setVolumeBindOverrides(p => ({ ...p, [vol.name]: '' }))
                      }
                    }} className="text-[11px] text-primary bg-transparent border-none cursor-pointer p-0 font-mono hover:underline whitespace-nowrap">
                      {isBind ? 'use pve volume' : 'use host path'}
                    </button>
                    {/* LVM-thin note when switching to host path on non-browsable storage */}
                    {isBind && defaults?.storage_details && (() => {
                      const volStorage = volumeStorages[vol.name] || storage
                      const sd = defaults.storage_details.find(s => s.id === volStorage)
                      if (sd && !sd.browsable) return (
                        <span className="text-[10px] text-status-warning font-mono">
                          {volStorage} is block storage ({sd.type}) — no host filesystem to browse
                        </span>
                      )
                      return null
                    })()}
                    {isBind ? (
                      <>
                        <Badge className="bg-status-warning/10 text-status-warning">host path</Badge>
                      </>
                    ) : (
                      <>
                        {defaults && defaults.storages.length > 1 ? (
                          <select value={volumeStorages[vol.name] || storage}
                            onChange={e => setVolumeStorages(p => ({ ...p, [vol.name]: e.target.value }))}
                            className="px-2 py-1 text-xs bg-bg-primary border border-border rounded text-text-primary font-mono">
                            {defaults.storages.map(s => <option key={s} value={s}>{s}</option>)}
                          </select>
                        ) : (
                          <span className="text-xs text-text-secondary font-mono">{storage}</span>
                        )}
                        <Badge className="bg-primary/10 text-primary">pve volume</Badge>
                      </>
                    )}
                  </div>
                  {isBind && (
                    <div className="flex gap-2 mt-1.5">
                      <FormInput value={volumeBindOverrides[vol.name]} onChange={v => setVolumeBindOverrides(p => ({ ...p, [vol.name]: v }))} placeholder="/host/path" />
                      <button type="button" onClick={() => openBrowser(`volbind-${vol.name}`, volumeBindOverrides[vol.name] || '')}
                        className="px-3 py-2 text-xs font-mono border border-border rounded-md text-text-secondary bg-bg-primary hover:border-primary hover:text-primary transition-colors cursor-pointer whitespace-nowrap">
                        Browse
                      </button>
                    </div>
                  )}
                </div>
              )
            })}

            {/* Host path bind mounts from manifest */}
            {bindVolumes.map(vol => (
              <div key={vol.name} className="bg-bg-secondary rounded-lg p-3 mb-2">
                <div className="flex items-center justify-between mb-1.5">
                  <div className="flex items-center gap-2">
                    <span className="text-sm text-text-primary">{vol.label || vol.name}</span>
                    <Badge className="bg-status-warning/10 text-status-warning">bind</Badge>
                    {!vol.required && <Badge className="bg-bg-primary text-text-muted">optional</Badge>}
                  </div>
                </div>
                <div className="flex gap-2 mb-1">
                  <FormInput value={bindMounts[vol.name] || ''} onChange={v => setBindMounts(p => ({ ...p, [vol.name]: v }))} placeholder="/host/path" />
                  <button type="button" onClick={() => openBrowser(vol.name, bindMounts[vol.name] || vol.default_host_path || '')}
                    className="px-3 py-2 text-xs font-mono border border-border rounded-md text-text-secondary bg-bg-primary hover:border-primary hover:text-primary transition-colors cursor-pointer whitespace-nowrap">
                    Browse
                  </button>
                </div>
                <div className="text-[11px] text-text-muted font-mono">Container path: {vol.mount_path}</div>
              </div>
            ))}

            {/* Extra user-added mounts (stacked card layout) */}
            {extraMounts.map((em, i) => (
              <div key={i} className="bg-bg-secondary rounded-lg p-3 mb-2">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-text-muted font-mono">Extra Path #{i + 1}</span>
                  <button type="button" onClick={() => setExtraMounts(p => p.filter((_, j) => j !== i))}
                    className="text-status-stopped text-sm bg-transparent border-none cursor-pointer hover:text-status-stopped/80 leading-none px-1">&times;</button>
                </div>
                <div className="flex gap-2 mb-1.5">
                  <input type="text" value={em.host_path} onChange={e => setExtraMounts(p => p.map((x, j) => j === i ? { ...x, host_path: e.target.value } : x))} placeholder="/host/path"
                    className="flex-1 px-3 py-2 bg-bg-primary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono placeholder:text-text-muted" />
                  <button type="button" onClick={() => openBrowser(`extra-${i}`, em.host_path || '')}
                    className="px-3 py-2 text-xs font-mono border border-border rounded-md text-text-secondary bg-bg-primary hover:border-primary hover:text-primary transition-colors cursor-pointer">
                    Browse
                  </button>
                </div>
                <div className="flex gap-2 items-center">
                  <input type="text" value={em.mount_path} onChange={e => setExtraMounts(p => p.map((x, j) => j === i ? { ...x, mount_path: e.target.value } : x))} placeholder="/container/path"
                    className="flex-1 px-3 py-2 bg-bg-primary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono placeholder:text-text-muted" />
                  <label className="flex items-center gap-1.5 text-xs text-text-muted whitespace-nowrap cursor-pointer">
                    <input type="checkbox" checked={em.read_only} onChange={e => setExtraMounts(p => p.map((x, j) => j === i ? { ...x, read_only: e.target.checked } : x))} className="w-3.5 h-3.5 accent-primary" />
                    Read-only
                  </label>
                </div>
              </div>
            ))}

            <button type="button" onClick={() => setExtraMounts(p => [...p, { host_path: '', mount_path: '', read_only: false }])}
              className="text-primary text-xs font-mono bg-transparent border-none cursor-pointer hover:underline p-0">
              + Add Path
            </button>
          </>
        )}

        {/* App inputs (non-storage-path groups) */}
        {hasOtherInputs && (
          <div className="mt-4 mb-1 text-[11px] text-text-muted font-mono border-l-2 border-primary/30 pl-2">
            All app settings below apply inside the LXC container, not on the Proxmox host.
          </div>
        )}
        {Object.entries(otherInputGroups).map(([group, groupInps]) => (
          <div key={group}>
            <SectionTitle>{group}</SectionTitle>
            {groupInps.map(inp => (
              <FormRow key={inp.key} label={inp.label} help={inp.help} description={inp.description}>
                {inp.type === 'select' && inp.validation?.enum ? (
                  <select value={inputs[inp.key] || ''} onChange={e => setInputs(p => ({ ...p, [inp.key]: e.target.value }))} className="w-full px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono">
                    {inp.validation.enum.map(v => <option key={v} value={v}>{v}</option>)}
                  </select>
                ) : inp.type === 'boolean' ? (
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input type="checkbox" checked={inputs[inp.key] === 'true'} onChange={e => setInputs(p => ({ ...p, [inp.key]: e.target.checked ? 'true' : 'false' }))}
                      className="w-4 h-4 accent-primary" />
                    <span className="text-sm text-text-secondary">Enable</span>
                  </label>
                ) : (
                  <FormInput value={inputs[inp.key] || ''} onChange={v => setInputs(p => ({ ...p, [inp.key]: v }))}
                    type={inp.type === 'secret' ? 'password' : inp.type === 'number' ? 'number' : 'text'} />
                )}
              </FormRow>
            ))}
          </div>
        ))}

        {/* Storage-path inputs with optional "mount externally" toggle */}
        {storagePathInputs.length > 0 && (
          <>
            <SectionTitle>Storage</SectionTitle>
            {storagePathInputs.map(inp => (
              <div key={inp.key} className="mb-3">
                <label className="text-sm text-text-secondary block mb-1">{inp.label}</label>
                {inp.description && <div className="text-xs text-text-muted mb-1.5 leading-relaxed">{inp.description}</div>}
                <FormInput value={inputs[inp.key] || ''} onChange={v => setInputs(p => ({ ...p, [inp.key]: v }))} />
                {inp.help && <div className="text-[11px] text-text-muted mt-0.5 italic">{inp.help}</div>}
                <div className="mt-1.5">
                  <label className="flex items-center gap-2 text-xs text-text-muted cursor-pointer">
                    <input type="checkbox" checked={!!storageInputMounts[inp.key]}
                      onChange={e => {
                        if (e.target.checked) {
                          setStorageInputMounts(p => ({ ...p, [inp.key]: '' }))
                        } else {
                          setStorageInputMounts(p => { const n = { ...p }; delete n[inp.key]; return n })
                        }
                      }}
                      className="w-3.5 h-3.5 accent-primary" />
                    Mount from host path
                  </label>
                  {storageInputMounts[inp.key] !== undefined && (
                    <div className="flex gap-2 mt-1.5">
                      <FormInput value={storageInputMounts[inp.key]} onChange={v => setStorageInputMounts(p => ({ ...p, [inp.key]: v }))} placeholder="/host/path" />
                      <button type="button" onClick={() => openBrowser(`storage-${inp.key}`, storageInputMounts[inp.key] || '')}
                        className="px-3 py-2 text-xs font-mono border border-border rounded-md text-text-secondary bg-bg-secondary hover:border-primary hover:text-primary transition-colors cursor-pointer whitespace-nowrap">
                        Browse
                      </button>
                    </div>
                  )}
                </div>
              </div>
            ))}
          </>
        )}

        {/* Custom variables */}
        <SectionTitle>Custom Config</SectionTitle>
        {customVars.map((v, i) => (
          <div key={i} className="flex gap-2 mb-1.5 items-center">
            <input type="text" value={v.key} onChange={e => setCustomVars(p => p.map((x, j) => j === i ? { ...x, key: e.target.value } : x))} placeholder="KEY"
              className="w-1/3 px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono placeholder:text-text-muted uppercase" />
            <input type="text" value={v.value} onChange={e => setCustomVars(p => p.map((x, j) => j === i ? { ...x, value: e.target.value } : x))} placeholder="value"
              className="flex-1 px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono placeholder:text-text-muted" />
            <button type="button" onClick={() => setCustomVars(p => p.filter((_, j) => j !== i))}
              className="text-status-stopped text-sm bg-transparent border-none cursor-pointer hover:text-status-stopped/80 leading-none px-1">&times;</button>
          </div>
        ))}
        <button type="button" onClick={() => setCustomVars(p => [...p, { key: '', value: '' }])}
          className="text-primary text-xs font-mono bg-transparent border-none cursor-pointer hover:underline p-0">
          + Add Variable
        </button>
        <div className="text-[11px] text-text-muted mt-2">
          Ports: LXC containers get their own IP — all ports are directly accessible.
        </div>

        {/* Device Passthrough */}
        {(app.gpu.supported && app.gpu.supported.length > 0) && (
          <>
            <SectionTitle>Device Passthrough</SectionTitle>
            {app.gpu.profiles && app.gpu.profiles.length > 0 && (
              <div className="text-xs text-text-muted mb-2 font-mono">GPU profiles: {app.gpu.profiles.join(', ')}</div>
            )}
            <label className="flex items-center gap-2 text-xs text-text-muted cursor-pointer mb-2">
              <input type="checkbox" checked={devices.length > 0}
                onChange={e => {
                  if (e.target.checked) {
                    // Auto-populate from GPU profiles
                    const profileDevs: DevicePassthrough[] = []
                    for (const profile of (app.gpu.profiles || [])) {
                      if (profile === 'dri-render') profileDevs.push({ path: '/dev/dri/renderD128', gid: 44, mode: '0666' })
                      else if (profile === 'nvidia-basic') {
                        profileDevs.push({ path: '/dev/nvidia0' }, { path: '/dev/nvidiactl' }, { path: '/dev/nvidia-uvm' })
                      }
                    }
                    setDevices(profileDevs.length > 0 ? profileDevs : [{ path: '' }])
                  } else setDevices([])
                }}
                className="w-3.5 h-3.5 accent-primary" />
              Enable GPU/device passthrough
            </label>
            {devices.map((dev, i) => (
              <div key={i} className="flex gap-2 mb-1.5 items-center">
                <input type="text" value={dev.path} onChange={e => setDevices(p => p.map((x, j) => j === i ? { ...x, path: e.target.value } : x))} placeholder="/dev/dri/renderD128"
                  className="flex-1 px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono placeholder:text-text-muted" />
                <button type="button" onClick={() => setDevices(p => p.filter((_, j) => j !== i))}
                  className="text-status-stopped text-sm bg-transparent border-none cursor-pointer hover:text-status-stopped/80 leading-none px-1">&times;</button>
              </div>
            ))}
            {devices.length > 0 && (
              <button type="button" onClick={() => setDevices(p => [...p, { path: '' }])}
                className="text-primary text-xs font-mono bg-transparent border-none cursor-pointer hover:underline p-0">
                + Add Device
              </button>
            )}
          </>
        )}

        {/* Environment Variables */}
        {(app.provisioning as { env?: Record<string, string> }).env && Object.keys((app.provisioning as { env?: Record<string, string> }).env || {}).length > 0 && (
          <>
            <SectionTitle>Environment Variables</SectionTitle>
            <div className="text-[11px] text-text-muted font-mono mb-2 border-l-2 border-primary/30 pl-2">
              Manifest defaults (passed to provision script):
            </div>
            {Object.entries((app.provisioning as { env?: Record<string, string> }).env || {}).map(([k, v]) => (
              <div key={k} className="flex gap-2 mb-1 text-xs font-mono">
                <span className="text-text-muted w-1/3 truncate">{k}</span>
                <span className="text-text-secondary flex-1 truncate">{envVars[k] ?? v}</span>
              </div>
            ))}
          </>
        )}
        {envVarList.length > 0 && (
          <>
            {!((app.provisioning as { env?: Record<string, string> }).env && Object.keys((app.provisioning as { env?: Record<string, string> }).env || {}).length > 0) && <SectionTitle>Environment Variables</SectionTitle>}
            {envVarList.map((ev, i) => (
              <div key={i} className="flex gap-2 mb-1.5 items-center">
                <input type="text" value={ev.key} onChange={e => setEnvVarList(p => p.map((x, j) => j === i ? { ...x, key: e.target.value } : x))} placeholder="ENV_KEY"
                  className="w-1/3 px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono placeholder:text-text-muted uppercase" />
                <input type="text" value={ev.value} onChange={e => setEnvVarList(p => p.map((x, j) => j === i ? { ...x, value: e.target.value } : x))} placeholder="value"
                  className="flex-1 px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono placeholder:text-text-muted" />
                <button type="button" onClick={() => setEnvVarList(p => p.filter((_, j) => j !== i))}
                  className="text-status-stopped text-sm bg-transparent border-none cursor-pointer hover:text-status-stopped/80 leading-none px-1">&times;</button>
              </div>
            ))}
          </>
        )}
        <button type="button" onClick={() => setEnvVarList(p => [...p, { key: '', value: '' }])}
          className="text-primary text-xs font-mono bg-transparent border-none cursor-pointer hover:underline p-0 mt-1">
          + Add Env Var
        </button>

        {/* Advanced settings */}
        <div className="mt-5">
          <button onClick={() => setShowAdvanced(!showAdvanced)} className="bg-transparent border-none text-primary text-sm cursor-pointer p-0 font-mono hover:underline">
            {showAdvanced ? '- hide' : '+ show'} advanced
          </button>
          {showAdvanced && (
            <div className="mt-3 space-y-3">
              <FormRow label="Hostname" description="Container hostname on the network" help={`Defaults to: ${app.id}`}>
                <FormInput value={hostname} onChange={setHostname} placeholder={app.id} />
              </FormRow>
              <FormRow label="Static IP" description="Fixed IP address for this container" help="Leave blank for DHCP">
                <FormInput value={ipAddress} onChange={setIpAddress} placeholder="e.g. 192.168.1.100" />
              </FormRow>
              <FormRow label="Start on Boot">
                <label className="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" checked={onboot} onChange={e => setOnboot(e.target.checked)} className="w-4 h-4 accent-primary" />
                  <span className="text-sm text-text-secondary">Auto-start when Proxmox host boots</span>
                </label>
              </FormRow>
              <FormRow label="Unprivileged">
                <label className="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" checked={unprivileged} onChange={e => setUnprivileged(e.target.checked)} className="w-4 h-4 accent-primary" />
                  <span className="text-sm text-text-secondary">Run as unprivileged container (recommended)</span>
                </label>
              </FormRow>
              <div className="text-[11px] text-text-muted font-mono mt-2">
                OS: {app.lxc.ostemplate} | Features: {(app.lxc.defaults.features || []).join(', ') || 'none'}
              </div>
            </div>
          )}
        </div>

        {error && <div className="text-status-stopped text-sm mt-3 font-mono">{error}</div>}

        <div className="flex gap-3 mt-6 justify-end">
          <button onClick={onClose} className="px-5 py-2.5 text-sm font-semibold border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-text-secondary transition-colors font-mono">Cancel</button>
          <button onClick={handleInstall} disabled={installing} className="px-5 py-2.5 text-sm font-semibold border-none rounded-lg cursor-pointer bg-primary text-bg-primary hover:shadow-[0_0_20px_rgba(0,255,157,0.3)] transition-all disabled:opacity-50 font-mono">
            {installing ? 'Installing...' : 'Install'}
          </button>
        </div>
      </div>
      {browseTarget !== null && (
        <DirectoryBrowser initialPath={browseInitPath} onSelect={handleBrowseSelect} onClose={() => setBrowseTarget(null)} />
      )}
    </div>
  )
}

// --- Job View ---

function JobView({ id }: { id: string }) {
  const [job, setJob] = useState<Job | null>(null)
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [cancelling, setCancelling] = useState(false)
  const [cancelError, setCancelError] = useState('')
  const lastLogId = useRef(0)
  const logEndRef = useRef<HTMLDivElement>(null)

  const refreshJob = useCallback(async () => {
    try {
      const [j, l] = await Promise.all([api.job(id), api.jobLogs(id, lastLogId.current)])
      setJob(j)
      if (l.logs && l.logs.length > 0) {
        setLogs(prev => [...prev, ...l.logs])
        lastLogId.current = l.last_id
      }
    } catch { /* ignore */ }
  }, [id])

  useEffect(() => {
    api.job(id).then(setJob).catch(() => {})
    api.jobLogs(id).then(d => { setLogs(d.logs || []); lastLogId.current = d.last_id })
  }, [id])

  useEffect(() => {
    if (!job || job.state === 'completed' || job.state === 'failed' || job.state === 'cancelled') return
    const interval = setInterval(refreshJob, 1500)
    return () => clearInterval(interval)
  }, [id, job?.state, refreshJob])

  useEffect(() => { logEndRef.current?.scrollIntoView({ behavior: 'smooth' }) }, [logs])

  const handleCancel = async () => {
    setCancelling(true)
    setCancelError('')
    try {
      await api.cancelJob(id)
      // Immediately refresh to pick up state change
      setTimeout(refreshJob, 500)
      setTimeout(refreshJob, 2000)
      setTimeout(refreshJob, 5000)
    } catch (e) {
      setCancelError(e instanceof Error ? e.message : 'Cancel failed')
      setCancelling(false)
    }
  }

  if (!job) return <Center>Loading...</Center>

  const done = job.state === 'completed' || job.state === 'failed' || job.state === 'cancelled'

  return (
    <div>
      <BackLink />
      <div className="mt-4 flex items-center gap-4">
        <h2 className="text-xl font-bold text-text-primary">{job.type === 'install' ? 'Installing' : job.type === 'uninstall' ? 'Uninstalling' : 'Reinstalling'} {job.app_name}</h2>
        <StateBadge state={job.state} />
        {!done && (
          <button onClick={handleCancel} disabled={cancelling} className="ml-auto px-4 py-1.5 text-sm font-mono rounded-lg border border-status-stopped/50 text-status-stopped hover:bg-status-stopped/10 transition-colors disabled:opacity-50">
            {cancelling ? 'Cancelling...' : 'Cancel'}
          </button>
        )}
      </div>

      {cancelError && <div className="mt-3 p-3 bg-status-stopped/10 border border-status-stopped/30 rounded-lg text-status-stopped text-sm font-mono">{cancelError}</div>}

      <div className="grid grid-cols-[repeat(auto-fit,minmax(200px,1fr))] gap-3 mt-4">
        <InfoCard title="Job Info">
          <InfoRow label="Job ID" value={job.id} />
          <InfoRow label="CTID" value={job.ctid ? String(job.ctid) : 'pending'} />
          <InfoRow label="Node" value={job.node} />
          <InfoRow label="Pool" value={job.pool} />
          {job.cores > 0 && <InfoRow label="Resources" value={`${job.cores}c / ${job.memory_mb}MB / ${job.disk_gb}GB`} />}
        </InfoCard>
        {done && job.outputs && Object.keys(job.outputs).length > 0 && (
          <InfoCard title="Outputs">
            {Object.entries(job.outputs).map(([k, v]) => <InfoRow key={k} label={k} value={v} />)}
          </InfoCard>
        )}
      </div>

      {job.error && <div className="mt-4 p-4 bg-status-stopped/10 border border-status-stopped/30 rounded-lg text-status-stopped text-sm font-mono">{job.error}</div>}

      <div className="mt-5 bg-bg-card border border-border rounded-lg p-4 max-h-[400px] overflow-auto">
        <h4 className="text-xs text-text-muted mb-2 uppercase font-mono tracking-wider">Logs</h4>
        {logs.length === 0 ? <div className="text-text-muted text-sm font-mono">Waiting for logs...</div> : logs.map((l, i) => (
          <div key={i} className={`text-xs font-mono py-0.5 ${l.level === 'error' ? 'text-status-stopped' : l.level === 'warn' ? 'text-status-warning' : 'text-text-secondary'}`}>
            <span className="text-text-muted">{new Date(l.timestamp).toLocaleTimeString()} </span>
            {l.message}
          </div>
        ))}
        <div ref={logEndRef} />
        {!done && !cancelling && <div className="text-status-warning text-xs mt-2 font-mono flex items-center gap-2"><span className="inline-block w-2 h-2 rounded-full bg-status-warning animate-pulse-glow" /> Running...</div>}
        {!done && cancelling && <div className="text-status-stopped text-xs mt-2 font-mono flex items-center gap-2"><span className="inline-block w-2 h-2 rounded-full bg-status-stopped animate-pulse-glow" /> Cancelling...</div>}
        {job.state === 'cancelled' && <div className="text-text-muted text-xs mt-2 font-mono flex items-center gap-2"><span className="inline-block w-2 h-2 rounded-full bg-text-muted" /> Cancelled</div>}
      </div>
    </div>
  )
}

// --- Install Detail View ---

function InstallDetailView({ id, requireAuth }: { id: string; requireAuth: (cb: () => void) => void }) {
  const [detail, setDetail] = useState<InstallDetail | null>(null)
  const [appInfo, setAppInfo] = useState<AppDetail | null>(null)
  const [error, setError] = useState('')
  const [uninstalling, setUninstalling] = useState(false)
  const [reinstalling, setReinstalling] = useState(false)
  const [showTerminal, setShowTerminal] = useState(false)
  const [showUninstallDialog, setShowUninstallDialog] = useState(false)
  const [showUpdateDialog, setShowUpdateDialog] = useState(false)
  const [updating, setUpdating] = useState(false)

  const fetchDetail = useCallback(() => {
    api.installDetail(id).then(d => {
      setDetail(d)
      if (!appInfo) api.app(d.app_id).then(setAppInfo).catch(() => {})
    }).catch(e => setError(e.message))
  }, [id, appInfo])

  useEffect(() => { fetchDetail() }, [fetchDetail])

  useEffect(() => {
    if (detail?.status === 'uninstalled') return // no polling for uninstalled
    const interval = setInterval(fetchDetail, 5000)
    return () => clearInterval(interval)
  }, [fetchDetail, detail?.status])

  const handleUninstall = (keepVolumes: boolean) => {
    requireAuth(async () => {
      if (!detail) return
      setUninstalling(true)
      setShowUninstallDialog(false)
      try {
        const j = await api.uninstall(detail.id, keepVolumes)
        window.location.hash = `#/job/${j.id}`
      } catch (e: unknown) {
        alert(e instanceof Error ? e.message : 'Uninstall failed')
        setUninstalling(false)
      }
    })
  }

  const handleReinstall = () => {
    requireAuth(async () => {
      if (!detail) return
      setReinstalling(true)
      try {
        const j = await api.reinstall(detail.id)
        window.location.hash = `#/job/${j.id}`
      } catch (e: unknown) {
        alert(e instanceof Error ? e.message : 'Reinstall failed')
        setReinstalling(false)
      }
    })
  }

  const handleUpdate = () => {
    requireAuth(async () => {
      if (!detail) return
      setUpdating(true)
      setShowUpdateDialog(false)
      try {
        const j = await api.update(detail.id)
        window.location.hash = `#/job/${j.id}`
      } catch (e: unknown) {
        alert(e instanceof Error ? e.message : 'Update failed')
        setUpdating(false)
      }
    })
  }

  if (error) return <div><BackLink href="#/installs" label="Back to installed" /><Center className="text-status-stopped">{error}</Center></div>
  if (!detail) return <Center>Loading...</Center>

  const isUninstalled = detail.status === 'uninstalled'
  const isRunning = !isUninstalled && (detail.live?.status === 'running' || detail.status === 'running')
  const live = detail.live
  const hasVolumes = detail.mount_points && detail.mount_points.length > 0

  return (
    <div>
      <BackLink href="#/installs" label="Back to installed" />

      {/* Header */}
      <div className="mt-4 flex items-start justify-between">
        <div className="flex items-center gap-4">
          <div className="w-14 h-14 rounded-xl bg-bg-secondary flex items-center justify-center overflow-hidden">
            <img src={`/api/apps/${detail.app_id}/icon`} alt="" className="w-12 h-12 rounded-lg" onError={e => { (e.target as HTMLImageElement).style.display = 'none' }} />
          </div>
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-2xl font-bold text-text-primary">{detail.app_name}</h1>
              {detail.app_version && <span className="text-sm text-text-muted font-mono">v{detail.app_version}</span>}
              {isUninstalled ? (
                <Badge className="bg-status-warning/10 text-status-warning">uninstalled</Badge>
              ) : (
                <>
                  <StatusDot running={isRunning} />
                  <span className={`text-sm font-mono ${isRunning ? 'text-status-running' : 'text-status-stopped'}`}>{detail.live?.status || detail.status}</span>
                </>
              )}
            </div>
            <div className="flex items-center gap-4 mt-1 text-sm text-text-muted font-mono">
              {detail.ctid > 0 && <span>CT {detail.ctid}</span>}
              {detail.ip && <span>IP: <span className="text-primary">{detail.ip}</span></span>}
              {live && live.uptime > 0 && <span>uptime: {formatUptime(live.uptime)}</span>}
            </div>
            <div className="flex items-center gap-3 mt-1.5 text-xs font-mono">
              <a href={`#/app/${detail.app_id}`} className="text-primary hover:underline">App Store Page</a>
              {appInfo?.homepage && <><span className="text-text-muted">|</span><a href={appInfo.homepage} target="_blank" rel="noreferrer" className="text-primary hover:underline">Project Homepage</a></>}
            </div>
          </div>
        </div>
        <div className="flex gap-2">
          {detail.update_available && !isUninstalled && (
            <button onClick={() => setShowUpdateDialog(true)} disabled={updating} className="px-4 py-2 text-sm font-mono border-none rounded-lg cursor-pointer text-bg-primary bg-primary hover:shadow-[0_0_20px_rgba(0,255,157,0.3)] transition-all disabled:opacity-50 font-semibold">
              {updating ? 'Updating...' : 'Update'}
            </button>
          )}
          {isUninstalled && hasVolumes && (
            <button onClick={handleReinstall} disabled={reinstalling} className="px-4 py-2 text-sm font-mono border-none rounded-lg cursor-pointer text-bg-primary bg-primary hover:shadow-[0_0_20px_rgba(0,255,157,0.3)] transition-all disabled:opacity-50 font-semibold">
              {reinstalling ? 'Reinstalling...' : 'Reinstall'}
            </button>
          )}
          {isRunning && (
            <button onClick={() => setShowTerminal(true)} className="px-4 py-2 text-sm font-mono border border-border rounded-lg cursor-pointer text-text-primary bg-transparent hover:border-primary hover:text-primary transition-colors">
              &gt;_ Shell
            </button>
          )}
          {!isUninstalled && (
            <button onClick={() => hasVolumes ? setShowUninstallDialog(true) : handleUninstall(false)} disabled={uninstalling} className="px-4 py-2 text-sm font-mono border border-status-stopped/30 rounded-lg cursor-pointer text-status-stopped bg-status-stopped/10 hover:bg-status-stopped/20 transition-colors disabled:opacity-50">
              {uninstalling ? 'Removing...' : 'Uninstall'}
            </button>
          )}
        </div>
      </div>

      {/* Uninstall dialog with keep-volumes toggle */}
      {showUninstallDialog && detail && (
        <UninstallDialog
          appName={detail.app_name}
          ctid={detail.ctid}
          mountPoints={detail.mount_points || []}
          onConfirm={handleUninstall}
          onCancel={() => setShowUninstallDialog(false)}
        />
      )}

      {/* Update confirmation dialog */}
      {showUpdateDialog && detail && (
        <UpdateDialog
          appName={detail.app_name}
          ctid={detail.ctid}
          currentVersion={detail.app_version}
          newVersion={detail.catalog_version || ''}
          isRunning={isRunning}
          onConfirm={handleUpdate}
          onCancel={() => setShowUpdateDialog(false)}
        />
      )}

      {/* Update available banner */}
      {detail.update_available && (
        <div className="mt-4 p-3 bg-status-warning/10 border border-status-warning/30 rounded-lg flex items-center justify-between">
          <span className="text-status-warning text-sm font-mono">Update available: v{detail.app_version} &rarr; v{detail.catalog_version}</span>
          {!isUninstalled && (
            <button onClick={() => setShowUpdateDialog(true)} disabled={updating} className="px-4 py-2 text-sm font-mono border-none rounded-lg cursor-pointer text-bg-primary bg-primary hover:shadow-[0_0_20px_rgba(0,255,157,0.3)] transition-all disabled:opacity-50 font-semibold">
              {updating ? 'Updating...' : 'Update'}
            </button>
          )}
        </div>
      )}

      {/* Uninstalled with volumes banner */}
      {isUninstalled && hasVolumes && (
        <div className="mt-4 p-3 bg-primary/10 border border-primary/30 rounded-lg">
          <span className="text-primary text-sm font-mono">Data preserved: {detail.mount_points!.length} volume(s) available for reinstall</span>
        </div>
      )}

      {/* Resource cards */}
      {live && isRunning && (
        <div className="grid grid-cols-[repeat(auto-fit,minmax(200px,1fr))] gap-3 mt-4">
          <ResourceCard label="CPU" value={`${(live.cpu * 100).toFixed(1)}%`} sub={`${live.cpus} core${live.cpus > 1 ? 's' : ''}`} pct={live.cpu * 100} />
          <ResourceCard label="Memory" value={formatBytes(live.mem)} sub={formatBytes(live.maxmem)} pct={live.maxmem > 0 ? (live.mem / live.maxmem) * 100 : 0} />
          <ResourceCard label="Disk" value={formatBytes(live.disk)} sub={formatBytes(live.maxdisk)} pct={live.maxdisk > 0 ? (live.disk / live.maxdisk) * 100 : 0} />
          <ResourceCard label="Network" value={`${formatBytesShort(live.netin)} in`} sub={`${formatBytesShort(live.netout)} out`} />
        </div>
      )}

      {/* Mounts */}
      {hasVolumes && (
        <div className="mt-4 bg-bg-card border border-border rounded-lg p-5">
          <h3 className="text-xs font-semibold text-text-muted mb-3 uppercase tracking-wider font-mono">Mounts</h3>
          <div className="grid grid-cols-[repeat(auto-fit,minmax(200px,1fr))] gap-3">
            {detail.mount_points!.map(mp => (
              <div key={mp.index} className="bg-bg-secondary rounded-lg p-3">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium text-text-primary">{mp.name}</span>
                  <Badge className={mp.type === 'bind' ? 'bg-status-warning/10 text-status-warning' : 'bg-primary/10 text-primary'}>
                    {mp.type || 'volume'}
                  </Badge>
                  {mp.read_only && <Badge className="bg-bg-primary text-text-muted">ro</Badge>}
                </div>
                <div className="text-xs text-text-muted font-mono mt-1">{mp.mount_path}</div>
                {mp.type === 'bind' && mp.host_path && (
                  <div className="text-xs text-text-secondary font-mono">host: {mp.host_path}</div>
                )}
                {(mp.type === 'volume' || !mp.type) && (
                  <>
                    {mp.size_gb ? <div className="text-xs text-text-muted font-mono">{mp.size_gb} GB</div> : null}
                    {mp.volume_id && <div className="text-xs text-primary font-mono mt-1 truncate" title={mp.volume_id}>{mp.volume_id}</div>}
                  </>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Outputs / URLs */}
      {detail.outputs && Object.keys(detail.outputs).length > 0 && (
        <div className="mt-4 bg-bg-card border border-border rounded-lg p-5">
          <h3 className="text-xs font-semibold text-text-muted mb-3 uppercase tracking-wider font-mono">Service URLs & Outputs</h3>
          {Object.entries(detail.outputs).map(([k, v]) => {
            const resolved = detail.ip ? v.replace(/\{\{ip\}\}/g, detail.ip) : v
            return <InfoRow key={k} label={k} value={resolved} />
          })}
        </div>
      )}

      {/* Config info */}
      <div className="grid grid-cols-[repeat(auto-fit,minmax(250px,1fr))] gap-3 mt-4">
        <InfoCard title="Container Config">
          <InfoRow label="CTID" value={detail.ctid > 0 ? String(detail.ctid) : '-'} />
          <InfoRow label="Node" value={detail.node} />
          <InfoRow label="IP Address" value={detail.ip || (detail.ip_address ? `${detail.ip_address} (static)` : 'DHCP')} />
          <InfoRow label="Pool" value={detail.pool || '-'} />
          <InfoRow label="Storage" value={detail.storage || '-'} />
          <InfoRow label="Bridge" value={detail.bridge || '-'} />
        </InfoCard>
        <InfoCard title="Resources (configured)">
          <InfoRow label="CPU Cores" value={String(detail.cores)} />
          <InfoRow label="Memory" value={`${detail.memory_mb} MB`} />
          <InfoRow label="Disk" value={`${detail.disk_gb} GB`} />
        </InfoCard>
      </div>

      {showTerminal && (
        <TerminalModal installId={detail.id} onClose={() => setShowTerminal(false)} />
      )}
    </div>
  )
}

// --- Uninstall Dialog ---

function UninstallDialog({ appName, ctid, mountPoints, onConfirm, onCancel }: {
  appName: string; ctid: number; mountPoints: MountPoint[];
  onConfirm: (keepVolumes: boolean) => void; onCancel: () => void;
}) {
  const [keepVolumes, setKeepVolumes] = useState(true)

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-[100]">
      <div className="bg-bg-card border border-border rounded-xl p-8 w-full max-w-[480px]">
        <h2 className="text-lg font-bold text-text-primary mb-2 font-mono">Uninstall {appName}</h2>
        <p className="text-sm text-text-secondary mb-4">This will destroy container CT {ctid}.</p>

        {mountPoints.length > 0 && (
          <div className="mb-4">
            <label className="flex items-center gap-3 cursor-pointer p-3 bg-bg-secondary rounded-lg">
              <input type="checkbox" checked={keepVolumes} onChange={e => setKeepVolumes(e.target.checked)} className="w-4 h-4 accent-primary" />
              <div>
                <span className="text-sm text-text-primary font-medium">Keep data volumes</span>
                <p className="text-xs text-text-muted mt-0.5">Preserve {mountPoints.length} volume(s) for future reinstall</p>
              </div>
            </label>

            <div className="mt-3 space-y-1">
              {mountPoints.map(mp => (
                <div key={mp.index} className={`flex justify-between text-xs font-mono px-3 py-1.5 rounded ${keepVolumes ? 'bg-primary/10 text-primary' : 'bg-status-stopped/10 text-status-stopped'}`}>
                  <span>{mp.name} ({mp.mount_path})</span>
                  <span>{keepVolumes ? 'preserved' : 'destroyed'}</span>
                </div>
              ))}
            </div>

            {!keepVolumes && (
              <div className="mt-3 p-2.5 bg-status-stopped/10 border border-status-stopped/30 rounded text-status-stopped text-xs font-mono">
                Warning: This will permanently delete all data in these volumes.
              </div>
            )}
          </div>
        )}

        <div className="flex gap-3 justify-end">
          <button onClick={onCancel} className="px-5 py-2.5 text-sm font-semibold border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-text-secondary transition-colors font-mono">Cancel</button>
          <button onClick={() => onConfirm(keepVolumes)} className="px-5 py-2.5 text-sm font-semibold border-none rounded-lg cursor-pointer bg-status-stopped text-white hover:opacity-90 transition-all font-mono">
            Uninstall
          </button>
        </div>
      </div>
    </div>
  )
}

function UpdateDialog({ appName, ctid, currentVersion, newVersion, isRunning, onConfirm, onCancel }: {
  appName: string; ctid: number; currentVersion: string; newVersion: string; isRunning: boolean;
  onConfirm: () => void; onCancel: () => void;
}) {
  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-[100]">
      <div className="bg-bg-card border border-border rounded-xl p-8 w-full max-w-[480px]">
        <h2 className="text-lg font-bold text-text-primary mb-2 font-mono">Update {appName}</h2>
        <p className="text-sm text-text-secondary mb-4">
          Update from <span className="font-mono text-text-primary">v{currentVersion}</span> to <span className="font-mono text-primary">v{newVersion}</span>
        </p>

        {isRunning && (
          <div className="mb-4 p-2.5 bg-status-warning/10 border border-status-warning/30 rounded text-status-warning text-xs font-mono">
            Warning: Container CT {ctid} is currently running. It will be stopped and recreated during the update.
          </div>
        )}

        <p className="text-xs text-text-muted mb-4 font-mono">
          This will destroy the current container and create a new one with the latest version. Data volumes (if any) will be preserved and reattached.
        </p>

        <div className="flex gap-3 justify-end">
          <button onClick={onCancel} className="px-5 py-2.5 text-sm font-semibold border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-text-secondary transition-colors font-mono">Cancel</button>
          <button onClick={onConfirm} className="px-5 py-2.5 text-sm font-semibold border-none rounded-lg cursor-pointer bg-primary text-bg-primary hover:shadow-[0_0_20px_rgba(0,255,157,0.3)] transition-all font-mono">
            Update
          </button>
        </div>
      </div>
    </div>
  )
}

// --- Installs List ---

function InstallsList({ requireAuth }: { requireAuth: (cb: () => void) => void }) {
  const [installs, setInstalls] = useState<InstallListItem[]>([])
  const [stacks, setStacks] = useState<StackListItem[]>([])
  const [loading, setLoading] = useState(true)
  const [contextMenu, setContextMenu] = useState<{ install: InstallListItem; x: number; y: number } | null>(null)
  const [stackMenu, setStackMenu] = useState<{ stack: StackListItem; x: number; y: number } | null>(null)
  const [showTerminal, setShowTerminal] = useState<string | null>(null)
  const [showStackTerminal, setShowStackTerminal] = useState<string | null>(null)
  const [showLogs, setShowLogs] = useState<string | null>(null)
  const [showStackLogs, setShowStackLogs] = useState<string | null>(null)
  const [actionLoading, setActionLoading] = useState<string | null>(null)

  const fetchInstalls = useCallback(async () => {
    try {
      const [instData, stackData] = await Promise.all([api.installs(), api.stacks()])
      setInstalls(instData.installs || [])
      setStacks(stackData.stacks || [])
    } catch { /* ignore */ }
    setLoading(false)
  }, [])

  useEffect(() => { fetchInstalls() }, [fetchInstalls])
  useEffect(() => {
    const interval = setInterval(fetchInstalls, 10000)
    return () => clearInterval(interval)
  }, [fetchInstalls])

  const handleAction = async (action: string, installId: string) => {
    setActionLoading(installId)
    try {
      switch (action) {
        case 'start': await api.startContainer(installId); break
        case 'stop': await api.stopContainer(installId); break
        case 'restart': await api.restartContainer(installId); break
        case 'uninstall': {
          const job = await api.uninstall(installId)
          window.location.hash = `#/job/${job.id}`
          return
        }
      }
      setTimeout(fetchInstalls, 1000)
      setTimeout(fetchInstalls, 4000)
    } catch (e) {
      alert(e instanceof Error ? e.message : `${action} failed`)
    }
    setActionLoading(null)
  }

  const handleStackAction = async (action: string, stackId: string) => {
    setActionLoading(`stack-${stackId}`)
    try {
      switch (action) {
        case 'start': await api.startStack(stackId); break
        case 'stop': await api.stopStack(stackId); break
        case 'restart': await api.restartStack(stackId); break
        case 'uninstall': {
          const job = await api.uninstallStack(stackId)
          window.location.hash = `#/job/${job.id}`
          return
        }
      }
      setTimeout(fetchInstalls, 1000)
      setTimeout(fetchInstalls, 4000)
    } catch (e) {
      alert(e instanceof Error ? e.message : `${action} failed`)
    }
    setActionLoading(null)
  }

  const resolveUrl = (inst: InstallListItem, url: string) =>
    inst.ip ? url.replace(/\{\{ip\}\}/g, inst.ip) : url

  const getServiceUrls = (inst: InstallListItem) =>
    inst.outputs
      ? Object.entries(inst.outputs).filter(([, v]) => v.startsWith('http')).map(([k, v]) => ({ key: k, url: resolveUrl(inst, v) }))
      : []

  const gridCols = 'grid-cols-[40px_1.5fr_90px_160px_110px_50px_70px_90px_36px]'

  const getStackAppUrls = (stack: StackListItem, app: StackApp) => {
    if (!app.outputs) return []
    return Object.entries(app.outputs)
      .filter(([, v]) => v.startsWith('http'))
      .map(([k, v]) => ({ key: k, url: stack.ip ? v.replace(/\{\{ip\}\}/g, stack.ip) : v }))
  }

  const isEmpty = installs.length === 0 && stacks.length === 0

  return (
    <div>
      <h2 className="text-xl font-bold text-text-primary mb-5 font-mono">Installed Apps</h2>
      {loading ? <Center>Loading...</Center> : isEmpty ? <Center>No apps installed</Center> : (
        <div className="bg-bg-card border border-border rounded-lg overflow-x-auto">
          {/* Table header */}
          <div className={`grid ${gridCols} gap-2 px-4 py-2 bg-bg-secondary text-[10px] text-text-muted font-mono uppercase tracking-wider border-b border-border items-center`}>
            <span></span>
            <span>App</span>
            <span>Status</span>
            <span>Network</span>
            <span>Resources</span>
            <span>Boot</span>
            <span>Uptime</span>
            <span>Created</span>
            <span></span>
          </div>
          {/* Install rows */}
          {installs.map(inst => {
            const isRunning = inst.status === 'running'
            const isUninstalled = inst.status === 'uninstalled'
            const urls = getServiceUrls(inst)
            const isLoading = actionLoading === inst.id
            return (
              <div key={inst.id}
                className={`grid ${gridCols} gap-2 px-4 py-3 border-b border-border items-center hover:bg-bg-secondary/50 cursor-pointer transition-colors ${isUninstalled ? 'opacity-60' : ''} ${isLoading ? 'opacity-50 pointer-events-none' : ''}`}
                onClick={() => window.location.hash = `#/install/${inst.id}`}>
                {/* Icon */}
                <div className="w-8 h-8 rounded bg-bg-secondary overflow-hidden flex items-center justify-center flex-shrink-0">
                  <img src={`/api/apps/${inst.app_id}/icon`} alt="" className="w-7 h-7 rounded" onError={e => { (e.target as HTMLImageElement).style.display = 'none' }} />
                </div>
                {/* Name+Version+CTID */}
                <div className="min-w-0">
                  <div className="flex items-center gap-1.5 flex-wrap">
                    <span className="text-sm font-semibold text-text-primary truncate">{inst.app_name}</span>
                    {inst.app_version && <span className="text-[10px] text-text-muted font-mono">v{inst.app_version}</span>}
                    {inst.update_available && (
                      <span className="text-[9px] bg-status-warning/20 text-status-warning px-1.5 py-0.5 rounded font-mono">update</span>
                    )}
                  </div>
                  {inst.ctid > 0 && <div className="text-[10px] text-text-muted font-mono">CT {inst.ctid}</div>}
                </div>
                {/* Status */}
                <div className="flex items-center gap-1.5">
                  {isUninstalled ? (
                    <span className="text-[10px] font-mono text-status-warning">uninstalled</span>
                  ) : (
                    <>
                      <StatusDot running={isRunning} />
                      <span className={`text-xs font-mono ${isRunning ? 'text-status-running' : 'text-status-stopped'}`}>{inst.status}</span>
                    </>
                  )}
                </div>
                {/* Network: IP + URLs */}
                <div className="min-w-0 text-xs font-mono text-text-secondary">
                  {inst.ip && <div className="truncate">{inst.ip}</div>}
                  {urls.slice(0, 2).map(u => (
                    <a key={u.key} href={u.url} target="_blank" rel="noreferrer"
                      onClick={e => e.stopPropagation()}
                      className="text-primary hover:underline block truncate text-[10px]">{u.url.replace(/^https?:\/\//, '')}</a>
                  ))}
                  {urls.length > 2 && <span className="text-[10px] text-text-muted">+{urls.length - 2} more</span>}
                </div>
                {/* Resources */}
                <span className="text-xs font-mono text-text-muted">{inst.cores}c / {inst.memory_mb}MB / {inst.disk_gb}GB</span>
                {/* Boot */}
                <span className="text-xs font-mono text-text-muted">{inst.onboot ? 'On' : 'Off'}</span>
                {/* Uptime */}
                <span className="text-xs font-mono text-text-muted">
                  {isRunning && inst.uptime ? formatUptime(inst.uptime) : '-'}
                </span>
                {/* Created */}
                <span className="text-[10px] font-mono text-text-muted">
                  {new Date(inst.created_at).toLocaleDateString()}
                </span>
                {/* Actions button */}
                <button
                  onClick={e => { e.stopPropagation(); setContextMenu({ install: inst, x: e.clientX, y: e.clientY }) }}
                  className="text-text-muted hover:text-primary bg-transparent border-none cursor-pointer text-base font-mono p-0 leading-none"
                  title="Actions">&#x22EE;</button>
              </div>
            )
          })}
          {/* Stack rows */}
          {stacks.map(stack => {
            const isRunning = stack.status === 'running'
            const isStackLoading = actionLoading === `stack-${stack.id}`
            return (
              <div key={`stack-${stack.id}`}>
                {/* Stack header row */}
                <div
                  className={`grid ${gridCols} gap-2 px-4 py-3 border-b border-border items-center hover:bg-bg-secondary/50 cursor-pointer transition-colors bg-bg-secondary/30 ${isStackLoading ? 'opacity-50 pointer-events-none' : ''}`}
                  onClick={() => window.location.hash = `#/stack/${stack.id}`}>
                  {/* Icon */}
                  <div className="w-8 h-8 rounded bg-primary/10 flex items-center justify-center flex-shrink-0">
                    <span className="text-primary text-sm font-mono font-bold">S</span>
                  </div>
                  {/* Name */}
                  <div className="min-w-0">
                    <div className="flex items-center gap-1.5 flex-wrap">
                      <span className="text-sm font-semibold text-text-primary truncate">{stack.name}</span>
                      <span className="text-[9px] bg-primary/15 text-primary px-1.5 py-0.5 rounded font-mono">stack</span>
                      <span className="text-[10px] text-text-muted font-mono">{stack.apps.length} app{stack.apps.length !== 1 ? 's' : ''}</span>
                    </div>
                    {stack.ctid > 0 && <div className="text-[10px] text-text-muted font-mono">CT {stack.ctid}</div>}
                  </div>
                  {/* Status */}
                  <div className="flex items-center gap-1.5">
                    <StatusDot running={isRunning} />
                    <span className={`text-xs font-mono ${isRunning ? 'text-status-running' : 'text-status-stopped'}`}>{stack.status}</span>
                  </div>
                  {/* Network */}
                  <div className="min-w-0 text-xs font-mono text-text-secondary">
                    {stack.ip && <div className="truncate">{stack.ip}</div>}
                  </div>
                  {/* Resources */}
                  <span className="text-xs font-mono text-text-muted">{stack.cores}c / {stack.memory_mb}MB / {stack.disk_gb}GB</span>
                  {/* Boot */}
                  <span className="text-xs font-mono text-text-muted">{stack.onboot ? 'On' : 'Off'}</span>
                  {/* Uptime */}
                  <span className="text-xs font-mono text-text-muted">
                    {isRunning && stack.uptime ? formatUptime(stack.uptime) : '-'}
                  </span>
                  {/* Created */}
                  <span className="text-[10px] font-mono text-text-muted">
                    {new Date(stack.created_at).toLocaleDateString()}
                  </span>
                  {/* Actions button */}
                  <button
                    onClick={e => { e.stopPropagation(); setStackMenu({ stack, x: e.clientX, y: e.clientY }) }}
                    className="text-text-muted hover:text-primary bg-transparent border-none cursor-pointer text-base font-mono p-0 leading-none"
                    title="Actions">&#x22EE;</button>
                </div>
                {/* Indented app rows within stack */}
                {stack.apps.map(app => {
                  const appUrls = getStackAppUrls(stack, app)
                  const appDisplayStatus = app.status === 'completed' ? 'installed' : app.status
                  return (
                    <div key={`stack-${stack.id}-${app.app_id}`}
                      className="grid grid-cols-[40px_1.5fr_90px_160px_1fr] gap-2 px-4 py-2 border-b border-border/50 items-center pl-12 bg-bg-primary/50 cursor-pointer hover:bg-bg-secondary/30 transition-colors"
                      onClick={() => window.location.hash = `#/app/${app.app_id}`}>
                      {/* Icon */}
                      <div className="w-6 h-6 rounded bg-bg-secondary overflow-hidden flex items-center justify-center flex-shrink-0">
                        <img src={`/api/apps/${app.app_id}/icon`} alt="" className="w-5 h-5 rounded" onError={e => { (e.target as HTMLImageElement).style.display = 'none' }} />
                      </div>
                      {/* Name */}
                      <div className="min-w-0">
                        <div className="flex items-center gap-1.5">
                          <span className="text-xs text-text-secondary truncate">{app.app_name}</span>
                          {app.app_version && <span className="text-[9px] text-text-muted font-mono">v{app.app_version}</span>}
                        </div>
                      </div>
                      {/* Status */}
                      <span className={`text-[10px] font-mono ${app.status === 'completed' ? 'text-status-running' : app.status === 'failed' ? 'text-status-stopped' : 'text-text-muted'}`}>
                        {appDisplayStatus}
                      </span>
                      {/* URLs */}
                      <div className="min-w-0 text-xs font-mono">
                        {appUrls.slice(0, 2).map(u => (
                          <a key={u.key} href={u.url} target="_blank" rel="noreferrer"
                            onClick={e => e.stopPropagation()}
                            className="text-primary hover:underline block truncate text-[10px]">{u.url.replace(/^https?:\/\//, '')}</a>
                        ))}
                      </div>
                      <span></span>
                    </div>
                  )
                })}
              </div>
            )
          })}
        </div>
      )}

      {/* Context Menu — Installs */}
      {contextMenu && (
        <InstallContextMenu
          install={contextMenu.install}
          x={contextMenu.x}
          y={contextMenu.y}
          onClose={() => setContextMenu(null)}
          requireAuth={requireAuth}
          onAction={(action, id) => { setContextMenu(null); requireAuth(() => handleAction(action, id)) }}
          onShell={id => { setContextMenu(null); requireAuth(() => setShowTerminal(id)) }}
          onLogs={id => { setContextMenu(null); setShowLogs(id) }}
        />
      )}

      {/* Context Menu — Stacks */}
      {stackMenu && (
        <StackContextMenu
          stack={stackMenu.stack}
          x={stackMenu.x}
          y={stackMenu.y}
          onClose={() => setStackMenu(null)}
          onAction={(action, id) => { setStackMenu(null); requireAuth(() => handleStackAction(action, id)) }}
          onShell={id => { setStackMenu(null); requireAuth(() => setShowStackTerminal(id)) }}
          onLogs={id => { setStackMenu(null); setShowStackLogs(id) }}
        />
      )}

      {showTerminal && <TerminalModal installId={showTerminal} onClose={() => setShowTerminal(null)} />}
      {showStackTerminal && <StackTerminalModal stackId={showStackTerminal} onClose={() => setShowStackTerminal(null)} />}
      {showLogs && <LogViewerModal installId={showLogs} onClose={() => setShowLogs(null)} />}
      {showStackLogs && <StackLogViewerModal stackId={showStackLogs} onClose={() => setShowStackLogs(null)} />}
    </div>
  )
}

function InstallContextMenu({ install, x, y, onClose, onAction, onShell, onLogs }: {
  install: InstallListItem;
  x: number; y: number;
  onClose: () => void;
  requireAuth: (cb: () => void) => void;
  onAction: (action: string, id: string) => void;
  onShell: (id: string) => void;
  onLogs: (id: string) => void;
}) {
  const isRunning = install.status === 'running'
  const isStopped = install.status === 'stopped'
  const isUninstalled = install.status === 'uninstalled'

  const menuY = Math.min(y, window.innerHeight - 320)
  const menuX = Math.min(x, window.innerWidth - 200)

  return (
    <>
      <div className="fixed inset-0 z-[299]" onClick={onClose} />
      <div style={{ position: 'fixed', top: menuY, left: menuX, zIndex: 300 }}
        className="bg-bg-card border border-border rounded-lg shadow-lg py-1 min-w-[170px]">
        {isStopped && (
          <CtxMenuItem label="Start" onClick={() => onAction('start', install.id)} />
        )}
        {isRunning && (
          <>
            <CtxMenuItem label="Stop" onClick={() => onAction('stop', install.id)} />
            <CtxMenuItem label="Restart" onClick={() => onAction('restart', install.id)} />
          </>
        )}
        {(isRunning || isStopped) && <div className="border-t border-border my-1" />}
        {isRunning && (
          <>
            <CtxMenuItem label="Logs" onClick={() => onLogs(install.id)} />
            <CtxMenuItem label="Shell" onClick={() => onShell(install.id)} />
            <div className="border-t border-border my-1" />
          </>
        )}
        <CtxMenuItem label="Details" onClick={() => { onClose(); window.location.hash = `#/install/${install.id}` }} />
        <CtxMenuItem label="App Store Page" onClick={() => { onClose(); window.location.hash = `#/app/${install.app_id}` }} />
        {!isUninstalled && (
          <>
            <div className="border-t border-border my-1" />
            <CtxMenuItem label="Remove" danger onClick={() => onAction('uninstall', install.id)} />
          </>
        )}
      </div>
    </>
  )
}

function CtxMenuItem({ label, onClick, danger }: { label: string; onClick: () => void; danger?: boolean }) {
  return (
    <button onClick={onClick}
      className={`w-full text-left px-4 py-2 text-sm font-mono bg-transparent border-none cursor-pointer transition-colors ${danger ? 'text-status-stopped hover:bg-status-stopped/10' : 'text-text-secondary hover:bg-bg-secondary hover:text-text-primary'}`}>
      {label}
    </button>
  )
}

function LogViewerModal({ installId, onClose }: { installId: string; onClose: () => void }) {
  const termRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!termRef.current) return

    const term = new Terminal({
      cursorBlink: false,
      disableStdin: true,
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 13,
      theme: {
        background: '#0A0A0A',
        foreground: '#9CA3AF',
        cursor: 'transparent',
        selectionBackground: 'rgba(0,255,157,0.3)',
      },
    })

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.loadAddon(new WebLinksAddon())

    term.open(termRef.current)
    fitAddon.fit()

    let ws: WebSocket | null = null
    let cancelled = false

    api.terminalToken().then(({ token }) => {
      if (cancelled) return

      const wsUrl = api.journalLogsUrl(installId, token)
      ws = new WebSocket(wsUrl)

      ws.onopen = () => {
        term.writeln('\x1b[32m--- Journal log stream started ---\x1b[0m\r')
      }

      ws.onmessage = (event) => {
        const text = typeof event.data === 'string' ? event.data : new TextDecoder().decode(event.data)
        for (const line of text.split('\n')) {
          if (line) term.writeln(line)
        }
      }

      ws.onclose = () => {
        term.writeln('\r\n\x1b[31m--- Log stream ended ---\x1b[0m')
      }

      ws.onerror = () => {
        term.writeln('\r\n\x1b[31mWebSocket error.\x1b[0m')
      }
    }).catch(() => {
      term.writeln('\x1b[31mFailed to get log token. Are you logged in?\x1b[0m')
    })

    const handleResize = () => fitAddon.fit()
    window.addEventListener('resize', handleResize)

    return () => {
      cancelled = true
      window.removeEventListener('resize', handleResize)
      if (ws) ws.close()
      term.dispose()
    }
  }, [installId])

  return (
    <div className="fixed inset-0 bg-black/95 flex flex-col z-[200]">
      <div className="flex items-center justify-between px-4 py-2 bg-bg-card border-b border-border">
        <span className="text-text-secondary font-mono text-sm">journalctl &mdash; {installId}</span>
        <button onClick={onClose} className="text-text-muted hover:text-text-primary bg-transparent border-none cursor-pointer text-lg font-mono">&times;</button>
      </div>
      <div ref={termRef} className="flex-1 p-2" />
    </div>
  )
}

// --- Config View ---

function ConfigView({ requireAuth }: { requireAuth: (cb: () => void) => void }) {
  const [defaults, setDefaults] = useState<ConfigDefaultsResponse | null>(null)
  const [exportData, setExportData] = useState<ExportResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [applyYaml, setApplyYaml] = useState('')
  const [applyPreview, setApplyPreview] = useState<ExportRecipe[] | null>(null)
  const [applyError, setApplyError] = useState('')
  const [applyJobs, setApplyJobs] = useState<{ app_id: string; job_id: string }[]>([])
  const [applying, setApplying] = useState(false)
  const [installCount, setInstallCount] = useState(0)

  useEffect(() => {
    api.configDefaults().then(setDefaults).catch(() => {})
    api.installs().then(d => setInstallCount(d.total || 0)).catch(() => {})
  }, [])

  const handleExport = () => {
    requireAuth(async () => {
      setLoading(true)
      try {
        const data = await api.configExport()
        setExportData(data)
      } catch { /* ignore */ }
      setLoading(false)
    })
  }

  const handleDownload = () => {
    requireAuth(() => { api.configExportDownload() })
  }

  const handleCopy = async () => {
    if (!exportData) return
    const yaml = exportData.recipes.map(r => JSON.stringify(r, null, 2)).join('\n\n')
    await navigator.clipboard.writeText(yaml)
  }

  const handlePreview = () => {
    setApplyError('')
    setApplyPreview(null)
    try {
      const parsed = JSON.parse(applyYaml)
      const recipes = Array.isArray(parsed) ? parsed : parsed.recipes || [parsed]
      if (!Array.isArray(recipes) || recipes.length === 0) {
        setApplyError('No recipes found. Paste a JSON array of recipes or an export object.')
        return
      }
      for (const r of recipes) {
        if (!r.app_id) { setApplyError('Each recipe must have an app_id'); return }
      }
      setApplyPreview(recipes)
    } catch {
      setApplyError('Invalid JSON. Paste the recipes array from an export.')
    }
  }

  const handleApply = () => {
    if (!applyPreview) return
    requireAuth(async () => {
      setApplying(true)
      setApplyError('')
      try {
        const result = await api.configApply(applyPreview as InstallRequest[])
        setApplyJobs(result.jobs)
        setApplyPreview(null)
        setApplyYaml('')
      } catch (e: unknown) {
        setApplyError(e instanceof Error ? e.message : 'Apply failed')
      }
      setApplying(false)
    })
  }

  return (
    <div>
      <h2 className="text-xl font-bold text-text-primary mb-5 font-mono">Configuration</h2>

      {/* Section 1: Current Setup */}
      <InfoCard title="Current Setup">
        {defaults && (
          <div className="space-y-1">
            <InfoRow label="Storages" value={defaults.storages.join(', ') || '-'} />
            {defaults.storage_details && defaults.storage_details.length > 0 && (
              <div className="flex gap-1.5 flex-wrap my-1">
                {defaults.storage_details.map(sd => (
                  <Badge key={sd.id} className={sd.browsable ? 'bg-primary/10 text-primary' : 'bg-status-warning/10 text-status-warning'}>
                    {sd.id} ({sd.type}{sd.browsable ? '' : ', block'})
                  </Badge>
                ))}
              </div>
            )}
            <InfoRow label="Bridges" value={defaults.bridges.join(', ') || '-'} />
            <InfoRow label="Default Resources" value={`${defaults.defaults.cores}c / ${defaults.defaults.memory_mb}MB / ${defaults.defaults.disk_gb}GB`} />
            <InfoRow label="Active Installs" value={String(installCount)} />
          </div>
        )}
      </InfoCard>

      {/* Section 2: Export */}
      <div className="mt-4 bg-bg-card border border-border rounded-lg p-5">
        <h3 className="text-xs font-semibold text-text-muted mb-3 uppercase tracking-wider font-mono">Export Recipes</h3>
        <div className="flex gap-2 mb-3">
          <button onClick={handleExport} disabled={loading} className="px-4 py-2 text-sm font-mono border-none rounded-lg cursor-pointer bg-primary text-bg-primary hover:shadow-[0_0_20px_rgba(0,255,157,0.3)] transition-all disabled:opacity-50 font-semibold">
            {loading ? 'Exporting...' : 'Export Recipes'}
          </button>
          <button onClick={handleDownload} className="px-4 py-2 text-sm font-mono border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-primary hover:text-primary transition-colors">
            Download YAML
          </button>
          {exportData && (
            <button onClick={handleCopy} className="px-4 py-2 text-sm font-mono border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-primary hover:text-primary transition-colors">
              Copy to Clipboard
            </button>
          )}
        </div>
        {exportData && (
          <div>
            <div className="text-xs text-text-muted font-mono mb-2">
              Exported {exportData.recipes.length} recipe(s) from {exportData.node} at {new Date(exportData.exported_at).toLocaleString()}
            </div>
            <pre className="bg-bg-secondary border border-border rounded-lg p-4 text-xs font-mono text-text-secondary max-h-[400px] overflow-auto whitespace-pre-wrap">
              {JSON.stringify(exportData.recipes, null, 2)}
            </pre>
          </div>
        )}
      </div>

      {/* Section 3: Apply / Restore */}
      <div className="mt-4 bg-bg-card border border-border rounded-lg p-5">
        <h3 className="text-xs font-semibold text-text-muted mb-3 uppercase tracking-wider font-mono">Apply / Restore</h3>
        <p className="text-xs text-text-muted mb-3">Paste exported recipes JSON to restore apps with the same configuration.</p>
        <textarea value={applyYaml} onChange={e => setApplyYaml(e.target.value)} placeholder='Paste recipes JSON here...'
          rows={6}
          className="w-full px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono placeholder:text-text-muted resize-y" />
        {applyError && <div className="text-status-stopped text-xs mt-2 font-mono">{applyError}</div>}
        <div className="flex gap-2 mt-3">
          <button onClick={handlePreview} disabled={!applyYaml.trim()} className="px-4 py-2 text-sm font-mono border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-primary hover:text-primary transition-colors disabled:opacity-50">
            Preview
          </button>
          {applyPreview && (
            <button onClick={handleApply} disabled={applying} className="px-4 py-2 text-sm font-mono border-none rounded-lg cursor-pointer bg-primary text-bg-primary hover:shadow-[0_0_20px_rgba(0,255,157,0.3)] transition-all disabled:opacity-50 font-semibold">
              {applying ? 'Applying...' : `Apply ${applyPreview.length} Recipe(s)`}
            </button>
          )}
        </div>

        {applyPreview && (
          <div className="mt-3 border border-border rounded-lg overflow-hidden">
            <div className="bg-bg-secondary px-3 py-2 text-xs font-mono text-text-muted uppercase tracking-wider">Preview</div>
            {applyPreview.map((r, i) => (
              <div key={i} className="flex justify-between items-center px-3 py-2 border-t border-border">
                <div className="flex items-center gap-2">
                  <span className="text-sm text-text-primary font-semibold">{r.app_id}</span>
                  <span className="text-xs text-text-muted font-mono">{r.cores}c / {r.memory_mb}MB / {r.disk_gb}GB</span>
                </div>
                <span className="text-xs text-text-muted font-mono">{r.storage}</span>
              </div>
            ))}
          </div>
        )}

        {applyJobs.length > 0 && (
          <div className="mt-3 border border-primary/30 rounded-lg bg-primary/10 p-3">
            <div className="text-xs text-primary font-mono mb-2">Jobs created:</div>
            {applyJobs.map(j => (
              <a key={j.job_id} href={`#/job/${j.job_id}`} className="block text-sm text-primary font-mono hover:underline py-0.5">
                {j.app_id} &rarr; {j.job_id}
              </a>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

// --- Jobs List ---

function JobsList() {
  const [jobs, setJobs] = useState<Job[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => { api.jobs().then(d => { setJobs(d.jobs || []); setLoading(false) }) }, [])

  return (
    <div>
      <h2 className="text-xl font-bold text-text-primary mb-5 font-mono">Jobs</h2>
      {loading ? <Center>Loading...</Center> : jobs.length === 0 ? <Center>No jobs yet</Center> : (
        <div className="flex flex-col gap-2">
          {jobs.map(j => (
            <a key={j.id} href={`#/job/${j.id}`} className="bg-bg-card border border-border rounded-lg px-4 py-3 no-underline text-inherit flex justify-between items-center hover:border-border-hover transition-colors">
              <div className="flex items-center gap-3">
                <span className="text-text-primary font-semibold">{j.app_name}</span>
                <span className="text-text-muted text-sm font-mono">{j.type}</span>
                {j.ctid > 0 && <span className="text-text-muted text-sm font-mono">CT {j.ctid}</span>}
              </div>
              <StateBadge state={j.state} />
            </a>
          ))}
        </div>
      )}
    </div>
  )
}

// --- Terminal Modal (xterm.js + WebSocket) ---

function TerminalModal({ installId, onClose }: { installId: string; onClose: () => void }) {
  const termRef = useRef<HTMLDivElement>(null)
  const termInstance = useRef<Terminal | null>(null)
  const wsRef = useRef<WebSocket | null>(null)

  useEffect(() => {
    if (!termRef.current) return

    const term = new Terminal({
      cursorBlink: true,
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 14,
      theme: {
        background: '#0A0A0A',
        foreground: '#00FF9D',
        cursor: '#00FF9D',
        selectionBackground: 'rgba(0,255,157,0.3)',
      },
    })
    termInstance.current = term

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.loadAddon(new WebLinksAddon())

    term.open(termRef.current)
    fitAddon.fit()

    // Fetch a short-lived terminal token, then connect WebSocket
    let ws: WebSocket | null = null
    let cancelled = false

    api.terminalToken().then(({ token }) => {
      if (cancelled) return

      const wsUrl = api.terminalUrl(installId, token)
      ws = new WebSocket(wsUrl)
      wsRef.current = ws

      ws.binaryType = 'arraybuffer'

      ws.onopen = () => {
        term.writeln('\x1b[32mConnected to container shell.\x1b[0m\r\n')
        ws!.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
      }

      ws.onmessage = (event) => {
        if (event.data instanceof ArrayBuffer) {
          term.write(new Uint8Array(event.data))
        } else {
          term.write(event.data)
        }
      }

      ws.onclose = () => {
        term.writeln('\r\n\x1b[31mConnection closed.\x1b[0m')
      }

      ws.onerror = () => {
        term.writeln('\r\n\x1b[31mWebSocket error.\x1b[0m')
      }

      term.onData((data) => {
        if (ws && ws.readyState === WebSocket.OPEN) {
          ws.send(data)
        }
      })
    }).catch(() => {
      term.writeln('\x1b[31mFailed to get terminal token. Are you logged in?\x1b[0m')
    })

    // Handle resize
    const handleResize = () => {
      fitAddon.fit()
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
      }
    }
    window.addEventListener('resize', handleResize)

    return () => {
      cancelled = true
      window.removeEventListener('resize', handleResize)
      if (ws) ws.close()
      term.dispose()
    }
  }, [installId])

  return (
    <div className="fixed inset-0 bg-black/95 flex flex-col z-[200]">
      <div className="flex items-center justify-between px-4 py-2 bg-bg-card border-b border-border">
        <span className="text-primary font-mono text-sm">&gt;_ Terminal — {installId}</span>
        <button onClick={onClose} className="text-text-muted hover:text-text-primary bg-transparent border-none cursor-pointer text-lg font-mono">&times;</button>
      </div>
      <div ref={termRef} className="flex-1 p-2" />
    </div>
  )
}

// --- Directory Browser ---

function DirectoryBrowser({ initialPath, onSelect, onClose }: { initialPath: string; onSelect: (path: string) => void; onClose: () => void }) {
  const [path, setPath] = useState(initialPath || '')
  const [entries, setEntries] = useState<BrowseEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [manualPath, setManualPath] = useState(initialPath || '')
  const [mounts, setMounts] = useState<MountInfo[]>([])
  const [newFolderName, setNewFolderName] = useState('')
  const [showNewFolder, setShowNewFolder] = useState(false)
  const [refreshKey, setRefreshKey] = useState(0)

  useEffect(() => {
    api.browseMounts().then(d => {
      const m = d.mounts || []
      setMounts(m)
      // If the current path isn't under any allowed mount, redirect to the first one
      if (m.length > 0 && !m.some(mt => path === mt.path || path.startsWith(mt.path + '/'))) {
        setPath(m[0].path)
        setManualPath(m[0].path)
      }
    }).catch(() => {})
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!path) return // wait for mounts to resolve the initial path
    setLoading(true)
    setError('')
    api.browsePaths(path).then(d => {
      setEntries(d.entries)
      setManualPath(d.path)
      setLoading(false)
    }).catch(e => {
      setError(e instanceof Error ? e.message : 'Failed to browse')
      setEntries([])
      setLoading(false)
    })
  }, [path, refreshKey])

  const handleCreateFolder = async () => {
    if (!newFolderName.trim()) return
    try {
      await api.browseMkdir(path + '/' + newFolderName.trim())
      setNewFolderName('')
      setShowNewFolder(false)
      setRefreshKey(k => k + 1)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to create folder')
    }
  }

  const bookmarks = mounts.map(m => ({ label: m.path, path: m.path }))

  const pathSegments = path.split('/').filter(Boolean)

  return (
    <div className="fixed inset-0 bg-black/80 flex items-center justify-center z-[150]" onClick={onClose}>
      <div className="bg-bg-card border border-border rounded-xl p-6 w-full max-w-[500px] max-h-[70vh] flex flex-col" onClick={e => e.stopPropagation()}>
        <h3 className="text-sm font-bold text-text-primary mb-3 font-mono">Browse Host Path</h3>

        {/* Bookmark chips */}
        <div className="flex gap-1.5 mb-3 flex-wrap">
          {bookmarks.map(bm => (
            <button key={bm.path} onClick={() => setPath(bm.path)}
              className={`px-2.5 py-1 text-[11px] font-mono rounded-md border cursor-pointer transition-colors ${
                path.startsWith(bm.path) ? 'border-primary text-primary bg-primary/10' : 'border-border text-text-muted bg-bg-secondary hover:border-primary hover:text-primary'
              }`}>
              {bm.label}
            </button>
          ))}
        </div>

        <div className="flex gap-2 mb-3">
          <input type="text" value={manualPath} onChange={e => setManualPath(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') setPath(manualPath) }}
            className="flex-1 px-3 py-1.5 bg-bg-secondary border border-border rounded-md text-text-primary text-xs outline-none focus:border-primary font-mono" />
          <button onClick={() => setPath(manualPath)} className="px-3 py-1.5 text-xs font-mono border border-border rounded-md text-text-secondary bg-bg-secondary hover:border-primary hover:text-primary transition-colors cursor-pointer">Go</button>
        </div>

        <div className="flex items-center gap-1 mb-2 text-xs font-mono flex-wrap">
          <button onClick={() => setPath('/')} className="text-primary hover:underline bg-transparent border-none cursor-pointer p-0 font-mono text-xs">/</button>
          {pathSegments.map((seg, i) => (
            <span key={i} className="flex items-center gap-1">
              <span className="text-text-muted">/</span>
              <button onClick={() => setPath('/' + pathSegments.slice(0, i + 1).join('/'))}
                className="text-primary hover:underline bg-transparent border-none cursor-pointer p-0 font-mono text-xs">{seg}</button>
            </span>
          ))}
        </div>

        <div className="flex-1 overflow-auto border border-border rounded-md bg-bg-secondary">
          {loading ? (
            <div className="p-4 text-text-muted text-xs font-mono">Loading...</div>
          ) : error ? (
            <div className="p-4 text-status-stopped text-xs font-mono">{error}</div>
          ) : (
            <>
              {path !== '/' && (
                <button onClick={() => setPath(path.replace(/\/[^/]+\/?$/, '') || '/')}
                  className="w-full text-left px-3 py-1.5 text-xs font-mono text-primary hover:bg-primary/10 transition-colors bg-transparent border-none border-b border-border cursor-pointer flex items-center gap-2">
                  <span className="text-text-muted">..</span> <span className="text-text-secondary">(up)</span>
                </button>
              )}
              {entries.filter(e => e.is_dir).map(entry => (
                <button key={entry.path} onClick={() => setPath(entry.path)}
                  className="w-full text-left px-3 py-1.5 text-xs font-mono text-text-primary hover:bg-primary/10 transition-colors bg-transparent border-none cursor-pointer flex items-center gap-2">
                  <span className="text-primary">&#128193;</span> {entry.name}/
                </button>
              ))}
              {entries.filter(e => !e.is_dir).map(entry => (
                <div key={entry.path} className="px-3 py-1.5 text-xs font-mono text-text-muted flex items-center gap-2">
                  <span>&#128196;</span> {entry.name}
                </div>
              ))}
              {entries.length === 0 && !error && (
                <div className="p-4 text-text-muted text-xs font-mono italic">Empty directory</div>
              )}
            </>
          )}
        </div>

        {/* New Folder inline */}
        <div className="mt-2 flex items-center gap-2">
          {showNewFolder ? (
            <>
              <input type="text" value={newFolderName} onChange={e => setNewFolderName(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') handleCreateFolder(); if (e.key === 'Escape') { setShowNewFolder(false); setNewFolderName('') } }}
                placeholder="folder name" autoFocus
                className="flex-1 px-3 py-1.5 bg-bg-secondary border border-border rounded-md text-text-primary text-xs outline-none focus:border-primary font-mono placeholder:text-text-muted" />
              <button onClick={handleCreateFolder} className="px-3 py-1.5 text-xs font-mono border-none rounded-md text-bg-primary bg-primary cursor-pointer hover:shadow-[0_0_10px_rgba(0,255,157,0.3)] transition-all">Create</button>
              <button onClick={() => { setShowNewFolder(false); setNewFolderName('') }} className="px-2 py-1.5 text-xs font-mono text-text-muted bg-transparent border-none cursor-pointer hover:text-text-primary">&times;</button>
            </>
          ) : (
            <button onClick={() => setShowNewFolder(true)} className="text-primary text-xs font-mono bg-transparent border-none cursor-pointer hover:underline p-0">
              + New Folder
            </button>
          )}
        </div>

        <div className="flex gap-3 mt-4 justify-end">
          <button onClick={onClose} className="px-4 py-2 text-xs font-semibold border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-text-secondary transition-colors font-mono">Cancel</button>
          <button onClick={() => onSelect(path)} className="px-4 py-2 text-xs font-semibold border-none rounded-lg cursor-pointer bg-primary text-bg-primary hover:shadow-[0_0_20px_rgba(0,255,157,0.3)] transition-all font-mono">
            Select: {path}
          </button>
        </div>
      </div>
    </div>
  )
}

// --- Login Modal ---

function LoginModal({ onSuccess, onClose }: { onSuccess: () => void; onClose: () => void }) {
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true); setError('')
    try {
      await api.login(password)
      onSuccess()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Login failed')
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-[200]">
      <form onSubmit={handleSubmit} className="bg-bg-card border border-border rounded-xl p-8 w-full max-w-[380px]">
        <h2 className="text-lg font-bold text-text-primary mb-2 font-mono">Login Required</h2>
        <p className="text-sm text-text-muted mb-5">Enter your password to perform this action.</p>
        <FormInput value={password} onChange={setPassword} type="password" />
        {error && <div className="text-status-stopped text-sm mt-2 font-mono">{error}</div>}
        <div className="flex gap-3 mt-5 justify-end">
          <button type="button" onClick={onClose} className="px-5 py-2.5 text-sm font-semibold border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-text-secondary transition-colors font-mono">Cancel</button>
          <button type="submit" disabled={loading || !password} className="px-5 py-2.5 text-sm font-semibold border-none rounded-lg cursor-pointer bg-primary text-bg-primary hover:shadow-[0_0_20px_rgba(0,255,157,0.3)] transition-all disabled:opacity-50 font-mono">
            {loading ? 'Logging in...' : 'Login'}
          </button>
        </div>
      </form>
    </div>
  )
}

// --- Shared Components ---

function Center({ children, className }: { children: React.ReactNode; className?: string }) {
  return <div className={`text-center py-12 text-text-muted ${className || ''}`}>{children}</div>
}

function BackLink({ href = '#/', label = 'Back to apps' }: { href?: string; label?: string }) {
  return <a href={href} className="text-primary text-sm no-underline font-mono hover:underline">&larr; {label}</a>
}

function Badge({ children, className }: { children: React.ReactNode; className?: string }) {
  return <span className={`text-[11px] px-2 py-0.5 rounded font-mono ${className || ''}`}>{children}</span>
}

function StatusDot({ running }: { running: boolean }) {
  return <span className={`inline-block w-2.5 h-2.5 rounded-full ${running ? 'bg-status-running animate-pulse-glow text-status-running' : 'bg-status-stopped'}`} />
}

function StateBadge({ state }: { state: string }) {
  const cls = state === 'completed' ? 'bg-status-running/10 text-status-running' : state === 'failed' ? 'bg-status-stopped/10 text-status-stopped' : state === 'cancelled' ? 'bg-text-muted/10 text-text-muted' : 'bg-status-warning/10 text-status-warning'
  return <span className={`text-xs px-2.5 py-1 rounded font-mono font-semibold ${cls}`}>{state}</span>
}

function ResourceCard({ label, value, sub, pct }: { label: string; value: string; sub?: string; pct?: number }) {
  return (
    <div className="bg-bg-card border border-border rounded-lg p-4">
      <div className="text-xs text-text-muted uppercase font-mono tracking-wider mb-2">{label}</div>
      <div className="text-xl font-bold text-text-primary font-mono">{value}</div>
      {sub && <div className="text-xs text-text-muted font-mono mt-1">/ {sub}</div>}
      {pct !== undefined && (
        <div className="mt-2 h-1.5 bg-bg-secondary rounded-full overflow-hidden">
          <div className={`h-full rounded-full transition-all duration-500 ${pct > 80 ? 'bg-status-stopped' : pct > 50 ? 'bg-status-warning' : 'bg-primary'}`} style={{ width: `${Math.min(pct, 100)}%` }} />
        </div>
      )}
    </div>
  )
}

function InfoCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-bg-card border border-border rounded-lg p-5">
      <h3 className="text-xs font-semibold text-text-muted mb-3 uppercase tracking-wider font-mono">{title}</h3>
      {children}
    </div>
  )
}

function Linkify({ text }: { text: string }) {
  const urlRegex = /(https?:\/\/[^\s<>"']+)/g
  const parts = text.split(urlRegex)
  if (parts.length === 1) return <>{text}</>
  return <>{parts.map((part, i) => urlRegex.test(part)
    ? <a key={i} href={part} target="_blank" rel="noreferrer" className="text-primary hover:underline">{part}</a>
    : part
  )}</>
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between py-1 text-sm">
      <span className="text-text-muted">{label}</span>
      <span className="text-text-secondary text-right break-all"><Linkify text={value} /></span>
    </div>
  )
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return <h4 className="text-xs text-primary uppercase mt-5 mb-2 tracking-wider font-mono">{children}</h4>
}

function FormRow({ label, help, description, children }: { label: string; help?: string; description?: string; children: React.ReactNode }) {
  return (
    <div className="mb-3">
      <label className="text-sm text-text-secondary block mb-1">{label}</label>
      {description && <div className="text-xs text-text-muted mb-1.5 leading-relaxed">{description}</div>}
      {children}
      {help && <div className="text-[11px] text-text-muted mt-0.5 italic">{help}</div>}
    </div>
  )
}

function FormInput({ value, onChange, type = 'text', placeholder }: { value: string; onChange: (v: string) => void; type?: string; placeholder?: string }) {
  return <input type={type} value={value} onChange={e => onChange(e.target.value)} placeholder={placeholder} className="w-full px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary focus:ring-1 focus:ring-primary transition-colors font-mono placeholder:text-text-muted" />
}

function FormField({ label, description, help, children }: { label: string; description?: string; help?: string; children: React.ReactNode }) {
  return (
    <div className="mb-3">
      <label className="text-xs text-text-muted font-mono mb-1 block">{label}</label>
      {description && <div className="text-[10px] text-text-muted mb-1">{description}</div>}
      {children}
      {help && <div className="text-[10px] text-text-muted mt-1">{help}</div>}
    </div>
  )
}

// --- Stacks ---

function StacksList({ requireAuth }: { requireAuth: (cb: () => void) => void }) {
  const [stacks, setStacks] = useState<StackListItem[]>([])
  const [loading, setLoading] = useState(true)
  const [contextMenu, setContextMenu] = useState<{ stack: StackListItem; x: number; y: number } | null>(null)
  const [showTerminal, setShowTerminal] = useState<string | null>(null)
  const [showLogs, setShowLogs] = useState<string | null>(null)
  const [actionLoading, setActionLoading] = useState<string | null>(null)

  const fetchStacks = useCallback(async () => {
    try {
      const d = await api.stacks()
      setStacks(d.stacks || [])
    } catch { /* ignore */ }
    setLoading(false)
  }, [])

  useEffect(() => { fetchStacks() }, [fetchStacks])
  useEffect(() => {
    const interval = setInterval(fetchStacks, 10000)
    return () => clearInterval(interval)
  }, [fetchStacks])

  const handleAction = async (action: string, stackId: string) => {
    setActionLoading(stackId)
    try {
      switch (action) {
        case 'start': await api.startStack(stackId); break
        case 'stop': await api.stopStack(stackId); break
        case 'restart': await api.restartStack(stackId); break
        case 'uninstall': {
          const job = await api.uninstallStack(stackId)
          window.location.hash = `#/job/${job.id}`
          return
        }
      }
      setTimeout(fetchStacks, 1000)
      setTimeout(fetchStacks, 4000)
    } catch (e) {
      alert(e instanceof Error ? e.message : `${action} failed`)
    }
    setActionLoading(null)
  }

  const gridCols = 'grid-cols-[64px_1.5fr_80px_90px_140px_110px_50px_70px_90px_36px]'

  return (
    <div>
      <div className="flex items-center justify-between mb-5">
        <h2 className="text-xl font-bold text-text-primary font-mono">Stacks</h2>
        <button onClick={() => { requireAuth(() => { window.location.hash = '#/create-stack' }) }}
          className="px-4 py-2 bg-primary text-bg-primary font-mono text-sm font-bold rounded-md cursor-pointer hover:opacity-90 transition-opacity border-none">
          + New Stack
        </button>
      </div>
      {loading ? <Center>Loading...</Center> : stacks.length === 0 ? <Center>No stacks created. <a href="#/create-stack" className="text-primary hover:underline ml-1">Create one</a></Center> : (
        <div className="bg-bg-card border border-border rounded-lg overflow-x-auto">
          <div className={`grid ${gridCols} gap-2 px-4 py-2 bg-bg-secondary text-[10px] text-text-muted font-mono uppercase tracking-wider border-b border-border items-center`}>
            <span>Icons</span>
            <span>Stack</span>
            <span>Apps</span>
            <span>Status</span>
            <span>Network</span>
            <span>Resources</span>
            <span>Boot</span>
            <span>Uptime</span>
            <span>Created</span>
            <span></span>
          </div>
          {stacks.map(stack => {
            const isRunning = stack.status === 'running'
            const isLoading = actionLoading === stack.id
            return (
              <div key={stack.id}
                className={`grid ${gridCols} gap-2 px-4 py-3 border-b border-border items-center hover:bg-bg-secondary/50 cursor-pointer transition-colors ${isLoading ? 'opacity-50 pointer-events-none' : ''}`}
                onClick={() => window.location.hash = `#/stack/${stack.id}`}>
                {/* Overlapping icons */}
                <div className="flex items-center -space-x-2">
                  {stack.apps.slice(0, 3).map((app, i) => (
                    <div key={app.app_id} className="w-7 h-7 rounded bg-bg-secondary overflow-hidden border-2 border-bg-card flex items-center justify-center" style={{ zIndex: 3 - i }}>
                      <img src={`/api/apps/${app.app_id}/icon`} alt="" className="w-5 h-5 rounded" onError={e => { (e.target as HTMLImageElement).style.display = 'none' }} />
                    </div>
                  ))}
                  {stack.apps.length > 3 && <span className="text-[10px] text-text-muted font-mono ml-1">+{stack.apps.length - 3}</span>}
                </div>
                {/* Name */}
                <div className="min-w-0">
                  <div className="text-sm font-semibold text-text-primary truncate">{stack.name}</div>
                  {stack.ctid > 0 && <div className="text-[10px] text-text-muted font-mono">CT {stack.ctid}</div>}
                </div>
                {/* Apps count */}
                <span className="text-xs font-mono text-text-muted">{stack.apps.length} app{stack.apps.length !== 1 ? 's' : ''}</span>
                {/* Status */}
                <div className="flex items-center gap-1.5">
                  <StatusDot running={isRunning} />
                  <span className={`text-xs font-mono ${isRunning ? 'text-status-running' : 'text-status-stopped'}`}>{stack.status}</span>
                </div>
                {/* Network */}
                <div className="min-w-0 text-xs font-mono text-text-secondary">
                  {stack.ip && <div className="truncate">{stack.ip}</div>}
                </div>
                {/* Resources */}
                <span className="text-xs font-mono text-text-muted">{stack.cores}c / {stack.memory_mb}MB / {stack.disk_gb}GB</span>
                {/* Boot */}
                <span className="text-xs font-mono text-text-muted">{stack.onboot ? 'On' : 'Off'}</span>
                {/* Uptime */}
                <span className="text-xs font-mono text-text-muted">
                  {isRunning && stack.uptime ? formatUptime(stack.uptime) : '-'}
                </span>
                {/* Created */}
                <span className="text-[10px] font-mono text-text-muted">
                  {new Date(stack.created_at).toLocaleDateString()}
                </span>
                {/* Actions */}
                <button
                  onClick={e => { e.stopPropagation(); setContextMenu({ stack, x: e.clientX, y: e.clientY }) }}
                  className="text-text-muted hover:text-primary bg-transparent border-none cursor-pointer text-base font-mono p-0 leading-none"
                  title="Actions">&#x22EE;</button>
              </div>
            )
          })}
        </div>
      )}

      {contextMenu && (
        <StackContextMenu
          stack={contextMenu.stack}
          x={contextMenu.x}
          y={contextMenu.y}
          onClose={() => setContextMenu(null)}
          onAction={(action, id) => { setContextMenu(null); requireAuth(() => handleAction(action, id)) }}
          onShell={id => { setContextMenu(null); requireAuth(() => setShowTerminal(id)) }}
          onLogs={id => { setContextMenu(null); setShowLogs(id) }}
        />
      )}

      {showTerminal && <StackTerminalModal stackId={showTerminal} onClose={() => setShowTerminal(null)} />}
      {showLogs && <StackLogViewerModal stackId={showLogs} onClose={() => setShowLogs(null)} />}
    </div>
  )
}

function StackContextMenu({ stack, x, y, onClose, onAction, onShell, onLogs }: {
  stack: StackListItem;
  x: number; y: number;
  onClose: () => void;
  onAction: (action: string, id: string) => void;
  onShell: (id: string) => void;
  onLogs: (id: string) => void;
}) {
  const isRunning = stack.status === 'running'
  const isStopped = stack.status === 'stopped'

  const menuY = Math.min(y, window.innerHeight - 320)
  const menuX = Math.min(x, window.innerWidth - 200)

  return (
    <>
      <div className="fixed inset-0 z-[299]" onClick={onClose} />
      <div style={{ position: 'fixed', top: menuY, left: menuX, zIndex: 300 }}
        className="bg-bg-card border border-border rounded-lg shadow-lg py-1 min-w-[170px]">
        {isStopped && <CtxMenuItem label="Start" onClick={() => onAction('start', stack.id)} />}
        {isRunning && (
          <>
            <CtxMenuItem label="Stop" onClick={() => onAction('stop', stack.id)} />
            <CtxMenuItem label="Restart" onClick={() => onAction('restart', stack.id)} />
          </>
        )}
        {(isRunning || isStopped) && <div className="border-t border-border my-1" />}
        {isRunning && (
          <>
            <CtxMenuItem label="Logs" onClick={() => onLogs(stack.id)} />
            <CtxMenuItem label="Shell" onClick={() => onShell(stack.id)} />
            <div className="border-t border-border my-1" />
          </>
        )}
        <CtxMenuItem label="Details" onClick={() => { onClose(); window.location.hash = `#/stack/${stack.id}` }} />
        <div className="border-t border-border my-1" />
        <CtxMenuItem label="Remove" danger onClick={() => onAction('uninstall', stack.id)} />
      </div>
    </>
  )
}

function StackDetailView({ id, requireAuth }: { id: string; requireAuth: (cb: () => void) => void }) {
  const [detail, setDetail] = useState<StackDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showTerminal, setShowTerminal] = useState(false)
  const [showLogs, setShowLogs] = useState(false)

  const fetchDetail = useCallback(async () => {
    try {
      const d = await api.stackDetail(id)
      setDetail(d)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load')
    }
    setLoading(false)
  }, [id])

  useEffect(() => { fetchDetail() }, [fetchDetail])
  useEffect(() => {
    const interval = setInterval(fetchDetail, 5000)
    return () => clearInterval(interval)
  }, [fetchDetail])

  const handleAction = async (action: string) => {
    try {
      switch (action) {
        case 'start': await api.startStack(id); break
        case 'stop': await api.stopStack(id); break
        case 'restart': await api.restartStack(id); break
        case 'uninstall': {
          const job = await api.uninstallStack(id)
          window.location.hash = `#/job/${job.id}`
          return
        }
      }
      setTimeout(fetchDetail, 1000)
    } catch (e) {
      alert(e instanceof Error ? e.message : `${action} failed`)
    }
  }

  if (loading) return <Center>Loading...</Center>
  if (error || !detail) return <Center className="text-status-error">{error || 'Stack not found'}</Center>

  const isRunning = detail.status === 'running'

  return (
    <div>
      <a href="#/stacks" className="text-primary text-xs font-mono no-underline hover:underline mb-4 inline-block">&larr; Back to Stacks</a>

      {/* Header */}
      <div className="flex items-start justify-between mb-6">
        <div>
          <h2 className="text-xl font-bold text-text-primary mb-1 font-mono">{detail.name}</h2>
          <div className="flex items-center gap-3 text-xs font-mono text-text-muted">
            <span>CT {detail.ctid}</span>
            {detail.ip && <span>{detail.ip}</span>}
            <span className="flex items-center gap-1"><StatusDot running={isRunning} /><span className={isRunning ? 'text-status-running' : 'text-status-stopped'}>{detail.status}</span></span>
            {isRunning && detail.live?.uptime ? <span>up {formatUptime(detail.live.uptime)}</span> : null}
          </div>
        </div>
        <div className="flex gap-2">
          {detail.status === 'stopped' && <ActionButton label="Start" onClick={() => requireAuth(() => handleAction('start'))} />}
          {isRunning && (
            <>
              <ActionButton label="Stop" onClick={() => requireAuth(() => handleAction('stop'))} />
              <ActionButton label="Restart" onClick={() => requireAuth(() => handleAction('restart'))} />
              <ActionButton label="Shell" onClick={() => requireAuth(() => setShowTerminal(true))} accent />
              <ActionButton label="Logs" onClick={() => setShowLogs(true)} />
            </>
          )}
          <ActionButton label="Remove" onClick={() => requireAuth(() => handleAction('uninstall'))} danger />
        </div>
      </div>

      {/* Resource Cards */}
      {isRunning && detail.live && (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mb-6">
          <ResourceCard label="CPU" value={`${(detail.live.cpu * 100).toFixed(1)}%`} sub={`${detail.live.cpus} cores`} />
          <ResourceCard label="Memory" value={formatBytes(detail.live.mem)} sub={`/ ${formatBytes(detail.live.maxmem)}`} />
          <ResourceCard label="Disk" value={formatBytes(detail.live.disk)} sub={`/ ${formatBytes(detail.live.maxdisk)}`} />
          <ResourceCard label="Network" value={`${formatBytesShort(detail.live.netin)} in`} sub={`${formatBytesShort(detail.live.netout)} out`} />
        </div>
      )}

      {/* Contained Apps */}
      <div className="bg-bg-card border border-border rounded-lg p-5 mb-4">
        <h3 className="text-sm font-bold text-text-primary mb-3 font-mono">Contained Apps ({detail.apps.length})</h3>
        <div className="grid grid-cols-[40px_1fr_100px_80px_1fr] gap-2 px-2 py-1 text-[10px] text-text-muted font-mono uppercase tracking-wider border-b border-border">
          <span>#</span><span>App</span><span>Version</span><span>Status</span><span>Key Outputs</span>
        </div>
        {detail.apps.map((app: StackApp, i: number) => (
          <div key={app.app_id} className="grid grid-cols-[40px_1fr_100px_80px_1fr] gap-2 px-2 py-2 border-b border-border items-center">
            <span className="text-xs font-mono text-text-muted">{i + 1}</span>
            <div className="flex items-center gap-2">
              <div className="w-6 h-6 rounded bg-bg-secondary overflow-hidden flex items-center justify-center">
                <img src={`/api/apps/${app.app_id}/icon`} alt="" className="w-5 h-5 rounded" onError={e => { (e.target as HTMLImageElement).style.display = 'none' }} />
              </div>
              <span className="text-sm text-text-primary">{app.app_name}</span>
            </div>
            <span className="text-xs font-mono text-text-muted">{app.app_version}</span>
            <span className={`text-xs font-mono ${app.status === 'completed' ? 'text-status-running' : app.status === 'failed' ? 'text-status-error' : 'text-text-muted'}`}>
              {app.status}{app.error ? ` (${app.error})` : ''}
            </span>
            <div className="text-xs font-mono text-text-muted truncate">
              {app.outputs && Object.entries(app.outputs).slice(0, 2).map(([k, v]) => (
                <span key={k} className="mr-3">{k}: {v}</span>
              ))}
            </div>
          </div>
        ))}
      </div>

      {/* All Outputs */}
      {detail.apps.some(a => a.outputs && Object.keys(a.outputs).length > 0) && (
        <div className="bg-bg-card border border-border rounded-lg p-5 mb-4">
          <h3 className="text-sm font-bold text-text-primary mb-3 font-mono">Outputs</h3>
          <div className="grid grid-cols-2 gap-2">
            {detail.apps.map(app => app.outputs && Object.entries(app.outputs).map(([k, v]) => (
              <div key={`${app.app_id}-${k}`} className="text-sm font-mono">
                <span className="text-text-muted">{app.app_id}.{k}:</span>{' '}
                {v.startsWith('http') ? <a href={v} target="_blank" rel="noreferrer" className="text-primary hover:underline">{v}</a> : <span className="text-text-primary">{v}</span>}
              </div>
            )))}
          </div>
        </div>
      )}

      {/* Mounts */}
      {detail.mount_points && detail.mount_points.length > 0 && (
        <div className="bg-bg-card border border-border rounded-lg p-5 mb-4">
          <h3 className="text-sm font-bold text-text-primary mb-3 font-mono">Mounts</h3>
          {detail.mount_points.map((mp: MountPoint) => (
            <div key={mp.index} className="flex items-center gap-3 py-1 text-sm font-mono">
              <span className="text-text-muted">mp{mp.index}</span>
              <span className="text-primary">{mp.mount_path}</span>
              <span className="text-text-muted text-xs">({mp.type}{mp.host_path ? `: ${mp.host_path}` : ''}{mp.volume_id ? `: ${mp.volume_id}` : ''})</span>
            </div>
          ))}
        </div>
      )}

      {showTerminal && <StackTerminalModal stackId={id} onClose={() => setShowTerminal(false)} />}
      {showLogs && <StackLogViewerModal stackId={id} onClose={() => setShowLogs(false)} />}
    </div>
  )
}

function ActionButton({ label, onClick, accent, danger }: { label: string; onClick: () => void; accent?: boolean; danger?: boolean }) {
  const cls = danger
    ? 'border-status-error text-status-error hover:bg-status-error hover:text-bg-primary'
    : accent
    ? 'border-primary text-primary hover:bg-primary hover:text-bg-primary'
    : 'border-border text-text-muted hover:border-primary hover:text-primary'
  return (
    <button onClick={onClick} className={`px-3 py-1.5 bg-transparent border rounded text-xs font-mono cursor-pointer transition-colors ${cls}`}>
      {label}
    </button>
  )
}


function StackTerminalModal({ stackId, onClose }: { stackId: string; onClose: () => void }) {
  const termRef = useRef<HTMLDivElement>(null)
  const termInstance = useRef<Terminal | null>(null)
  const wsRef = useRef<WebSocket | null>(null)

  useEffect(() => {
    if (!termRef.current) return

    const term = new Terminal({
      cursorBlink: true,
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 14,
      theme: { background: '#0A0A0A', foreground: '#00FF9D', cursor: '#00FF9D', selectionBackground: 'rgba(0,255,157,0.3)' },
    })
    termInstance.current = term

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.loadAddon(new WebLinksAddon())
    term.open(termRef.current)
    fitAddon.fit()

    let ws: WebSocket | null = null
    let cancelled = false

    api.terminalToken().then(({ token }) => {
      if (cancelled) return
      const wsUrl = api.stackTerminalUrl(stackId, token)
      ws = new WebSocket(wsUrl)
      wsRef.current = ws
      ws.binaryType = 'arraybuffer'

      ws.onopen = () => {
        term.writeln('\x1b[32mConnected to stack container shell.\x1b[0m\r\n')
        ws!.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
      }
      ws.onmessage = (event) => {
        if (event.data instanceof ArrayBuffer) term.write(new Uint8Array(event.data))
        else term.write(event.data)
      }
      ws.onclose = () => { term.writeln('\r\n\x1b[31mConnection closed.\x1b[0m') }
      ws.onerror = () => { term.writeln('\r\n\x1b[31mWebSocket error.\x1b[0m') }

      term.onData(data => { if (ws?.readyState === WebSocket.OPEN) ws.send(data) })
      term.onResize(size => { if (ws?.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: 'resize', cols: size.cols, rows: size.rows })) })
    }).catch(() => { term.writeln('\x1b[31mFailed to get terminal token.\x1b[0m') })

    const observer = new ResizeObserver(() => fitAddon.fit())
    observer.observe(termRef.current)

    return () => {
      cancelled = true
      observer.disconnect()
      ws?.close()
      term.dispose()
    }
  }, [stackId])

  return (
    <div className="fixed inset-0 bg-black/80 flex items-center justify-center z-[200] p-4" onClick={onClose}>
      <div className="bg-bg-card border border-border rounded-lg w-full max-w-4xl h-[70vh] flex flex-col" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-4 py-2 border-b border-border">
          <span className="text-sm font-mono text-text-primary">Stack Shell</span>
          <button onClick={onClose} className="text-text-muted hover:text-primary bg-transparent border-none cursor-pointer text-lg font-mono">&times;</button>
        </div>
        <div ref={termRef} className="flex-1 p-2" />
      </div>
    </div>
  )
}

function StackLogViewerModal({ stackId, onClose }: { stackId: string; onClose: () => void }) {
  const termRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!termRef.current) return

    const term = new Terminal({
      cursorBlink: false, disableStdin: true,
      fontFamily: "'JetBrains Mono', monospace", fontSize: 13,
      theme: { background: '#0A0A0A', foreground: '#9CA3AF', cursor: 'transparent', selectionBackground: 'rgba(0,255,157,0.3)' },
    })

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.loadAddon(new WebLinksAddon())
    term.open(termRef.current)
    fitAddon.fit()

    let ws: WebSocket | null = null
    let cancelled = false

    api.terminalToken().then(({ token }) => {
      if (cancelled) return
      const wsUrl = api.stackJournalLogsUrl(stackId, token)
      ws = new WebSocket(wsUrl)
      ws.onopen = () => { term.writeln('\x1b[32m--- Journal log stream started ---\x1b[0m\r') }
      ws.onmessage = (event) => {
        const lines = (event.data as string).split('\n')
        for (const line of lines) {
          if (line.trim()) term.writeln(line)
        }
      }
      ws.onclose = () => { term.writeln('\r\n\x1b[31m--- Stream ended ---\x1b[0m') }
      ws.onerror = () => { term.writeln('\r\n\x1b[31mWebSocket error.\x1b[0m') }
    }).catch(() => { term.writeln('\x1b[31mFailed to get terminal token.\x1b[0m') })

    const observer = new ResizeObserver(() => fitAddon.fit())
    observer.observe(termRef.current)

    return () => {
      cancelled = true
      observer.disconnect()
      ws?.close()
      term.dispose()
    }
  }, [stackId])

  return (
    <div className="fixed inset-0 bg-black/80 flex items-center justify-center z-[200] p-4" onClick={onClose}>
      <div className="bg-bg-card border border-border rounded-lg w-full max-w-4xl h-[70vh] flex flex-col" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-4 py-2 border-b border-border">
          <span className="text-sm font-mono text-text-primary">Stack Logs (journalctl)</span>
          <button onClick={onClose} className="text-text-muted hover:text-primary bg-transparent border-none cursor-pointer text-lg font-mono">&times;</button>
        </div>
        <div ref={termRef} className="flex-1 p-2" />
      </div>
    </div>
  )
}

function StackCreateWizard({ requireAuth }: { requireAuth: (cb: () => void) => void }) {
  const [step, setStep] = useState(1)
  const [name, setName] = useState('')
  const [selectedApps, setSelectedApps] = useState<{ app_id: string; name: string }[]>([])
  const [appSearch, setAppSearch] = useState('')
  const [allApps, setAllApps] = useState<AppSummary[]>([])
  const [perAppInputs, setPerAppInputs] = useState<Record<string, Record<string, string>>>({})
  const [appDetails, setAppDetails] = useState<Record<string, AppDetail>>({})
  const [cores, setCores] = useState(2)
  const [memoryMB, setMemoryMB] = useState(2048)
  const [diskGB, setDiskGB] = useState(16)
  const [storage, setStorage] = useState('')
  const [bridge, setBridge] = useState('')
  const [ipAddress, setIpAddress] = useState('')
  const [defaults, setDefaults] = useState<{ storages: string[]; bridges: string[]; defaults: { cores: number; memory_mb: number; disk_gb: number } } | null>(null)
  const [validating, setValidating] = useState(false)
  const [validation, setValidation] = useState<StackValidateResponse | null>(null)
  const [creating, setCreating] = useState(false)

  useEffect(() => {
    api.apps().then(d => setAllApps(d.apps || []))
    api.configDefaults().then(d => {
      setDefaults(d)
      if (d.storages.length > 0) setStorage(d.storages[0])
      if (d.bridges.length > 0) setBridge(d.bridges[0])
    })
  }, [])

  const addApp = (app: AppSummary) => {
    if (selectedApps.some(a => a.app_id === app.id)) return
    setSelectedApps(prev => [...prev, { app_id: app.id, name: app.name }])
    // Fetch app detail for inputs
    api.app(app.id).then(detail => {
      setAppDetails(prev => ({ ...prev, [app.id]: detail }))
    })
  }

  const removeApp = (appId: string) => {
    setSelectedApps(prev => prev.filter(a => a.app_id !== appId))
    setPerAppInputs(prev => { const next = { ...prev }; delete next[appId]; return next })
  }

  const moveApp = (index: number, direction: -1 | 1) => {
    const newIndex = index + direction
    if (newIndex < 0 || newIndex >= selectedApps.length) return
    const copy = [...selectedApps]
    const tmp = copy[index]
    copy[index] = copy[newIndex]
    copy[newIndex] = tmp
    setSelectedApps(copy)
  }

  const filteredApps = appSearch
    ? allApps.filter(a => a.name.toLowerCase().includes(appSearch.toLowerCase()) || a.id.toLowerCase().includes(appSearch.toLowerCase()))
    : allApps

  const handleValidate = async () => {
    setValidating(true)
    try {
      const result = await api.validateStack({
        name,
        apps: selectedApps.map(a => ({ app_id: a.app_id })),
      })
      setValidation(result)
      if (result.valid && result.recommended) {
        setCores(prev => prev || result.recommended!.cores)
        setMemoryMB(prev => prev || result.recommended!.memory_mb)
        setDiskGB(prev => prev || result.recommended!.disk_gb)
      }
    } catch (e) {
      setValidation({ valid: false, errors: [e instanceof Error ? e.message : 'Validation failed'], warnings: [] })
    }
    setValidating(false)
  }

  const handleCreate = async () => {
    setCreating(true)
    requireAuth(async () => {
      try {
        const req: StackCreateRequest = {
          name,
          apps: selectedApps.map(a => ({
            app_id: a.app_id,
            inputs: perAppInputs[a.app_id],
          })),
          storage,
          bridge,
          cores,
          memory_mb: memoryMB,
          disk_gb: diskGB,
          ip_address: ipAddress || undefined,
        }
        const job = await api.createStack(req)
        window.location.hash = `#/job/${job.id}`
      } catch (e) {
        alert(e instanceof Error ? e.message : 'Failed to create stack')
        setCreating(false)
      }
    })
  }

  const canProceedStep1 = name.trim() !== '' && selectedApps.length > 0

  return (
    <div className="max-w-3xl mx-auto">
      <a href="#/stacks" className="text-primary text-xs font-mono no-underline hover:underline mb-4 inline-block">&larr; Back to Stacks</a>
      <h2 className="text-xl font-bold text-text-primary mb-6 font-mono">Create Stack</h2>

      {/* Step indicator */}
      <div className="flex items-center gap-2 mb-6">
        {[1, 2, 3, 4].map(s => (
          <div key={s} className={`flex items-center gap-2 ${s <= step ? 'text-primary' : 'text-text-muted'}`}>
            <div className={`w-7 h-7 rounded-full flex items-center justify-center text-xs font-mono font-bold border ${s <= step ? 'border-primary bg-primary/10' : 'border-border'}`}>{s}</div>
            <span className="text-xs font-mono">{s === 1 ? 'Apps' : s === 2 ? 'Resources' : s === 3 ? 'Inputs' : 'Review'}</span>
            {s < 4 && <span className="text-text-muted mx-1">/</span>}
          </div>
        ))}
      </div>

      {/* Step 1: Name + App Selection */}
      {step === 1 && (
        <div className="bg-bg-card border border-border rounded-lg p-6">
          <FormField label="Stack Name">
            <FormInput value={name} onChange={setName} placeholder="my-media-stack" />
          </FormField>

          <div className="mt-4 grid grid-cols-2 gap-4">
            {/* Left: Catalog search */}
            <div>
              <label className="text-xs text-text-muted font-mono mb-2 block">Available Apps</label>
              <input type="text" value={appSearch} onChange={e => setAppSearch(e.target.value)} placeholder="Search apps..."
                className="w-full px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono mb-2 placeholder:text-text-muted" />
              <div className="max-h-[300px] overflow-y-auto space-y-1">
                {filteredApps.map(app => (
                  <div key={app.id} className={`flex items-center justify-between p-2 rounded cursor-pointer hover:bg-bg-secondary ${selectedApps.some(a => a.app_id === app.id) ? 'opacity-40' : ''}`}
                    onClick={() => addApp(app)}>
                    <div className="flex items-center gap-2">
                      <img src={`/api/apps/${app.id}/icon`} alt="" className="w-6 h-6 rounded" onError={e => { (e.target as HTMLImageElement).style.display = 'none' }} />
                      <span className="text-sm text-text-primary">{app.name}</span>
                      <span className="text-[10px] text-text-muted">v{app.version}</span>
                    </div>
                    <button className="text-primary text-sm font-bold bg-transparent border-none cursor-pointer">+</button>
                  </div>
                ))}
              </div>
            </div>

            {/* Right: Selected apps */}
            <div>
              <label className="text-xs text-text-muted font-mono mb-2 block">Selected Apps ({selectedApps.length})</label>
              <div className="space-y-1">
                {selectedApps.map((app, i) => (
                  <div key={app.app_id} className="flex items-center justify-between p-2 bg-bg-secondary rounded">
                    <div className="flex items-center gap-2">
                      <span className="text-xs text-text-muted font-mono w-5">{i + 1}.</span>
                      <img src={`/api/apps/${app.app_id}/icon`} alt="" className="w-5 h-5 rounded" onError={e => { (e.target as HTMLImageElement).style.display = 'none' }} />
                      <span className="text-sm text-text-primary">{app.name}</span>
                    </div>
                    <div className="flex items-center gap-1">
                      <button onClick={() => moveApp(i, -1)} disabled={i === 0} className="text-text-muted hover:text-primary bg-transparent border-none cursor-pointer text-xs disabled:opacity-30">&uarr;</button>
                      <button onClick={() => moveApp(i, 1)} disabled={i === selectedApps.length - 1} className="text-text-muted hover:text-primary bg-transparent border-none cursor-pointer text-xs disabled:opacity-30">&darr;</button>
                      <button onClick={() => removeApp(app.app_id)} className="text-status-error hover:text-red-400 bg-transparent border-none cursor-pointer text-sm ml-1">&times;</button>
                    </div>
                  </div>
                ))}
                {selectedApps.length === 0 && <div className="text-sm text-text-muted text-center py-8">Select apps from the left panel</div>}
              </div>
            </div>
          </div>

          {validation && !validation.valid && (
            <div className="mt-3 p-3 bg-status-error/10 border border-status-error/30 rounded text-sm text-status-error">
              {validation.errors.map((e, i) => <div key={i}>{e}</div>)}
            </div>
          )}
          {validation && validation.warnings.length > 0 && (
            <div className="mt-3 p-3 bg-status-warning/10 border border-status-warning/30 rounded text-sm text-status-warning">
              {validation.warnings.map((w, i) => <div key={i}>{w}</div>)}
            </div>
          )}

          <div className="mt-4 flex justify-end">
            <button onClick={() => { if (canProceedStep1) { handleValidate().then(() => setStep(2)) } }}
              disabled={!canProceedStep1 || validating}
              className="px-6 py-2 bg-primary text-bg-primary font-mono text-sm font-bold rounded-md cursor-pointer hover:opacity-90 transition-opacity border-none disabled:opacity-50 disabled:cursor-not-allowed">
              {validating ? 'Validating...' : 'Next'}
            </button>
          </div>
        </div>
      )}

      {/* Step 2: Resources */}
      {step === 2 && (
        <div className="bg-bg-card border border-border rounded-lg p-6">
          {validation?.recommended && (
            <div className="mb-4 p-3 bg-bg-secondary border border-border rounded text-xs text-text-muted font-mono">
              Recommended: {validation.recommended.cores} cores, {validation.recommended.memory_mb} MB RAM, {validation.recommended.disk_gb} GB disk
              {validation.ostemplate && <span> | Template: {validation.ostemplate}</span>}
            </div>
          )}

          <div className="grid grid-cols-3 gap-4 mb-4">
            <FormField label="CPU Cores">
              <FormInput value={String(cores)} onChange={v => setCores(parseInt(v) || 0)} type="number" />
            </FormField>
            <FormField label="Memory (MB)">
              <FormInput value={String(memoryMB)} onChange={v => setMemoryMB(parseInt(v) || 0)} type="number" />
            </FormField>
            <FormField label="Disk (GB)">
              <FormInput value={String(diskGB)} onChange={v => setDiskGB(parseInt(v) || 0)} type="number" />
            </FormField>
          </div>
          <div className="grid grid-cols-2 gap-4 mb-4">
            <FormField label="Storage">
              <select value={storage} onChange={e => setStorage(e.target.value)}
                className="w-full px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono">
                {defaults?.storages.map(s => <option key={s} value={s}>{s}</option>)}
              </select>
            </FormField>
            <FormField label="Bridge">
              <select value={bridge} onChange={e => setBridge(e.target.value)}
                className="w-full px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono">
                {defaults?.bridges.map(b => <option key={b} value={b}>{b}</option>)}
              </select>
            </FormField>
          </div>
          <div className="mb-4">
            <FormField label="Static IP (optional)">
              <FormInput value={ipAddress} onChange={setIpAddress} placeholder="e.g. 192.168.1.100 (blank = DHCP)" />
            </FormField>
          </div>

          <div className="flex justify-between">
            <button onClick={() => setStep(1)} className="px-4 py-2 bg-transparent border border-border text-text-muted rounded text-sm font-mono cursor-pointer hover:border-primary hover:text-primary transition-colors">Back</button>
            <button onClick={() => setStep(3)} className="px-6 py-2 bg-primary text-bg-primary font-mono text-sm font-bold rounded-md cursor-pointer hover:opacity-90 border-none">Next</button>
          </div>
        </div>
      )}

      {/* Step 3: Per-App Inputs */}
      {step === 3 && (
        <div className="bg-bg-card border border-border rounded-lg p-6">
          <h3 className="text-sm font-bold text-text-primary mb-4 font-mono">Per-App Configuration</h3>
          {selectedApps.map(app => {
            const detail = appDetails[app.app_id]
            const inputs = detail?.inputs || []
            return (
              <div key={app.app_id} className="mb-4 border border-border rounded-lg overflow-hidden">
                <div className="px-4 py-2 bg-bg-secondary flex items-center gap-2">
                  <img src={`/api/apps/${app.app_id}/icon`} alt="" className="w-5 h-5 rounded" onError={e => { (e.target as HTMLImageElement).style.display = 'none' }} />
                  <span className="text-sm font-bold text-text-primary font-mono">{app.name}</span>
                </div>
                <div className="p-4">
                  {inputs.length === 0 ? (
                    <div className="text-sm text-text-muted">(no configuration needed)</div>
                  ) : inputs.map(input => (
                    <FormField key={input.key} label={input.label} description={input.description} help={input.help}>
                      {input.type === 'select' && input.validation?.enum ? (
                        <select value={perAppInputs[app.app_id]?.[input.key] || String(input.default || '')}
                          onChange={e => setPerAppInputs(prev => ({ ...prev, [app.app_id]: { ...prev[app.app_id], [input.key]: e.target.value } }))}
                          className="w-full px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono">
                          {input.validation.enum.map(v => <option key={v} value={v}>{v}</option>)}
                        </select>
                      ) : input.type === 'boolean' ? (
                        <select value={perAppInputs[app.app_id]?.[input.key] || String(input.default || 'false')}
                          onChange={e => setPerAppInputs(prev => ({ ...prev, [app.app_id]: { ...prev[app.app_id], [input.key]: e.target.value } }))}
                          className="w-full px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono">
                          <option value="true">Yes</option>
                          <option value="false">No</option>
                        </select>
                      ) : (
                        <FormInput
                          value={perAppInputs[app.app_id]?.[input.key] || String(input.default || '')}
                          onChange={v => setPerAppInputs(prev => ({ ...prev, [app.app_id]: { ...prev[app.app_id], [input.key]: v } }))}
                          type={input.type === 'number' ? 'number' : input.type === 'secret' ? 'password' : 'text'}
                          placeholder={input.default != null ? String(input.default) : undefined}
                        />
                      )}
                    </FormField>
                  ))}
                </div>
              </div>
            )
          })}

          <div className="flex justify-between mt-4">
            <button onClick={() => setStep(2)} className="px-4 py-2 bg-transparent border border-border text-text-muted rounded text-sm font-mono cursor-pointer hover:border-primary hover:text-primary transition-colors">Back</button>
            <button onClick={() => setStep(4)} className="px-6 py-2 bg-primary text-bg-primary font-mono text-sm font-bold rounded-md cursor-pointer hover:opacity-90 border-none">Next</button>
          </div>
        </div>
      )}

      {/* Step 4: Review & Create */}
      {step === 4 && (
        <div className="bg-bg-card border border-border rounded-lg p-6">
          <h3 className="text-sm font-bold text-text-primary mb-4 font-mono">Review</h3>

          <div className="grid grid-cols-2 gap-4 text-sm font-mono mb-4">
            <div><span className="text-text-muted">Name:</span> <span className="text-text-primary">{name}</span></div>
            <div><span className="text-text-muted">Apps:</span> <span className="text-text-primary">{selectedApps.length}</span></div>
            <div><span className="text-text-muted">Resources:</span> <span className="text-text-primary">{cores}c / {memoryMB}MB / {diskGB}GB</span></div>
            <div><span className="text-text-muted">Storage:</span> <span className="text-text-primary">{storage}</span></div>
            <div><span className="text-text-muted">Bridge:</span> <span className="text-text-primary">{bridge}</span></div>
            {validation?.ostemplate && <div><span className="text-text-muted">Template:</span> <span className="text-text-primary">{validation.ostemplate}</span></div>}
          </div>

          <div className="mb-4">
            <div className="text-xs text-text-muted font-mono mb-2">Install Order:</div>
            {selectedApps.map((app, i) => (
              <div key={app.app_id} className="flex items-center gap-2 py-1 text-sm">
                <span className="text-text-muted font-mono w-5">{i + 1}.</span>
                <img src={`/api/apps/${app.app_id}/icon`} alt="" className="w-5 h-5 rounded" onError={e => { (e.target as HTMLImageElement).style.display = 'none' }} />
                <span className="text-text-primary">{app.name}</span>
              </div>
            ))}
          </div>

          <div className="flex justify-between">
            <button onClick={() => setStep(3)} className="px-4 py-2 bg-transparent border border-border text-text-muted rounded text-sm font-mono cursor-pointer hover:border-primary hover:text-primary transition-colors">Back</button>
            <button onClick={handleCreate} disabled={creating}
              className="px-6 py-2 bg-primary text-bg-primary font-mono text-sm font-bold rounded-md cursor-pointer hover:opacity-90 border-none disabled:opacity-50">
              {creating ? 'Creating...' : 'Create Stack'}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

// --- Helpers ---

function formatUptime(seconds: number): string {
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`
}

function formatBytesShort(bytes: number): string {
  if (bytes === 0) return '0B'
  const units = ['B', 'K', 'M', 'G', 'T']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(1)}${units[i]}`
}

export default App
