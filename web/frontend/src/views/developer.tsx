import { useState, useEffect, useCallback, useRef } from 'react'
import { api } from '../api'
import type { DevAppMeta, DevTemplate, DockerfileChainEvent, DevStackMeta } from '../types'
import { Center, DevStatusBadge } from '../components/ui'

function DeveloperDashboard({ requireAuth }: { requireAuth: (cb: () => void) => void }) {
  const [apps, setApps] = useState<DevAppMeta[]>([])
  const [devStacks, setDevStacks] = useState<DevStackMeta[]>([])
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [showCreateStack, setShowCreateStack] = useState(false)
  const [showUnraid, setShowUnraid] = useState(false)
  const [showDockerfile, setShowDockerfile] = useState(false)

  const fetchAll = useCallback(async () => {
    try {
      const [appData, stackData] = await Promise.all([api.devApps(), api.devStacks()])
      setApps(appData.apps || [])
      setDevStacks(stackData.stacks || [])
    } catch { setApps([]); setDevStacks([]) }
    setLoading(false)
  }, [])

  useEffect(() => { fetchAll() }, [fetchAll])

  const handleDelete = (id: string, name: string) => {
    requireAuth(async () => {
      if (!confirm(`Delete dev app "${name}"? This cannot be undone.`)) return
      try {
        await api.devDeleteApp(id)
        fetchAll()
      } catch (e: unknown) { alert(e instanceof Error ? e.message : 'Failed') }
    })
  }

  const handleDeleteStack = (id: string, name: string) => {
    requireAuth(async () => {
      if (!confirm(`Delete dev stack "${name}"? This cannot be undone.`)) return
      try {
        await api.devDeleteStack(id)
        fetchAll()
      } catch (e: unknown) { alert(e instanceof Error ? e.message : 'Failed') }
    })
  }

  const handleImportZip = () => {
    requireAuth(() => {
      const input = document.createElement('input')
      input.type = 'file'
      input.accept = '.zip'
      input.onchange = async () => {
        const file = input.files?.[0]
        if (!file) return
        try {
          const result = await api.devImportZip(file)
          if (result.type === 'stack') {
            window.location.hash = `#/dev/stack/${result.id}`
          } else {
            window.location.hash = `#/dev/${result.id}`
          }
        } catch (e: unknown) { alert(e instanceof Error ? e.message : 'Import failed') }
      }
      input.click()
    })
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-xl font-bold text-text-primary font-mono">Developer Dashboard</h2>
        <div className="flex gap-2">
          <button onClick={handleImportZip} className="bg-transparent border border-border rounded px-4 py-2 text-sm font-mono text-text-secondary hover:border-primary hover:text-primary cursor-pointer transition-colors">Import ZIP</button>
          <button onClick={() => setShowDockerfile(true)} className="bg-transparent border border-border rounded px-4 py-2 text-sm font-mono text-text-secondary hover:border-primary hover:text-primary cursor-pointer transition-colors">Import Dockerfile</button>
          <button onClick={() => setShowUnraid(true)} className="bg-transparent border border-border rounded px-4 py-2 text-sm font-mono text-text-secondary hover:border-primary hover:text-primary cursor-pointer transition-colors">Import Unraid XML</button>
          <button onClick={() => setShowCreateStack(true)} className="bg-transparent border border-border rounded px-4 py-2 text-sm font-mono text-text-secondary hover:border-primary hover:text-primary cursor-pointer transition-colors">+ New Stack</button>
          <button onClick={() => setShowCreate(true)} className="bg-primary text-bg-primary rounded px-4 py-2 text-sm font-mono font-bold cursor-pointer hover:opacity-90 transition-opacity">+ New App</button>
        </div>
      </div>

      {loading ? (
        <Center className="py-16"><span className="text-text-muted font-mono">Loading...</span></Center>
      ) : apps.length === 0 && devStacks.length === 0 ? (
        <div className="border border-dashed border-border rounded-lg p-12 text-center">
          <p className="text-text-muted font-mono mb-4">No dev apps or stacks yet. Create your first one to get started.</p>
          <div className="flex gap-2 justify-center">
            <button onClick={() => setShowCreate(true)} className="bg-primary text-bg-primary rounded px-6 py-2 text-sm font-mono font-bold cursor-pointer hover:opacity-90">Create App</button>
            <button onClick={() => setShowCreateStack(true)} className="bg-transparent border border-primary rounded px-6 py-2 text-sm font-mono font-bold text-primary cursor-pointer hover:opacity-90">Create Stack</button>
          </div>
        </div>
      ) : (
        <>
          {/* Dev Apps */}
          {apps.length > 0 && (
            <>
              <h3 className="text-sm font-bold text-text-muted font-mono uppercase tracking-wider mb-3">Apps ({apps.length})</h3>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 mb-8">
                {apps.map(app => (
                  <a key={app.id} href={`#/dev/${app.id}`} className="no-underline">
                    <div className="border border-border rounded-lg p-4 hover:border-primary/50 transition-colors cursor-pointer bg-bg-card">
                      <div className="flex items-start gap-3 mb-2">
                        <img src={api.devIconUrl(app.id)} alt="" className="w-10 h-10 rounded-lg flex-shrink-0" onError={e => { (e.target as HTMLImageElement).style.display = 'none' }} />
                        <div className="flex-1 min-w-0 flex items-start justify-between">
                          <div className="min-w-0">
                            <h3 className="text-sm font-bold text-text-primary font-mono truncate">{app.name || app.id}</h3>
                            <p className="text-xs text-text-muted font-mono mt-0.5">v{app.version || '0.0.0'}</p>
                          </div>
                          <DevStatusBadge status={app.status} />
                        </div>
                      </div>
                      <p className="text-xs text-text-secondary line-clamp-2 mb-3">{app.description || 'No description'}</p>
                      <div className="flex items-center gap-2 text-xs text-text-muted font-mono">
                        {app.has_readme && <span title="Has README">readme</span>}
                        <span className="ml-auto" onClick={(e) => { e.preventDefault(); handleDelete(app.id, app.name) }}>delete</span>
                      </div>
                    </div>
                  </a>
                ))}
              </div>
            </>
          )}

          {/* Dev Stacks */}
          {devStacks.length > 0 && (
            <>
              <h3 className="text-sm font-bold text-text-muted font-mono uppercase tracking-wider mb-3">Stacks ({devStacks.length})</h3>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {devStacks.map(stack => (
                  <a key={stack.id} href={`#/dev/stack/${stack.id}`} className="no-underline">
                    <div className="border border-border rounded-lg p-4 hover:border-primary/50 transition-colors cursor-pointer bg-bg-card">
                      <div className="flex items-start gap-3 mb-2">
                        <img src={api.devStackIconUrl(stack.id)} alt="" className="w-10 h-10 rounded-lg flex-shrink-0" onError={e => { (e.target as HTMLImageElement).style.display = 'none' }} />
                        <div className="flex-1 min-w-0 flex items-start justify-between">
                          <div className="min-w-0">
                            <h3 className="text-sm font-bold text-text-primary font-mono truncate">{stack.name || stack.id}</h3>
                            <p className="text-xs text-text-muted font-mono mt-0.5">v{stack.version || '0.0.0'} &middot; {stack.app_count} app{stack.app_count !== 1 ? 's' : ''}</p>
                          </div>
                          <DevStatusBadge status={stack.status} />
                        </div>
                      </div>
                      <p className="text-xs text-text-secondary line-clamp-2 mb-3">{stack.description || 'No description'}</p>
                      <div className="flex items-center gap-2 text-xs text-text-muted font-mono">
                        <span className="ml-auto" onClick={(e) => { e.preventDefault(); handleDeleteStack(stack.id, stack.name) }}>delete</span>
                      </div>
                    </div>
                  </a>
                ))}
              </div>
            </>
          )}
        </>
      )}

      {showCreate && <DevCreateWizard onClose={() => setShowCreate(false)} onCreated={(id) => { setShowCreate(false); window.location.hash = `#/dev/${id}` }} requireAuth={requireAuth} />}
      {showCreateStack && <DevCreateStackWizard onClose={() => setShowCreateStack(false)} onCreated={(id) => { setShowCreateStack(false); window.location.hash = `#/dev/stack/${id}` }} requireAuth={requireAuth} />}
      {showUnraid && <DevUnraidImport onClose={() => setShowUnraid(false)} onCreated={(id) => { setShowUnraid(false); window.location.hash = `#/dev/${id}` }} requireAuth={requireAuth} />}
      {showDockerfile && <DevDockerfileImport onClose={() => setShowDockerfile(false)} onCreated={(id) => { setShowDockerfile(false); window.location.hash = `#/dev/${id}` }} requireAuth={requireAuth} />}
    </div>
  )
}

function DevCreateWizard({ onClose, onCreated, requireAuth }: { onClose: () => void; onCreated: (id: string) => void; requireAuth: (cb: () => void) => void }) {
  const [templates, setTemplates] = useState<DevTemplate[]>([])
  const [appId, setAppId] = useState('')
  const [selectedTemplate, setSelectedTemplate] = useState('blank')
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => { api.devTemplates().then(d => setTemplates(d.templates || [])).catch(() => {}) }, [])

  const handleCreate = () => {
    requireAuth(async () => {
      if (!appId) { setError('App ID is required'); return }
      setCreating(true)
      setError('')
      try {
        await api.devCreateApp(appId, selectedTemplate)
        onCreated(appId)
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : 'Failed to create')
      }
      setCreating(false)
    })
  }

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-bg-card border border-border rounded-lg p-6 w-full max-w-lg" onClick={(e) => e.stopPropagation()}>
        <h3 className="text-lg font-bold text-text-primary font-mono mb-4">Create New App</h3>

        <div className="mb-4">
          <label className="text-xs text-text-muted font-mono uppercase block mb-1">App ID (kebab-case)</label>
          <input
            value={appId}
            onChange={(e) => setAppId(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))}
            placeholder="my-awesome-app"
            className="w-full bg-bg-primary border border-border rounded px-3 py-2 text-sm font-mono text-text-primary"
            autoFocus
          />
        </div>

        <div className="mb-4">
          <label className="text-xs text-text-muted font-mono uppercase block mb-2">Template</label>
          <div className="grid grid-cols-1 gap-2">
            {templates.map(t => (
              <label key={t.id} className={`flex items-start gap-3 p-3 border rounded cursor-pointer transition-colors ${selectedTemplate === t.id ? 'border-primary bg-primary/5' : 'border-border hover:border-primary/50'}`}>
                <input type="radio" name="template" value={t.id} checked={selectedTemplate === t.id} onChange={() => setSelectedTemplate(t.id)} className="mt-1" />
                <div>
                  <span className="text-sm font-mono text-text-primary font-bold">{t.name}</span>
                  <p className="text-xs text-text-muted mt-0.5">{t.description}</p>
                </div>
              </label>
            ))}
          </div>
        </div>

        {error && <p className="text-red-400 text-sm font-mono mb-3">{error}</p>}

        <div className="flex justify-end gap-2">
          <button onClick={onClose} className="bg-transparent border border-border rounded px-4 py-2 text-sm font-mono text-text-secondary cursor-pointer hover:border-primary transition-colors">Cancel</button>
          <button onClick={handleCreate} disabled={creating || !appId} className="bg-primary text-bg-primary rounded px-4 py-2 text-sm font-mono font-bold cursor-pointer hover:opacity-90 disabled:opacity-50">{creating ? 'Creating...' : 'Create'}</button>
        </div>
      </div>
    </div>
  )
}

function DevUnraidImport({ onClose, onCreated, requireAuth }: { onClose: () => void; onCreated: (id: string) => void; requireAuth: (cb: () => void) => void }) {
  const [mode, setMode] = useState<'url' | 'xml'>('url')
  const [url, setUrl] = useState('')
  const [xml, setXml] = useState('')
  const [importing, setImporting] = useState(false)
  const [error, setError] = useState('')

  const canImport = mode === 'url' ? url.trim().length > 0 : xml.trim().length > 0

  const handleImport = () => {
    requireAuth(async () => {
      if (!canImport) { setError(mode === 'url' ? 'URL is required' : 'XML content is required'); return }
      setImporting(true)
      setError('')
      try {
        const payload = mode === 'url' ? { url: url.trim() } : { xml: xml.trim() }
        const app = await api.devImportUnraid(payload)
        onCreated(app.id)
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : 'Failed to import')
      }
      setImporting(false)
    })
  }

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-bg-card border border-border rounded-lg p-6 w-full max-w-2xl" onClick={(e) => e.stopPropagation()}>
        <h3 className="text-lg font-bold text-text-primary font-mono mb-4">Import Unraid XML Template</h3>
        <p className="text-xs text-text-muted mb-3">Import an Unraid Docker template to create a scaffold app. Ports, volumes, and environment variables will be converted to inputs.</p>

        <div className="flex gap-2 mb-3">
          <button onClick={() => setMode('url')} className={`px-3 py-1 text-xs font-mono rounded border cursor-pointer transition-colors ${mode === 'url' ? 'border-primary text-primary bg-primary/10' : 'border-border text-text-secondary bg-transparent hover:border-primary'}`}>From URL</button>
          <button onClick={() => setMode('xml')} className={`px-3 py-1 text-xs font-mono rounded border cursor-pointer transition-colors ${mode === 'xml' ? 'border-primary text-primary bg-primary/10' : 'border-border text-text-secondary bg-transparent hover:border-primary'}`}>Paste XML</button>
        </div>

        {mode === 'url' ? (
          <div>
            <input
              type="text"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://raw.githubusercontent.com/linuxserver/templates/main/unraid/app.xml"
              className="w-full bg-bg-primary border border-border rounded px-3 py-2 text-sm font-mono text-text-primary"
            />
            <p className="text-xs text-text-muted mt-2">Paste a direct link to an Unraid XML template file (e.g. from the linuxserver/templates repo on GitHub).</p>
          </div>
        ) : (
          <textarea
            value={xml}
            onChange={(e) => setXml(e.target.value)}
            placeholder={"<?xml version='1.0'?>\n<Container>\n  <Name>MyApp</Name>\n  ..."}
            className="w-full h-64 bg-bg-primary border border-border rounded px-3 py-2 text-sm font-mono text-text-primary resize-none"
          />
        )}

        {error && <p className="text-red-400 text-sm font-mono mt-2">{error}</p>}

        <div className="flex justify-end gap-2 mt-4">
          <button onClick={onClose} className="bg-transparent border border-border rounded px-4 py-2 text-sm font-mono text-text-secondary cursor-pointer hover:border-primary transition-colors">Cancel</button>
          <button onClick={handleImport} disabled={importing || !canImport} className="bg-primary text-bg-primary rounded px-4 py-2 text-sm font-mono font-bold cursor-pointer hover:opacity-90 disabled:opacity-50">{importing ? 'Importing...' : 'Import'}</button>
        </div>
      </div>
    </div>
  )
}

function DevDockerfileImport({ onClose, onCreated, requireAuth }: { onClose: () => void; onCreated: (id: string) => void; requireAuth: (cb: () => void) => void }) {
  const [mode, setMode] = useState<'url' | 'dockerfile'>('url')
  const [url, setUrl] = useState('')
  const [dockerfile, setDockerfile] = useState('')
  const [name, setName] = useState('')
  const [error, setError] = useState('')
  const [phase, setPhase] = useState<'input' | 'resolving' | 'done' | 'error'>('input')
  const [events, setEvents] = useState<DockerfileChainEvent[]>([])
  const [completedAppId, setCompletedAppId] = useState('')
  const abortRef = useRef<AbortController | null>(null)
  const progressRef = useRef<HTMLDivElement>(null)

  // Auto-scroll progress to bottom
  useEffect(() => {
    if (progressRef.current) {
      progressRef.current.scrollTop = progressRef.current.scrollHeight
    }
  }, [events])

  const canImport = name.trim().length > 0 && (mode === 'url' ? url.trim().length > 0 : dockerfile.trim().length > 0)

  const handleImport = () => {
    requireAuth(async () => {
      if (!canImport) { setError('App name and Dockerfile content are required'); return }
      setPhase('resolving')
      setEvents([])
      setError('')

      const controller = new AbortController()
      abortRef.current = controller

      try {
        const payload: { name: string; url?: string; dockerfile?: string } = { name: name.trim() }
        if (mode === 'url') payload.url = url.trim()
        else payload.dockerfile = dockerfile.trim()

        const appId = await api.devImportDockerfileStream(
          payload,
          (event) => setEvents(prev => [...prev, event]),
          controller.signal,
        )

        if (appId) {
          setCompletedAppId(appId)
          setPhase('done')
        } else {
          setPhase('error')
          setError('Import completed but no app ID returned')
        }
      } catch (e: unknown) {
        if ((e as Error).name === 'AbortError') return
        setError(e instanceof Error ? e.message : 'Failed to import')
        setPhase('error')
      }
    })
  }

  const handleCancel = () => {
    if (abortRef.current) abortRef.current.abort()
    onClose()
  }

  const eventIcon = (type: string, index: number) => {
    switch (type) {
      case 'parsed': return <span className="text-primary">&#10003;</span>
      case 'terminal': return <span className="text-blue-400">&#9632;</span>
      case 'fetching': {
        // Show spinner only if this is the last event and still resolving
        const isLast = index === events.length - 1 && phase === 'resolving'
        return isLast
          ? <span className="text-yellow-400 animate-spin inline-block">&#9696;</span>
          : <span className="text-yellow-400">&#8594;</span>
      }
      case 'error': return <span className="text-red-400">&#9888;</span>
      case 'merged': return <span className="text-primary">&#9733;</span>
      case 'complete': return <span className="text-primary">&#10003;</span>
      default: return <span className="text-text-muted">&#8226;</span>
    }
  }

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-50" onClick={handleCancel}>
      <div className="bg-bg-card border border-border rounded-lg p-6 w-full max-w-2xl" onClick={(e) => e.stopPropagation()}>
        <h3 className="text-lg font-bold text-text-primary font-mono mb-4">Import from Dockerfile</h3>

        {phase === 'input' && (
          <>
            <p className="text-xs text-text-muted mb-3">Parse a Dockerfile and resolve its FROM chain to generate a complete LXC app scaffold. Parent base images are fetched automatically.</p>

            <div className="mb-3">
              <label className="text-xs text-text-muted font-mono uppercase block mb-1">App Name *</label>
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="My App"
                className="w-full bg-bg-primary border border-border rounded px-3 py-2 text-sm font-mono text-text-primary"
                autoFocus
              />
            </div>

            <div className="flex gap-2 mb-3">
              <button onClick={() => setMode('url')} className={`px-3 py-1 text-xs font-mono rounded border cursor-pointer transition-colors ${mode === 'url' ? 'border-primary text-primary bg-primary/10' : 'border-border text-text-secondary bg-transparent hover:border-primary'}`}>From URL</button>
              <button onClick={() => setMode('dockerfile')} className={`px-3 py-1 text-xs font-mono rounded border cursor-pointer transition-colors ${mode === 'dockerfile' ? 'border-primary text-primary bg-primary/10' : 'border-border text-text-secondary bg-transparent hover:border-primary'}`}>Paste Dockerfile</button>
            </div>

            {mode === 'url' ? (
              <div>
                <input
                  type="text"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  placeholder="https://raw.githubusercontent.com/owner/repo/main/Dockerfile"
                  className="w-full bg-bg-primary border border-border rounded px-3 py-2 text-sm font-mono text-text-primary"
                />
                <p className="text-xs text-text-muted mt-2">Paste a direct link to a Dockerfile, or a GitHub repo URL (the Dockerfile will be auto-detected).</p>
              </div>
            ) : (
              <textarea
                value={dockerfile}
                onChange={(e) => setDockerfile(e.target.value)}
                placeholder={"FROM ubuntu:22.04\nRUN apt-get update && apt-get install -y nginx\nEXPOSE 80"}
                className="w-full h-64 bg-bg-primary border border-border rounded px-3 py-2 text-sm font-mono text-text-primary resize-none"
              />
            )}

            {error && <p className="text-red-400 text-sm font-mono mt-2">{error}</p>}

            <div className="flex justify-end gap-2 mt-4">
              <button onClick={onClose} className="bg-transparent border border-border rounded px-4 py-2 text-sm font-mono text-text-secondary cursor-pointer hover:border-primary transition-colors">Cancel</button>
              <button onClick={handleImport} disabled={!canImport} className="bg-primary text-bg-primary rounded px-4 py-2 text-sm font-mono font-bold cursor-pointer hover:opacity-90 disabled:opacity-50">Import</button>
            </div>
          </>
        )}

        {(phase === 'resolving' || phase === 'done' || phase === 'error') && (
          <>
            <p className="text-sm text-text-secondary mb-3 font-mono">
              Resolving Dockerfile chain for &quot;{name}&quot;{phase === 'resolving' ? '...' : ''}
            </p>

            <div ref={progressRef} className="overflow-y-auto max-h-[300px] bg-bg-primary border border-border rounded p-3 space-y-2 mb-4">
              {events.filter(e => e.type !== 'complete').map((event, i) => (
                <div key={i} className="flex items-start gap-2 text-sm font-mono">
                  <span className="flex-shrink-0 w-5 text-center">{eventIcon(event.type, i)}</span>
                  <div className="flex-1 min-w-0">
                    <span className={`${event.type === 'error' ? 'text-red-400' : event.type === 'merged' ? 'text-primary' : 'text-text-secondary'}`}>
                      {event.message}
                    </span>
                  </div>
                </div>
              ))}
              {phase === 'resolving' && (
                <div className="flex items-center gap-2 text-sm font-mono text-text-muted">
                  <span className="animate-pulse">...</span>
                </div>
              )}
            </div>

            {phase === 'done' && (
              <div className="flex items-center gap-2 mb-4 p-2 bg-primary/10 border border-primary/30 rounded">
                <span className="text-primary">&#10003;</span>
                <span className="text-sm font-mono text-primary">
                  {events.find(e => e.type === 'complete')?.message || 'Import complete!'}
                </span>
              </div>
            )}

            {error && <p className="text-red-400 text-sm font-mono mb-3">{error}</p>}

            <div className="flex justify-end gap-2">
              <button onClick={handleCancel} className="bg-transparent border border-border rounded px-4 py-2 text-sm font-mono text-text-secondary cursor-pointer hover:border-primary transition-colors">
                {phase === 'done' ? 'Close' : 'Cancel'}
              </button>
              {phase === 'done' && completedAppId && (
                <button onClick={() => onCreated(completedAppId)} className="bg-primary text-bg-primary rounded px-4 py-2 text-sm font-mono font-bold cursor-pointer hover:opacity-90">
                  Open App
                </button>
              )}
              {phase === 'error' && (
                <button onClick={() => { setPhase('input'); setEvents([]); setError('') }} className="bg-primary text-bg-primary rounded px-4 py-2 text-sm font-mono font-bold cursor-pointer hover:opacity-90">
                  Try Again
                </button>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  )
}

// --- Dev Create Stack Wizard ---

function DevCreateStackWizard({ onClose, onCreated, requireAuth }: { onClose: () => void; onCreated: (id: string) => void; requireAuth: (cb: () => void) => void }) {
  const [stackId, setStackId] = useState('')
  const [template, setTemplate] = useState('blank')
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')

  const handleCreate = () => {
    requireAuth(async () => {
      if (!stackId) { setError('Stack ID is required'); return }
      setCreating(true)
      setError('')
      try {
        await api.devCreateStack(stackId, template)
        onCreated(stackId)
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : 'Failed')
        setCreating(false)
      }
    })
  }

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-bg-card border border-border rounded-lg p-6 w-full max-w-md" onClick={(e) => e.stopPropagation()}>
        <h3 className="text-lg font-bold text-text-primary font-mono mb-4">New Stack</h3>
        <label className="block text-xs text-text-muted font-mono mb-1">Stack ID (kebab-case)</label>
        <input
          type="text" value={stackId} onChange={e => setStackId(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))}
          placeholder="my-stack" className="w-full bg-bg-primary border border-border rounded px-3 py-2 text-sm font-mono text-text-primary mb-4 outline-none focus:border-primary"
        />
        <label className="block text-xs text-text-muted font-mono mb-1">Template</label>
        <select value={template} onChange={e => setTemplate(e.target.value)} className="w-full bg-bg-primary border border-border rounded px-3 py-2 text-sm font-mono text-text-primary mb-4 outline-none focus:border-primary">
          <option value="blank">Blank</option>
          <option value="web-database">Web + Database</option>
        </select>
        {error && <p className="text-xs text-red-400 font-mono mb-3">{error}</p>}
        <div className="flex justify-end gap-2">
          <button onClick={onClose} className="bg-transparent border border-border rounded px-4 py-2 text-sm font-mono text-text-secondary cursor-pointer hover:border-primary transition-colors">Cancel</button>
          <button onClick={handleCreate} disabled={creating} className="bg-primary text-bg-primary rounded px-4 py-2 text-sm font-mono font-bold cursor-pointer hover:opacity-90 disabled:opacity-50">{creating ? 'Creating...' : 'Create Stack'}</button>
        </div>
      </div>
    </div>
  )
}

export { DeveloperDashboard, DevCreateWizard, DevUnraidImport, DevDockerfileImport, DevCreateStackWizard }
