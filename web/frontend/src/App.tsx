import { useState, useEffect, useCallback, useRef } from 'react'
import { api } from './api'
import type { AppSummary, AppDetail, AppInput, HealthResponse, Job, LogEntry, Install, ConfigDefaultsResponse } from './types'

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
  const isInstalls = hash === '#/installs'
  const isJobs = hash === '#/jobs'

  let content
  if (jobMatch) content = <JobView id={jobMatch[1]} />
  else if (appMatch) content = <AppDetailView id={appMatch[1]} requireAuth={requireAuth} />
  else if (isInstalls) content = <InstallsList requireAuth={requireAuth} />
  else if (isJobs) content = <JobsList />
  else content = <AppList />

  return (
    <div style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column' }}>
      <Header health={health} authed={authed} authRequired={authRequired} onLogout={handleLogout} onLogin={() => setShowLogin(true)} />
      <main style={{ flex: 1, maxWidth: 1200, margin: '0 auto', padding: '24px 16px', width: '100%' }}>
        {content}
      </main>
      {showLogin && <LoginModal onSuccess={handleLoginSuccess} onClose={() => { setShowLogin(false); setLoginCallback(null) }} />}
    </div>
  )
}

function Header({ health, authed, authRequired, onLogout, onLogin }: { health: HealthResponse | null; authed: boolean; authRequired: boolean; onLogout: () => void; onLogin: () => void }) {
  return (
    <header style={{ background: '#16213e', borderBottom: '1px solid #2a2a4a', padding: '12px 24px', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 24 }}>
        <a href="#/" style={{ textDecoration: 'none', color: 'inherit', display: 'flex', alignItems: 'center', gap: 12 }}>
          <span style={{ fontSize: 24 }}>&#9881;</span>
          <span style={{ fontSize: 18, fontWeight: 700, color: '#fff' }}>PVE App Store</span>
        </a>
        <nav style={{ display: 'flex', gap: 16 }}>
          <a href="#/" style={navStyle}>Apps</a>
          <a href="#/installs" style={navStyle}>Installed</a>
          <a href="#/jobs" style={navStyle}>Jobs</a>
        </nav>
      </div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 16, fontSize: 13, color: '#888' }}>
        {health && <>
          <span>Node: {health.node}</span>
          <span>v{health.version}</span>
        </>}
        {authRequired && (authed ? (
          <button onClick={onLogout} style={{ background: 'none', border: '1px solid #2a2a4a', borderRadius: 6, color: '#888', fontSize: 12, padding: '4px 10px', cursor: 'pointer' }}>Logout</button>
        ) : (
          <button onClick={onLogin} style={{ background: 'none', border: '1px solid #e94560', borderRadius: 6, color: '#e94560', fontSize: 12, padding: '4px 10px', cursor: 'pointer' }}>Login</button>
        ))}
      </div>
    </header>
  )
}

const navStyle: React.CSSProperties = { color: '#7ec8e3', textDecoration: 'none', fontSize: 14 }

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
      <div style={{ display: 'flex', gap: 12, marginBottom: 24, flexWrap: 'wrap' }}>
        <input type="text" placeholder="Search apps..." value={search} onChange={e => setSearch(e.target.value)}
          style={{ flex: 1, minWidth: 200, padding: '10px 16px', background: '#0f3460', border: '1px solid #2a2a4a', borderRadius: 8, color: '#fff', fontSize: 14, outline: 'none' }} />
        <select value={category} onChange={e => setCategory(e.target.value)}
          style={{ padding: '10px 16px', background: '#0f3460', border: '1px solid #2a2a4a', borderRadius: 8, color: '#fff', fontSize: 14, outline: 'none' }}>
          <option value="">All Categories</option>
          {categories.map(c => <option key={c} value={c}>{c}</option>)}
        </select>
      </div>
      {loading ? <Center>Loading...</Center> : apps.length === 0 ? <Center>No apps found</Center> : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))', gap: 16 }}>
          {apps.map(app => <AppCard key={app.id} app={app} />)}
        </div>
      )}
    </div>
  )
}

function AppCard({ app }: { app: AppSummary }) {
  return (
    <a href={`#/app/${app.id}`} style={{ display: 'block', background: '#16213e', border: '1px solid #2a2a4a', borderRadius: 12, padding: 20, textDecoration: 'none', color: 'inherit', transition: 'border-color 0.2s, transform 0.2s' }}
      onMouseEnter={e => { e.currentTarget.style.borderColor = '#e94560'; e.currentTarget.style.transform = 'translateY(-2px)' }}
      onMouseLeave={e => { e.currentTarget.style.borderColor = '#2a2a4a'; e.currentTarget.style.transform = 'none' }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', gap: 16 }}>
        <div style={{ width: 48, height: 48, borderRadius: 10, background: '#0f3460', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 22, flexShrink: 0 }}>
          {app.has_icon ? <img src={`/api/apps/${app.id}/icon`} alt="" style={{ width: 40, height: 40, borderRadius: 8 }} /> : app.name[0]}
        </div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <h3 style={{ fontSize: 16, fontWeight: 600, color: '#fff' }}>{app.name}</h3>
            <span style={{ fontSize: 12, color: '#888' }}>v{app.version}</span>
          </div>
          <p style={{ fontSize: 13, color: '#aaa', marginTop: 4, overflow: 'hidden', textOverflow: 'ellipsis', display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical' }}>{app.description}</p>
          <div style={{ display: 'flex', gap: 6, marginTop: 8, flexWrap: 'wrap' }}>
            {app.categories.map(c => <Badge key={c} bg="#0f3460" color="#7ec8e3">{c}</Badge>)}
            {app.gpu_support && app.gpu_support.length > 0 && <Badge bg="#1a3a1a" color="#4ade80">GPU {app.gpu_required ? 'Required' : 'Optional'}</Badge>}
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

  useEffect(() => {
    setApp(null); setError('')
    api.app(id).then(setApp).catch(e => setError(e.message))
    api.appReadme(id).then(setReadme)
  }, [id])

  if (error) return <div><BackLink /><Center color="#e94560">{error}</Center></div>
  if (!app) return <Center>Loading...</Center>

  const inputGroups = app.inputs && app.inputs.length > 0 ? groupInputs(app.inputs) : null

  return (
    <div>
      <BackLink />
      <div style={{ marginTop: 16, display: 'flex', alignItems: 'flex-start', gap: 20 }}>
        <div style={{ width: 64, height: 64, borderRadius: 14, background: '#0f3460', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 28, flexShrink: 0 }}>
          {app.icon_path ? <img src={`/api/apps/${app.id}/icon`} alt="" style={{ width: 56, height: 56, borderRadius: 10 }} /> : app.name[0]}
        </div>
        <div style={{ flex: 1 }}>
          <h1 style={{ fontSize: 24, fontWeight: 700, color: '#fff' }}>{app.name}</h1>
          <p style={{ fontSize: 14, color: '#aaa', marginTop: 4 }}>{app.description}</p>
          <div style={{ display: 'flex', gap: 12, marginTop: 8, fontSize: 13, color: '#888', alignItems: 'center' }}>
            <span>v{app.version}</span>
            {app.license && <span>{app.license}</span>}
            {app.homepage && <a href={app.homepage} target="_blank" rel="noreferrer" style={{ color: '#7ec8e3' }}>Homepage</a>}
          </div>
        </div>
        <button onClick={() => requireAuth(() => setShowInstall(true))} style={{ ...btnStyle, background: '#e94560', color: '#fff' }}>Install</button>
      </div>

      {app.overview && (
        <div style={{ marginTop: 20, background: '#16213e', border: '1px solid #2a2a4a', borderRadius: 12, padding: 24 }}>
          <h3 style={{ fontSize: 14, fontWeight: 600, color: '#fff', marginBottom: 12, textTransform: 'uppercase', letterSpacing: '0.5px' }}>Overview</h3>
          {app.overview.split('\n\n').map((p, i) => (
            <p key={i} style={{ fontSize: 14, color: '#ccc', lineHeight: 1.7, marginTop: i > 0 ? 12 : 0 }}>{p}</p>
          ))}
        </div>
      )}

      {showInstall && <InstallWizard app={app} onClose={() => setShowInstall(false)} />}

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: 16, marginTop: 24 }}>
        <InfoCard title="Default Resources">
          <InfoRow label="OS Template" value={app.lxc.ostemplate} />
          <InfoRow label="CPU Cores" value={String(app.lxc.defaults.cores)} />
          <InfoRow label="Memory" value={`${app.lxc.defaults.memory_mb} MB`} />
          <InfoRow label="Disk" value={`${app.lxc.defaults.disk_gb} GB`} />
          <InfoRow label="Unprivileged" value={app.lxc.defaults.unprivileged ? 'Yes' : 'No'} />
          {app.lxc.defaults.features && app.lxc.defaults.features.length > 0 && <InfoRow label="Features" value={app.lxc.defaults.features.join(', ')} />}
        </InfoCard>
        {app.gpu.supported && app.gpu.supported.length > 0 && (
          <InfoCard title="GPU Support">
            <InfoRow label="Required" value={app.gpu.required ? 'Yes' : 'No'} />
            <InfoRow label="Supported" value={app.gpu.supported.join(', ')} />
            {app.gpu.profiles && <InfoRow label="Profiles" value={app.gpu.profiles.join(', ')} />}
            {app.gpu.notes && <p style={{ fontSize: 13, color: '#aaa', marginTop: 8 }}>{app.gpu.notes}</p>}
          </InfoCard>
        )}
        {app.outputs && app.outputs.length > 0 && (
          <InfoCard title="Outputs">
            {app.outputs.map(out => <InfoRow key={out.key} label={out.label} value={out.value} />)}
          </InfoCard>
        )}
      </div>

      {inputGroups && (
        <div style={{ marginTop: 24 }}>
          <h3 style={{ fontSize: 14, fontWeight: 600, color: '#fff', marginBottom: 16, textTransform: 'uppercase', letterSpacing: '0.5px' }}>Configuration Reference</h3>
          {Object.entries(inputGroups).map(([group, inputs]) => (
            <div key={group} style={{ background: '#16213e', border: '1px solid #2a2a4a', borderRadius: 12, padding: 20, marginBottom: 12 }}>
              <h4 style={{ fontSize: 13, color: '#7ec8e3', textTransform: 'uppercase', marginBottom: 12, letterSpacing: '0.5px' }}>{group}</h4>
              {inputs.map(inp => (
                <div key={inp.key} style={{ padding: '10px 0', borderBottom: '1px solid #2a2a4a' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <span style={{ fontSize: 14, color: '#fff', fontWeight: 500 }}>{inp.label}</span>
                      <Badge bg="#0f3460" color="#7ec8e3">{inp.type}</Badge>
                      {inp.required && <Badge bg="#e9456022" color="#e94560">required</Badge>}
                    </div>
                    {inp.default !== undefined && (
                      <span style={{ fontSize: 13, color: '#888' }}>Default: <span style={{ color: '#ddd' }}>{String(inp.default)}</span></span>
                    )}
                  </div>
                  {inp.description && <p style={{ fontSize: 13, color: '#aaa', marginTop: 6, lineHeight: 1.5 }}>{inp.description}</p>}
                  {inp.help && <p style={{ fontSize: 12, color: '#666', marginTop: 4, fontStyle: 'italic' }}>{inp.help}</p>}
                </div>
              ))}
            </div>
          ))}
        </div>
      )}

      {readme && (
        <div style={{ marginTop: 24, background: '#16213e', border: '1px solid #2a2a4a', borderRadius: 12, padding: 24 }}>
          <h3 style={{ fontSize: 16, fontWeight: 600, color: '#fff', marginBottom: 12 }}>README</h3>
          <pre style={{ fontSize: 13, color: '#ccc', whiteSpace: 'pre-wrap', lineHeight: 1.6 }}>{readme}</pre>
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
  const [installing, setInstalling] = useState(false)
  const [error, setError] = useState('')
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [defaults, setDefaults] = useState<ConfigDefaultsResponse | null>(null)

  useEffect(() => {
    api.configDefaults().then(d => {
      setDefaults(d)
      setStorage(d.storages[0] || '')
      setBridge(d.bridges[0] || '')
    }).catch(() => {})
  }, [])

  const handleInstall = async () => {
    setInstalling(true); setError('')
    try {
      const job = await api.installApp(app.id, {
        cores: parseInt(cores) || undefined,
        memory_mb: parseInt(memory) || undefined,
        disk_gb: parseInt(disk) || undefined,
        storage: storage || undefined,
        bridge: bridge || undefined,
        inputs,
      })
      window.location.hash = `#/job/${job.id}`
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Install failed')
      setInstalling(false)
    }
  }

  const inputGroups = app.inputs && app.inputs.length > 0 ? groupInputs(app.inputs) : null

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.7)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 100 }}>
      <div style={{ background: '#1a1a2e', border: '1px solid #2a2a4a', borderRadius: 16, padding: 32, width: '100%', maxWidth: 560, maxHeight: '90vh', overflow: 'auto' }}>
        <h2 style={{ fontSize: 20, fontWeight: 700, color: '#fff', marginBottom: 20 }}>Install {app.name}</h2>

        <h4 style={sectionTitle}>Resources</h4>
        <FormRow label="CPU Cores"><FormInput value={cores} onChange={setCores} type="number" /></FormRow>
        <FormRow label="Memory (MB)"><FormInput value={memory} onChange={setMemory} type="number" /></FormRow>
        <FormRow label="Disk (GB)"><FormInput value={disk} onChange={setDisk} type="number" /></FormRow>

        <h4 style={sectionTitle}>Networking & Storage</h4>
        <FormRow label="Storage Pool">
          {defaults && defaults.storages.length > 1 ? (
            <select value={storage} onChange={e => setStorage(e.target.value)} style={inputStyle}>
              {defaults.storages.map(s => <option key={s} value={s}>{s}</option>)}
            </select>
          ) : (
            <span style={readonlyStyle}>{storage}</span>
          )}
        </FormRow>
        <FormRow label="Network Bridge">
          {defaults && defaults.bridges.length > 1 ? (
            <select value={bridge} onChange={e => setBridge(e.target.value)} style={inputStyle}>
              {defaults.bridges.map(b => <option key={b} value={b}>{b}</option>)}
            </select>
          ) : (
            <span style={readonlyStyle}>{bridge}</span>
          )}
        </FormRow>

        {inputGroups && Object.entries(inputGroups).map(([group, groupInps]) => (
          <div key={group}>
            <h4 style={sectionTitle}>{group}</h4>
            {groupInps.map(inp => (
              <FormRow key={inp.key} label={inp.label} help={inp.help} description={inp.description}>
                {inp.type === 'select' && inp.validation?.enum ? (
                  <select value={inputs[inp.key] || ''} onChange={e => setInputs(p => ({ ...p, [inp.key]: e.target.value }))} style={inputStyle}>
                    {inp.validation.enum.map(v => <option key={v} value={v}>{v}</option>)}
                  </select>
                ) : inp.type === 'boolean' ? (
                  <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer' }}>
                    <input type="checkbox" checked={inputs[inp.key] === 'true'} onChange={e => setInputs(p => ({ ...p, [inp.key]: e.target.checked ? 'true' : 'false' }))}
                      style={{ width: 18, height: 18, accentColor: '#e94560' }} />
                    <span style={{ fontSize: 13, color: '#aaa' }}>Enable</span>
                  </label>
                ) : (
                  <FormInput value={inputs[inp.key] || ''} onChange={v => setInputs(p => ({ ...p, [inp.key]: v }))}
                    type={inp.type === 'secret' ? 'password' : inp.type === 'number' ? 'number' : 'text'} />
                )}
              </FormRow>
            ))}
          </div>
        ))}

        <div style={{ marginTop: 20 }}>
          <button onClick={() => setShowAdvanced(!showAdvanced)} style={{ background: 'none', border: 'none', color: '#7ec8e3', fontSize: 13, cursor: 'pointer', padding: 0 }}>
            {showAdvanced ? 'Hide' : 'Show'} Advanced Settings
          </button>
          {showAdvanced && (
            <div style={{ marginTop: 12, background: '#0f3460', borderRadius: 8, padding: 16 }}>
              <InfoRow label="OS Template" value={app.lxc.ostemplate} />
              <InfoRow label="Unprivileged" value={app.lxc.defaults.unprivileged ? 'Yes' : 'No'} />
              {app.lxc.defaults.features && app.lxc.defaults.features.length > 0 && <InfoRow label="Features" value={app.lxc.defaults.features.join(', ')} />}
              <InfoRow label="Start on Boot" value={app.lxc.defaults.onboot ? 'Yes' : 'No'} />
            </div>
          )}
        </div>

        {error && <div style={{ color: '#e94560', fontSize: 13, marginTop: 12 }}>{error}</div>}

        <div style={{ display: 'flex', gap: 12, marginTop: 24, justifyContent: 'flex-end' }}>
          <button onClick={onClose} style={{ ...btnStyle, background: '#333' }}>Cancel</button>
          <button onClick={handleInstall} disabled={installing} style={{ ...btnStyle, background: '#e94560', color: '#fff', opacity: installing ? 0.6 : 1 }}>
            {installing ? 'Installing...' : 'Install'}
          </button>
        </div>
      </div>
    </div>
  )
}

// --- Job View ---

function JobView({ id }: { id: string }) {
  const [job, setJob] = useState<Job | null>(null)
  const [logs, setLogs] = useState<LogEntry[]>([])
  const lastLogId = useRef(0)
  const logEndRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    api.job(id).then(setJob).catch(() => {})
    api.jobLogs(id).then(d => { setLogs(d.logs || []); lastLogId.current = d.last_id })
  }, [id])

  // Poll for updates while job is active
  useEffect(() => {
    if (!job || job.state === 'completed' || job.state === 'failed') return
    const interval = setInterval(async () => {
      try {
        const [j, l] = await Promise.all([api.job(id), api.jobLogs(id, lastLogId.current)])
        setJob(j)
        if (l.logs && l.logs.length > 0) {
          setLogs(prev => [...prev, ...l.logs])
          lastLogId.current = l.last_id
        }
      } catch { /* ignore */ }
    }, 1500)
    return () => clearInterval(interval)
  }, [id, job?.state])

  useEffect(() => { logEndRef.current?.scrollIntoView({ behavior: 'smooth' }) }, [logs])

  if (!job) return <Center>Loading...</Center>

  const done = job.state === 'completed' || job.state === 'failed'
  const stateColor = job.state === 'completed' ? '#4ade80' : job.state === 'failed' ? '#e94560' : '#f59e0b'

  return (
    <div>
      <BackLink />
      <div style={{ marginTop: 16, display: 'flex', alignItems: 'center', gap: 16 }}>
        <h2 style={{ fontSize: 20, fontWeight: 700, color: '#fff' }}>{job.type === 'install' ? 'Installing' : 'Uninstalling'} {job.app_name}</h2>
        <span style={{ fontSize: 13, padding: '3px 10px', borderRadius: 6, background: stateColor + '22', color: stateColor, fontWeight: 600 }}>{job.state}</span>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 12, marginTop: 16 }}>
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

      {job.error && <div style={{ marginTop: 16, padding: 16, background: '#2a1a1a', border: '1px solid #e94560', borderRadius: 8, color: '#e94560', fontSize: 13 }}>{job.error}</div>}

      <div style={{ marginTop: 20, background: '#0d1117', border: '1px solid #2a2a4a', borderRadius: 12, padding: 16, maxHeight: 400, overflow: 'auto' }}>
        <h4 style={{ fontSize: 13, color: '#888', marginBottom: 8, textTransform: 'uppercase' }}>Logs</h4>
        {logs.length === 0 ? <div style={{ color: '#555', fontSize: 13 }}>Waiting for logs...</div> : logs.map((l, i) => (
          <div key={i} style={{ fontSize: 12, fontFamily: 'monospace', padding: '2px 0', color: l.level === 'error' ? '#e94560' : l.level === 'warn' ? '#f59e0b' : '#aaa' }}>
            <span style={{ color: '#555' }}>{new Date(l.timestamp).toLocaleTimeString()} </span>
            {l.message}
          </div>
        ))}
        <div ref={logEndRef} />
        {!done && <div style={{ color: '#f59e0b', fontSize: 12, marginTop: 8, fontFamily: 'monospace' }}>&#9679; Running...</div>}
      </div>
    </div>
  )
}

// --- Installs List ---

function InstallsList({ requireAuth }: { requireAuth: (cb: () => void) => void }) {
  const [installs, setInstalls] = useState<Install[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => { api.installs().then(d => { setInstalls(d.installs || []); setLoading(false) }) }, [])

  return (
    <div>
      <h2 style={{ fontSize: 20, fontWeight: 700, color: '#fff', marginBottom: 20 }}>Installed Apps</h2>
      {loading ? <Center>Loading...</Center> : installs.length === 0 ? <Center>No apps installed</Center> : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          {installs.map(inst => (
            <div key={inst.id} style={{ background: '#16213e', border: '1px solid #2a2a4a', borderRadius: 12, padding: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <div>
                <h3 style={{ fontSize: 16, fontWeight: 600, color: '#fff' }}>{inst.app_name}</h3>
                <div style={{ fontSize: 13, color: '#888', marginTop: 4 }}>CTID: {inst.ctid} | Pool: {inst.pool} | Status: <span style={{ color: inst.status === 'running' ? '#4ade80' : '#f59e0b' }}>{inst.status}</span></div>
              </div>
              <button onClick={() => requireAuth(async () => { if (confirm(`Uninstall ${inst.app_name}? This will destroy container ${inst.ctid}.`)) { const j = await api.uninstall(inst.id); window.location.hash = `#/job/${j.id}` } })}
                style={{ ...btnStyle, background: '#e9456033', color: '#e94560', fontSize: 13 }}>Uninstall</button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// --- Jobs List ---

function JobsList() {
  const [jobs, setJobs] = useState<Job[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => { api.jobs().then(d => { setJobs(d.jobs || []); setLoading(false) }) }, [])

  const stateColor = (s: string) => s === 'completed' ? '#4ade80' : s === 'failed' ? '#e94560' : '#f59e0b'

  return (
    <div>
      <h2 style={{ fontSize: 20, fontWeight: 700, color: '#fff', marginBottom: 20 }}>Jobs</h2>
      {loading ? <Center>Loading...</Center> : jobs.length === 0 ? <Center>No jobs yet</Center> : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          {jobs.map(j => (
            <a key={j.id} href={`#/job/${j.id}`} style={{ background: '#16213e', border: '1px solid #2a2a4a', borderRadius: 10, padding: 14, textDecoration: 'none', color: 'inherit', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <div>
                <span style={{ color: '#fff', fontWeight: 600 }}>{j.app_name}</span>
                <span style={{ color: '#888', fontSize: 13, marginLeft: 8 }}>{j.type}</span>
                {j.ctid > 0 && <span style={{ color: '#666', fontSize: 13, marginLeft: 8 }}>CT {j.ctid}</span>}
              </div>
              <span style={{ fontSize: 12, padding: '2px 8px', borderRadius: 4, background: stateColor(j.state) + '22', color: stateColor(j.state) }}>{j.state}</span>
            </a>
          ))}
        </div>
      )}
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
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.7)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 200 }}>
      <form onSubmit={handleSubmit} style={{ background: '#1a1a2e', border: '1px solid #2a2a4a', borderRadius: 16, padding: 32, width: '100%', maxWidth: 380 }}>
        <h2 style={{ fontSize: 18, fontWeight: 700, color: '#fff', marginBottom: 8 }}>Login Required</h2>
        <p style={{ fontSize: 13, color: '#888', marginBottom: 20 }}>Enter your password to perform this action.</p>
        <FormInput value={password} onChange={setPassword} type="password" />
        {error && <div style={{ color: '#e94560', fontSize: 13, marginTop: 8 }}>{error}</div>}
        <div style={{ display: 'flex', gap: 12, marginTop: 20, justifyContent: 'flex-end' }}>
          <button type="button" onClick={onClose} style={{ ...btnStyle, background: '#333' }}>Cancel</button>
          <button type="submit" disabled={loading || !password} style={{ ...btnStyle, background: '#e94560', color: '#fff', opacity: loading || !password ? 0.6 : 1 }}>
            {loading ? 'Logging in...' : 'Login'}
          </button>
        </div>
      </form>
    </div>
  )
}

// --- Shared Components ---

function Center({ children, color }: { children: React.ReactNode; color?: string }) {
  return <div style={{ textAlign: 'center', padding: 48, color: color || '#666' }}>{children}</div>
}

function BackLink() {
  return <a href="#/" style={{ color: '#7ec8e3', fontSize: 14, textDecoration: 'none' }}>Back to apps</a>
}

function Badge({ children, bg, color }: { children: React.ReactNode; bg: string; color: string }) {
  return <span style={{ fontSize: 11, padding: '2px 8px', borderRadius: 4, background: bg, color }}>{children}</span>
}

function InfoCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={{ background: '#16213e', border: '1px solid #2a2a4a', borderRadius: 12, padding: 20 }}>
      <h3 style={{ fontSize: 14, fontWeight: 600, color: '#fff', marginBottom: 12, textTransform: 'uppercase', letterSpacing: '0.5px' }}>{title}</h3>
      {children}
    </div>
  )
}

function Linkify({ text }: { text: string }) {
  const urlRegex = /(https?:\/\/[^\s<>"']+)/g
  const parts = text.split(urlRegex)
  if (parts.length === 1) return <>{text}</>
  return <>{parts.map((part, i) => urlRegex.test(part)
    ? <a key={i} href={part} target="_blank" rel="noreferrer" style={{ color: '#7ec8e3', textDecoration: 'underline' }}>{part}</a>
    : part
  )}</>
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div style={{ display: 'flex', justifyContent: 'space-between', padding: '4px 0', fontSize: 13 }}>
      <span style={{ color: '#888' }}>{label}</span>
      <span style={{ color: '#ddd', textAlign: 'right', wordBreak: 'break-all' }}><Linkify text={value} /></span>
    </div>
  )
}

function FormRow({ label, help, description, children }: { label: string; help?: string; description?: string; children: React.ReactNode }) {
  return (
    <div style={{ marginBottom: 12 }}>
      <label style={{ fontSize: 13, color: '#aaa', display: 'block', marginBottom: 4 }}>{label}</label>
      {description && <div style={{ fontSize: 12, color: '#888', marginBottom: 6, lineHeight: 1.4 }}>{description}</div>}
      {children}
      {help && <div style={{ fontSize: 11, color: '#666', marginTop: 2, fontStyle: 'italic' }}>{help}</div>}
    </div>
  )
}

function FormInput({ value, onChange, type = 'text' }: { value: string; onChange: (v: string) => void; type?: string }) {
  return <input type={type} value={value} onChange={e => onChange(e.target.value)} style={inputStyle} />
}

const inputStyle: React.CSSProperties = { width: '100%', padding: '8px 12px', background: '#0f3460', border: '1px solid #2a2a4a', borderRadius: 6, color: '#fff', fontSize: 14, outline: 'none' }
const readonlyStyle: React.CSSProperties = { display: 'block', padding: '8px 12px', background: '#0a2240', border: '1px solid #2a2a4a', borderRadius: 6, color: '#aaa', fontSize: 14 }
const btnStyle: React.CSSProperties = { padding: '10px 24px', fontSize: 14, fontWeight: 600, border: 'none', borderRadius: 8, cursor: 'pointer', color: '#ddd' }
const sectionTitle: React.CSSProperties = { fontSize: 13, color: '#7ec8e3', textTransform: 'uppercase', marginTop: 20, marginBottom: 8, letterSpacing: '0.5px' }

export default App
