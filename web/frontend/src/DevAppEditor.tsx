import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import { api } from './api'
import { CodeEditor } from './CodeEditor'
import type { DevApp, ValidationResult, ValidationMsg, PublishStatus } from './types'
import { DevStatusBadge, Center, BackLink } from './App'

function DevAppEditor({ id, requireAuth }: { id: string; requireAuth: (cb: () => void) => void }) {
  const [app, setApp] = useState<DevApp | null>(null)
  const [activeFile, setActiveFile] = useState('app.yml')
  const [manifest, setManifest] = useState('')
  const [script, setScript] = useState('')
  const [readme, setReadme] = useState('')
  const [extraFiles, setExtraFiles] = useState<Record<string, string>>({})
  const [saving, setSaving] = useState(false)
  const [saveMsg, setSaveMsg] = useState('')
  const [validation, setValidation] = useState<ValidationResult | null>(null)
  const [deploying, setDeploying] = useState(false)
  const [showSubmit, setShowSubmit] = useState(false)
  const [showSdkRef, setShowSdkRef] = useState(false)
  const [showYamlRef, setShowYamlRef] = useState(false)
  const [iconUrl, setIconUrl] = useState('')
  const [showIconInput, setShowIconInput] = useState(false)
  const [iconKey, setIconKey] = useState(0)
  const [showNewFile, setShowNewFile] = useState(false)
  const [newFileName, setNewFileName] = useState('')
  const [showAddMenu, setShowAddMenu] = useState(false)
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; file: string } | null>(null)
  const uploadInputRef = useRef<HTMLInputElement>(null)
  const uploadModeRef = useRef<'general' | 'icon'>('general')

  const fetchApp = useCallback(async () => {
    try {
      const data = await api.devGetApp(id)
      setApp(data)
      setManifest(data.manifest)
      setScript(data.script)
      setReadme(data.readme)
    } catch { /* ignore */ }
  }, [id])

  useEffect(() => { fetchApp() }, [fetchApp])

  const saveFile = useCallback((file: string, content: string) => {
    requireAuth(async () => {
      setSaving(true)
      setSaveMsg('')
      try {
        if (file === 'app.yml') await api.devSaveManifest(id, content)
        else if (file === 'provision/install.py') await api.devSaveScript(id, content)
        else await api.devSaveFile(id, file, content)
        setSaveMsg('Saved')
        setTimeout(() => setSaveMsg(''), 1500)
      } catch (e: unknown) {
        setSaveMsg(`Error: ${e instanceof Error ? e.message : 'unknown'}`)
      }
      setSaving(false)
    })
  }, [id, requireAuth])

  const handleValidate = () => {
    requireAuth(async () => {
      try {
        // Save all files before validating
        await api.devSaveManifest(id, manifest)
        await api.devSaveScript(id, script)
        if (readme) await api.devSaveFile(id, 'README.md', readme)
        const result = await api.devValidate(id)
        setValidation(result)
      } catch (e: unknown) {
        alert(e instanceof Error ? e.message : 'Validation failed')
      }
    })
  }

  const handleDeploy = () => {
    requireAuth(async () => {
      setDeploying(true)
      try {
        // Save all files before deploying
        await api.devSaveManifest(id, manifest)
        await api.devSaveScript(id, script)
        if (readme) await api.devSaveFile(id, 'README.md', readme)
        const result = await api.devDeploy(id)
        alert(result.message)
        fetchApp()
      } catch (e: unknown) {
        alert(e instanceof Error ? e.message : 'Deploy failed')
      }
      setDeploying(false)
    })
  }

  const handleUndeploy = () => {
    requireAuth(async () => {
      try {
        await api.devUndeploy(id)
        fetchApp()
      } catch (e: unknown) {
        alert(e instanceof Error ? e.message : 'Undeploy failed')
      }
    })
  }

  const handleExport = () => {
    requireAuth(() => {
      const form = document.createElement('form')
      form.method = 'POST'
      form.action = api.devExportUrl(id)
      form.target = '_blank'
      document.body.appendChild(form)
      form.submit()
      document.body.removeChild(form)
    })
  }

  const handleSetIcon = () => {
    if (!iconUrl.trim()) return
    requireAuth(async () => {
      try {
        await api.devSetIcon(id, iconUrl.trim())
        setIconUrl('')
        setShowIconInput(false)
        setIconKey(k => k + 1) // bust cache
      } catch (e: unknown) {
        alert(e instanceof Error ? e.message : 'Failed to set icon')
      }
    })
  }

  const coreFileSet = useMemo(() => new Set(['app.yml', 'provision/install.py', 'README.md', 'icon.png']), [])
  const protectedFiles = useMemo(() => new Set(['app.yml', 'provision/install.py']), [])
  const hasIcon = useMemo(() => !!(app?.files || []).find(f => f.path === 'icon.png'), [app])
  const allFiles = useMemo(() => {
    if (!app) return ['app.yml', 'provision/install.py', 'README.md']
    const core = ['app.yml', 'provision/install.py']
    const extra = (app.files || [])
      .filter(f => !f.is_dir && !coreFileSet.has(f.path) && !f.path.startsWith('.') && !f.path.endsWith('.png') && !f.path.endsWith('.jpg') && !f.path.endsWith('.ico'))
      .map(f => f.path)
      .sort()
    return [...core, ...extra, 'README.md', ...(hasIcon ? ['icon.png'] : [])]
  }, [app, coreFileSet, hasIcon])

  const selectFile = useCallback(async (f: string) => {
    setActiveFile(f)
    if (!coreFileSet.has(f) && !(f in extraFiles)) {
      try {
        const data = await api.devGetFile(id, f)
        setExtraFiles(prev => ({ ...prev, [f]: data.content }))
      } catch { setExtraFiles(prev => ({ ...prev, [f]: '' })) }
    }
  }, [id, extraFiles, coreFileSet])

  const handleUploadFile = useCallback((file: File) => {
    const destPath = prompt('Save as:', file.name)
    if (!destPath?.trim()) return
    requireAuth(async () => {
      try {
        const result = await api.devUploadFile(id, destPath.trim(), file)
        if (result.resized) setSaveMsg('Uploaded (resized)')
        else setSaveMsg('Uploaded')
        setTimeout(() => setSaveMsg(''), 2000)
        const updated = await api.devGetApp(id)
        setApp(updated)
        if (destPath.trim() === 'icon.png') {
          setIconKey(k => k + 1)
          setActiveFile('icon.png')
        } else {
          setActiveFile(destPath.trim())
        }
      } catch (e: unknown) {
        alert(e instanceof Error ? e.message : 'Upload failed')
      }
    })
  }, [id, requireAuth])

  const handleDeleteFile = useCallback((filePath: string) => {
    if (protectedFiles.has(filePath)) return
    if (!confirm(`Delete "${filePath}"? This cannot be undone.`)) return
    requireAuth(async () => {
      try {
        await api.devDeleteFile(id, filePath)
        setSaveMsg('Deleted')
        setTimeout(() => setSaveMsg(''), 1500)
        // Clean up local state
        setExtraFiles(prev => {
          const next = { ...prev }
          delete next[filePath]
          return next
        })
        // Re-fetch app to refresh file list
        const updated = await api.devGetApp(id)
        setApp(updated)
        setManifest(updated.manifest)
        setScript(updated.script)
        setReadme(updated.readme)
        // Switch to app.yml if deleted the active file
        if (activeFile === filePath) setActiveFile('app.yml')
      } catch (e: unknown) {
        alert(e instanceof Error ? e.message : 'Delete failed')
      }
    })
  }, [id, requireAuth, activeFile, protectedFiles])

  const handleContextMenu = useCallback((e: React.MouseEvent, file: string) => {
    e.preventDefault()
    setContextMenu({ x: e.clientX, y: e.clientY, file })
  }, [])

  const dismissContextMenu = useCallback(() => setContextMenu(null), [])

  if (!app) return <Center className="py-16"><span className="text-text-muted font-mono">Loading...</span></Center>

  const currentContent = activeFile === 'app.yml' ? manifest : activeFile === 'provision/install.py' ? script : activeFile === 'README.md' ? readme : (extraFiles[activeFile] ?? '')
  const setCurrentContent = activeFile === 'app.yml' ? setManifest : activeFile === 'provision/install.py' ? setScript : activeFile === 'README.md' ? setReadme : ((v: string) => setExtraFiles(prev => ({ ...prev, [activeFile]: v })))

  return (
    <div>
      <BackLink href="#/developer" label="Back to dashboard" />

      {/* Header */}
      <div className="flex items-center justify-between mt-4 mb-4">
        <div className="flex items-center gap-3">
          <div className="relative group cursor-pointer" onClick={() => setShowIconInput(!showIconInput)}>
            <img key={iconKey} src={api.devIconUrl(id) + `?t=${iconKey}`} alt="" className="w-12 h-12 rounded-lg border border-border" onError={e => { (e.target as HTMLImageElement).style.display = 'none' }} />
            <div className="absolute inset-0 bg-black/50 rounded-lg opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
              <span className="text-[10px] text-white font-mono">edit</span>
            </div>
          </div>
          <div>
            <h2 className="text-xl font-bold text-text-primary font-mono">{app.name || app.id}</h2>
            <div className="flex items-center gap-3 mt-1">
              <span className="text-xs text-text-muted font-mono">v{app.version}</span>
              <DevStatusBadge status={app.status} />
            </div>
          </div>
        </div>
        <div className="flex gap-2">
          <button onClick={handleValidate} className="bg-transparent border border-border rounded px-3 py-1.5 text-xs font-mono text-text-secondary cursor-pointer hover:border-blue-400 hover:text-blue-400 transition-colors">Validate</button>
          {app.status === 'deployed' ? (
            <button onClick={handleUndeploy} className="bg-transparent border border-yellow-400 rounded px-3 py-1.5 text-xs font-mono text-yellow-400 cursor-pointer hover:bg-yellow-400/10 transition-colors">Undeploy</button>
          ) : (
            <button onClick={handleDeploy} disabled={deploying} className="bg-primary text-bg-primary rounded px-3 py-1.5 text-xs font-mono font-bold cursor-pointer hover:opacity-90 disabled:opacity-50">{deploying ? 'Deploying...' : 'Deploy'}</button>
          )}
          <button onClick={handleExport} className="bg-transparent border border-border rounded px-3 py-1.5 text-xs font-mono text-text-secondary cursor-pointer hover:border-primary hover:text-primary transition-colors">Export</button>
          <button onClick={() => { setShowYamlRef(!showYamlRef); if (!showYamlRef) setShowSdkRef(false) }} className={`bg-transparent border rounded px-3 py-1.5 text-xs font-mono cursor-pointer transition-colors ${showYamlRef ? 'border-primary text-primary' : 'border-border text-text-secondary hover:border-primary hover:text-primary'}`}>YAML Ref</button>
          <button onClick={() => { setShowSdkRef(!showSdkRef); if (!showSdkRef) setShowYamlRef(false) }} className={`bg-transparent border rounded px-3 py-1.5 text-xs font-mono cursor-pointer transition-colors ${showSdkRef ? 'border-primary text-primary' : 'border-border text-text-secondary hover:border-primary hover:text-primary'}`}>SDK Ref</button>
          <button onClick={() => setShowSubmit(true)} className="bg-transparent border border-border rounded px-3 py-1.5 text-xs font-mono text-text-secondary cursor-pointer hover:border-primary hover:text-primary transition-colors">Publish</button>
          {app.status === 'deployed' && <a href={`#/app/${app.id}?testInstall=1`} className="bg-transparent border border-primary rounded px-3 py-1.5 text-xs font-mono text-primary no-underline hover:bg-primary/10 transition-colors">Test Install</a>}
          <button onClick={() => { requireAuth(async () => { if (!confirm(`Delete "${app.name || id}"? This cannot be undone.`)) return; try { await api.devDeleteApp(id); window.location.hash = '#/developer' } catch (e: unknown) { alert(e instanceof Error ? e.message : 'Failed') } }) }} className="bg-transparent border border-red-500/50 rounded px-3 py-1.5 text-xs font-mono text-red-400 cursor-pointer hover:border-red-500 hover:bg-red-500/10 transition-colors">Delete</button>
        </div>
      </div>

      {showIconInput && (
        <div className="mb-4 p-3 border border-border rounded-lg bg-bg-card flex items-center gap-2">
          <span className="text-xs font-mono text-text-muted whitespace-nowrap">Icon URL:</span>
          <input
            type="text"
            value={iconUrl}
            onChange={e => setIconUrl(e.target.value)}
            placeholder="https://example.com/icon.png"
            className="flex-1 bg-bg-primary border border-border rounded px-2 py-1 text-xs font-mono text-text-primary outline-none focus:border-primary"
            onKeyDown={e => e.key === 'Enter' && handleSetIcon()}
          />
          <button onClick={handleSetIcon} className="bg-primary text-bg-primary rounded px-3 py-1 text-xs font-mono font-bold cursor-pointer hover:opacity-90">Set</button>
          <button onClick={() => setShowIconInput(false)} className="text-text-muted hover:text-text-secondary text-xs font-mono cursor-pointer">Cancel</button>
        </div>
      )}

      {/* Context menu overlay */}
      {contextMenu && (
        <>
          <div className="fixed inset-0 z-40" onClick={dismissContextMenu} onContextMenu={e => { e.preventDefault(); dismissContextMenu() }} />
          <div className="fixed z-50 bg-bg-card border border-border rounded-lg shadow-lg overflow-hidden min-w-[140px]" style={{ left: contextMenu.x, top: contextMenu.y }}>
            {!protectedFiles.has(contextMenu.file) && (
              <button
                className="w-full text-left px-3 py-2 text-xs font-mono text-red-400 hover:bg-red-500/10 cursor-pointer transition-colors"
                onClick={() => { dismissContextMenu(); handleDeleteFile(contextMenu.file) }}
              >Delete</button>
            )}
            {contextMenu.file === 'icon.png' && (
              <button
                className="w-full text-left px-3 py-2 text-xs font-mono text-text-secondary hover:bg-white/5 hover:text-text-primary cursor-pointer transition-colors border-t border-border"
                onClick={() => { dismissContextMenu(); uploadModeRef.current = 'icon'; uploadInputRef.current?.click() }}
              >Replace</button>
            )}
            <button
              className="w-full text-left px-3 py-2 text-xs font-mono text-text-secondary hover:bg-white/5 hover:text-text-primary cursor-pointer transition-colors border-t border-border"
              onClick={() => { dismissContextMenu(); setShowNewFile(true); setNewFileName('') }}
            >New file</button>
            <button
              className="w-full text-left px-3 py-2 text-xs font-mono text-text-secondary hover:bg-white/5 hover:text-text-primary cursor-pointer transition-colors border-t border-border"
              onClick={() => { dismissContextMenu(); uploadModeRef.current = 'general'; uploadInputRef.current?.click() }}
            >Upload file</button>
          </div>
        </>
      )}

      {/* Main editor area */}
      <div className="flex gap-4" style={{ height: 'calc(100vh - 280px)' }}>
        {/* File tree */}
        <div className="w-48 border border-border rounded-lg overflow-hidden shrink-0 flex flex-col">
          <div className="bg-bg-card px-3 py-2 border-b border-border flex items-center justify-between">
            <span className="text-xs text-text-muted font-mono uppercase">Files</span>
            <div className="relative">
              <button
                onClick={() => setShowAddMenu(!showAddMenu)}
                className="text-text-muted hover:text-primary text-sm font-mono cursor-pointer transition-colors leading-none"
                title="Add file"
              >+</button>
              {showAddMenu && (
                <>
                  <div className="fixed inset-0 z-10" onClick={() => setShowAddMenu(false)} />
                  <div className="absolute right-0 top-full mt-1 z-20 bg-bg-card border border-border rounded-lg shadow-lg overflow-hidden min-w-[120px]">
                    <button
                      className="w-full text-left px-3 py-2 text-xs font-mono text-text-secondary hover:bg-white/5 hover:text-text-primary cursor-pointer transition-colors"
                      onClick={() => { setShowAddMenu(false); setShowNewFile(true); setNewFileName('') }}
                    >New file</button>
                    <button
                      className="w-full text-left px-3 py-2 text-xs font-mono text-text-secondary hover:bg-white/5 hover:text-text-primary cursor-pointer transition-colors border-t border-border"
                      onClick={() => { setShowAddMenu(false); uploadModeRef.current = 'general'; uploadInputRef.current?.click() }}
                    >Upload file</button>
                  </div>
                </>
              )}
            </div>
            <input
              ref={uploadInputRef}
              type="file"
              className="hidden"
              onChange={e => {
                const f = e.target.files?.[0]
                if (f) {
                  if (uploadModeRef.current === 'icon') {
                    requireAuth(async () => {
                      try {
                        const result = await api.devUploadFile(id, 'icon.png', f)
                        setSaveMsg(result.resized ? 'Icon uploaded (resized)' : 'Icon uploaded')
                        setTimeout(() => setSaveMsg(''), 2000)
                        setIconKey(k => k + 1)
                        const updated = await api.devGetApp(id)
                        setApp(updated)
                        setActiveFile('icon.png')
                      } catch (err: unknown) { alert(err instanceof Error ? err.message : 'Upload failed') }
                    })
                  } else {
                    handleUploadFile(f)
                  }
                }
                e.target.value = ''
              }}
            />
          </div>
          <div
            className="p-1 flex-1 overflow-y-auto"
            onContextMenu={e => {
              // Right-click on empty space in file panel
              if (e.target === e.currentTarget) {
                e.preventDefault()
                setContextMenu({ x: e.clientX, y: e.clientY, file: '' })
              }
            }}
          >
            {allFiles.map(f => (
              <button
                key={f}
                onClick={() => selectFile(f)}
                onContextMenu={e => handleContextMenu(e, f)}
                className={`w-full text-left px-3 py-1.5 text-xs font-mono rounded cursor-pointer transition-colors ${activeFile === f ? 'bg-primary/10 text-primary' : 'text-text-secondary hover:text-text-primary hover:bg-white/5'}`}
              >
                {f}
              </button>
            ))}
            {showNewFile && (
              <form
                className="px-1 py-1"
                onSubmit={async (e) => {
                  e.preventDefault()
                  const name = newFileName.trim()
                  if (!name) return
                  try {
                    await api.devSaveFile(id, name, '')
                    setExtraFiles(prev => ({ ...prev, [name]: '' }))
                    setShowNewFile(false)
                    setNewFileName('')
                    // Re-fetch app to refresh file list
                    const updated = await api.devGetApp(id)
                    setApp(updated)
                    setActiveFile(name)
                  } catch (err) {
                    alert(err instanceof Error ? err.message : 'Failed to create file')
                  }
                }}
              >
                <input
                  autoFocus
                  value={newFileName}
                  onChange={e => setNewFileName(e.target.value)}
                  onBlur={() => { if (!newFileName.trim()) setShowNewFile(false) }}
                  placeholder="filename..."
                  className="w-full bg-bg-primary border border-primary rounded px-2 py-1 text-xs font-mono text-text-primary outline-none"
                  onKeyDown={e => { if (e.key === 'Escape') setShowNewFile(false) }}
                />
              </form>
            )}
          </div>
        </div>

        {/* Editor / Icon Preview */}
        <div className="flex-1 flex flex-col border border-border rounded-lg overflow-hidden">
          <div className="bg-bg-card px-4 py-2 border-b border-border flex items-center justify-between">
            <span className="text-xs font-mono text-text-muted">{activeFile}</span>
            <div className="flex items-center gap-2">
              {saveMsg && <span className={`text-xs font-mono ${saveMsg.startsWith('Error') ? 'text-red-400' : 'text-primary'}`}>{saveMsg}</span>}
              {activeFile !== 'icon.png' && (
                <button
                  onClick={() => saveFile(activeFile, currentContent)}
                  disabled={saving}
                  className="bg-primary text-bg-primary rounded px-3 py-1 text-xs font-mono font-bold cursor-pointer hover:opacity-90 disabled:opacity-50"
                >{saving ? 'Saving...' : 'Save'}</button>
              )}
            </div>
          </div>
          {activeFile === 'icon.png' ? (
            <div className="flex-1 flex flex-col items-center justify-center gap-4 p-8">
              <img
                key={iconKey}
                src={api.devIconUrl(id) + `?t=${iconKey}`}
                alt="App icon"
                className="w-48 h-48 rounded-xl border border-border object-contain bg-bg-secondary"
                onError={e => { (e.target as HTMLImageElement).src = '' }}
              />
              <span className="text-xs text-text-muted font-mono">256x256 max &middot; PNG or JPEG</span>
              <button
                onClick={() => { uploadModeRef.current = 'icon'; uploadInputRef.current?.click() }}
                className="bg-primary text-bg-primary rounded px-4 py-2 text-xs font-mono font-bold cursor-pointer hover:opacity-90"
              >Replace Icon</button>
            </div>
          ) : (
            <div className="flex-1 overflow-hidden">
              <CodeEditor
                value={currentContent}
                onChange={setCurrentContent}
                filename={activeFile}
                onSave={() => saveFile(activeFile, currentContent)}
              />
            </div>
          )}
        </div>

        {/* Validation panel */}
        {validation && (
          <div className="w-72 border border-border rounded-lg overflow-hidden shrink-0">
            <div className="bg-bg-card px-3 py-2 border-b border-border flex items-center justify-between">
              <span className="text-xs text-text-muted font-mono uppercase">Validation</span>
              <span className={`text-xs font-mono font-bold ${validation.valid ? 'text-primary' : 'text-red-400'}`}>{validation.valid ? 'PASS' : 'FAIL'}</span>
            </div>
            <div className="p-2 overflow-y-auto" style={{ maxHeight: 'calc(100vh - 340px)' }}>
              {validation.errors.length > 0 && (
                <div className="mb-3">
                  <span className="text-xs font-mono text-red-400 font-bold px-1">Errors ({validation.errors.length})</span>
                  {validation.errors.map((e, i) => <DevValidationMsg key={i} msg={e} type="error" />)}
                </div>
              )}
              {validation.warnings.length > 0 && (
                <div className="mb-3">
                  <span className="text-xs font-mono text-yellow-400 font-bold px-1">Warnings ({validation.warnings.length})</span>
                  {validation.warnings.map((e, i) => <DevValidationMsg key={i} msg={e} type="warning" />)}
                </div>
              )}
              <div>
                <span className="text-xs font-mono text-text-muted font-bold px-1">Checklist</span>
                {validation.checklist.map((c, i) => (
                  <div key={i} className="flex items-center gap-2 px-1 py-1 text-xs font-mono">
                    <span className={c.passed ? 'text-primary' : 'text-text-muted'}>{c.passed ? '[x]' : '[ ]'}</span>
                    <span className={c.passed ? 'text-text-secondary' : 'text-text-muted'}>{c.label}</span>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}
      </div>
      {showYamlRef && <YamlReferencePanel />}
      {showSdkRef && <SdkReferencePanel />}
      {showSubmit && <DevSubmitDialog id={id} appName={app.name || app.id} onClose={() => setShowSubmit(false)} requireAuth={requireAuth} />}
    </div>
  )
}

const sdkReference = [
  { group: 'Package Management', methods: [
    { name: 'self.apt_install(packages)', desc: 'Install apt packages. packages: list[str]' },
    { name: 'self.pip_install(packages)', desc: 'Install pip packages. packages: list[str]' },
    { name: 'self.add_apt_key(url, keyring)', desc: 'Download GPG key and save to keyring path' },
    { name: 'self.add_apt_repo(line, file)', desc: 'Add APT repository source line to file' },
  ]},
  { group: 'File Operations', methods: [
    { name: 'self.write_config(path, content)', desc: 'Write content to a file, creating dirs as needed' },
    { name: 'self.create_dir(path, owner?, mode?)', desc: 'Create a directory with optional owner and mode' },
    { name: 'self.chown(path, user, group)', desc: 'Change ownership of a file or directory' },
    { name: 'self.download(url, dest)', desc: 'Download a file from URL to destination path' },
  ]},
  { group: 'Service Management', methods: [
    { name: 'self.enable_service(name)', desc: 'Enable and start a systemd service' },
    { name: 'self.restart_service(name)', desc: 'Restart a systemd service' },
  ]},
  { group: 'Commands', methods: [
    { name: 'self.run_command(cmd)', desc: 'Run a shell command, raises on non-zero exit' },
    { name: 'self.run_installer_script(url)', desc: 'Download and execute an installer script' },
  ]},
  { group: 'User Management', methods: [
    { name: 'self.create_user(username, ...)', desc: 'Create a system user with optional home dir, shell, groups' },
  ]},
  { group: 'Inputs', methods: [
    { name: 'self.inputs.string(key, default?)', desc: 'Get string input value' },
    { name: 'self.inputs.integer(key, default?)', desc: 'Get integer input value' },
    { name: 'self.inputs.boolean(key, default?)', desc: 'Get boolean input value' },
    { name: 'self.inputs.secret(key)', desc: 'Get secret input value (not logged)' },
  ]},
  { group: 'Logging', methods: [
    { name: 'self.log(message)', desc: 'Log a message to the job output' },
  ]},
]

function SdkReferencePanel() {
  return (
    <div className="border border-border rounded-lg mt-4 overflow-hidden">
      <div className="bg-bg-card px-4 py-2 border-b border-border">
        <span className="text-xs text-text-muted font-mono uppercase">Python SDK Reference</span>
      </div>
      <div className="p-4 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 max-h-64 overflow-y-auto">
        {sdkReference.map(group => (
          <div key={group.group}>
            <h4 className="text-xs font-mono text-primary font-bold mb-1">{group.group}</h4>
            {group.methods.map(m => (
              <div key={m.name} className="mb-1.5">
                <code className="text-xs font-mono text-text-primary">{m.name}</code>
                <p className="text-xs text-text-muted mt-0.5">{m.desc}</p>
              </div>
            ))}
          </div>
        ))}
      </div>
    </div>
  )
}

const yamlReference = [
  { group: 'Top-Level (required)', fields: [
    { name: 'id', type: 'string', desc: 'Unique kebab-case identifier (e.g. my-app)' },
    { name: 'name', type: 'string', desc: 'Display name shown in the catalog' },
    { name: 'description', type: 'string', desc: 'Short one-line description' },
    { name: 'version', type: 'string', desc: 'App version (e.g. "1.0.0")' },
    { name: 'categories', type: 'string[]', desc: 'At least one: development, media, network, etc.' },
  ]},
  { group: 'Top-Level (optional)', fields: [
    { name: 'overview', type: 'string', desc: 'Multi-line markdown overview (use | for block)' },
    { name: 'tags', type: 'string[]', desc: 'Search tags (e.g. git, ci-cd, docker)' },
    { name: 'homepage', type: 'string', desc: 'URL to project homepage' },
    { name: 'license', type: 'string', desc: 'License identifier (e.g. MIT, GPL-3.0)' },
    { name: 'official', type: 'bool', desc: 'Maintained by the app store team' },
    { name: 'featured', type: 'bool', desc: 'Show in featured section' },
    { name: 'maintainers', type: 'string[]', desc: 'List of maintainer names' },
  ]},
  { group: 'lxc (required)', fields: [
    { name: 'ostemplate', type: 'string', desc: 'Template name (e.g. ubuntu-24.04, debian-12, alpine-3.21)' },
    { name: 'defaults.unprivileged', type: 'bool', desc: 'true recommended; false for special needs' },
    { name: 'defaults.cores', type: 'int', desc: 'CPU cores (min 1)' },
    { name: 'defaults.memory_mb', type: 'int', desc: 'RAM in MB (min 128)' },
    { name: 'defaults.disk_gb', type: 'int', desc: 'Root disk in GB (min 1)' },
    { name: 'defaults.features', type: 'string[]', desc: 'LXC features: nesting, keyctl, fuse' },
    { name: 'defaults.onboot', type: 'bool', desc: 'Start container on host boot' },
    { name: 'extra_config', type: 'string[]', desc: 'Raw lxc.* config lines appended to CT conf' },
  ]},
  { group: 'inputs[]', fields: [
    { name: 'key', type: 'string', desc: 'Unique key used in install.py via self.inputs' },
    { name: 'label', type: 'string', desc: 'Display label in the install form' },
    { name: 'type', type: 'string', desc: 'string | number | boolean | secret | select' },
    { name: 'default', type: 'any', desc: 'Default value (type-appropriate)' },
    { name: 'required', type: 'bool', desc: 'Must be provided at install time' },
    { name: 'reconfigurable', type: 'bool', desc: 'Can be changed post-install via reconfigure' },
    { name: 'group', type: 'string', desc: 'Group inputs visually (e.g. General, Network)' },
    { name: 'description', type: 'string', desc: 'Short description below the input' },
    { name: 'help', type: 'string', desc: 'Tooltip/help text' },
    { name: 'validation.regex', type: 'string', desc: 'Regex pattern to validate string input' },
    { name: 'validation.min / max', type: 'number', desc: 'Min/max for number inputs' },
    { name: 'validation.min_length / max_length', type: 'int', desc: 'String length constraints' },
    { name: 'validation.enum', type: 'string[]', desc: 'Allowed values for select type' },
    { name: 'show_when.input', type: 'string', desc: 'Key of another input to check' },
    { name: 'show_when.values', type: 'string[]', desc: 'Show only when input matches one of these' },
  ]},
  { group: 'volumes[]', fields: [
    { name: 'name', type: 'string', desc: 'Unique volume name (e.g. data, config)' },
    { name: 'type', type: 'string', desc: 'volume (managed disk) or bind (host path)' },
    { name: 'mount_path', type: 'string', desc: 'Absolute path inside the container' },
    { name: 'size_gb', type: 'int', desc: 'Size in GB (type=volume only, min 1)' },
    { name: 'label', type: 'string', desc: 'Display label in the install form' },
    { name: 'default_host_path', type: 'string', desc: 'Suggested host path (type=bind only)' },
    { name: 'required', type: 'bool', desc: 'Volume must be configured at install' },
    { name: 'read_only', type: 'bool', desc: 'Mount as read-only' },
    { name: 'description', type: 'string', desc: 'Description shown in the install form' },
  ]},
  { group: 'gpu', fields: [
    { name: 'required', type: 'bool', desc: 'true = fail if no GPU; false = optional' },
    { name: 'supported', type: 'string[]', desc: 'GPU vendors: nvidia, intel, amd' },
    { name: 'profiles', type: 'string[]', desc: 'Device profiles: nvidia-basic, dri-render' },
    { name: 'notes', type: 'string', desc: 'Shown to user in install form' },
  ]},
  { group: 'provisioning', fields: [
    { name: 'script', type: 'string', desc: 'Path to install script (e.g. provision/install.py)' },
    { name: 'timeout_sec', type: 'int', desc: 'Max provision time (default: 600)' },
    { name: 'env', type: 'map', desc: 'Extra env vars passed to the script' },
  ]},
  { group: 'permissions', fields: [
    { name: 'packages', type: 'string[]', desc: 'Allowed apt/apk packages' },
    { name: 'pip', type: 'string[]', desc: 'Allowed pip packages' },
    { name: 'urls', type: 'string[]', desc: 'Allowed URLs for downloads (supports *)' },
    { name: 'apt_repos', type: 'string[]', desc: 'Allowed APT repository URLs' },
    { name: 'installer_scripts', type: 'string[]', desc: 'Allowed installer script URLs' },
    { name: 'paths', type: 'string[]', desc: 'Allowed filesystem path prefixes' },
    { name: 'commands', type: 'string[]', desc: 'Allowed binary names (supports *)' },
    { name: 'services', type: 'string[]', desc: 'Allowed systemd service names' },
    { name: 'users', type: 'string[]', desc: 'Allowed usernames for create_user()' },
  ]},
  { group: 'outputs[]', fields: [
    { name: 'key', type: 'string', desc: 'Unique output key' },
    { name: 'label', type: 'string', desc: 'Display label (e.g. Web UI, Admin Password)' },
    { name: 'value', type: 'string', desc: 'Template: http://{{ip}}:{{port}} or {{input_key}}' },
  ]},
]

function YamlReferencePanel() {
  return (
    <div className="border border-border rounded-lg mt-4 overflow-hidden">
      <div className="bg-bg-card px-4 py-2 border-b border-border">
        <span className="text-xs text-text-muted font-mono uppercase">app.yml Manifest Reference</span>
      </div>
      <div className="p-4 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 max-h-[400px] overflow-y-auto">
        {yamlReference.map(group => (
          <div key={group.group}>
            <h4 className="text-xs font-mono text-primary font-bold mb-1">{group.group}</h4>
            {group.fields.map(f => (
              <div key={f.name} className="mb-1.5">
                <div className="flex items-baseline gap-1.5">
                  <code className="text-xs font-mono text-text-primary">{f.name}</code>
                  <span className="text-[10px] font-mono text-text-muted">{f.type}</span>
                </div>
                <p className="text-xs text-text-muted mt-0.5">{f.desc}</p>
              </div>
            ))}
          </div>
        ))}
      </div>
    </div>
  )
}

function DevSubmitDialog({ id, appName, onClose, requireAuth, isStack }: { id: string; appName: string; onClose: () => void; requireAuth: (cb: () => void) => void; isStack?: boolean }) {
  const [publishStatus, setPublishStatus] = useState<PublishStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [publishing, setPublishing] = useState(false)
  const [prUrl, setPrUrl] = useState('')
  const [publishAction, setPublishAction] = useState<'created' | 'updated' | ''>('')
  const [error, setError] = useState('')

  useEffect(() => {
    const fetchStatus = isStack ? api.devStackPublishStatus(id) : api.devPublishStatus(id)
    fetchStatus
      .then(s => { setPublishStatus(s); if (s.pr_url) setPrUrl(s.pr_url) })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [id, isStack])

  const handlePublish = () => {
    requireAuth(async () => {
      setPublishing(true)
      setError('')
      try {
        const result = isStack ? await api.devPublishStack(id) : await api.devPublish(id)
        setPrUrl(result.pr_url)
        setPublishAction((result.action as 'created' | 'updated') || 'created')
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : 'Publish failed')
      }
      setPublishing(false)
    })
  }

  const handleExportFallback = () => {
    requireAuth(() => {
      const form = document.createElement('form')
      form.method = 'POST'
      form.action = isStack ? api.devExportStackUrl(id) : api.devExportUrl(id)
      form.target = '_blank'
      document.body.appendChild(form)
      form.submit()
      document.body.removeChild(form)

      const kind = isStack ? 'Stack' : 'App'
      const title = encodeURIComponent(`New ${kind}: ${appName}`)
      const body = encodeURIComponent(`## ${kind} Submission\n\n**${kind} ID:** ${id}\n**${kind} Name:** ${appName}\n\nPlease attach the exported zip file to this issue.`)
      window.open(`https://github.com/battlewithbytes/pve-appstore-catalog/issues/new?title=${title}&body=${body}`, '_blank')
      onClose()
    })
  }

  const checkLabels: Record<string, string> = {
    github_connected: 'GitHub connected',
    validation_passed: 'Manifest validates',
    test_installed: 'Test install exists',
    apps_published: 'All apps published',
  }

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-bg-card border border-border rounded-lg p-6 w-full max-w-lg" onClick={(e) => e.stopPropagation()}>
        <h3 className="text-lg font-bold text-text-primary font-mono mb-4">Submit to Catalog</h3>

        {prUrl ? (
          <div>
            <div className="flex items-center gap-2 mb-4">
              <span className="text-primary text-lg">[OK]</span>
              <span className="text-sm font-mono text-text-primary">{publishAction === 'updated' ? 'Pull request updated!' : 'Pull request created!'}</span>
              {publishStatus?.pr_state && <PRStateBadge state={publishStatus.pr_state} />}
            </div>
            <a href={prUrl} target="_blank" rel="noopener noreferrer" className="text-sm font-mono text-primary underline break-all">{prUrl}</a>
            <div className="flex justify-end mt-4">
              <button onClick={onClose} className="bg-primary text-bg-primary rounded px-4 py-2 text-sm font-mono font-bold cursor-pointer hover:opacity-90">Done</button>
            </div>
          </div>
        ) : loading ? (
          <p className="text-text-muted font-mono text-sm">Checking publish readiness...</p>
        ) : (
          <div>
            {publishStatus && (
              <div className="mb-4">
                <span className="text-xs text-text-muted font-mono uppercase mb-2 block">Publish Checklist</span>
                {Object.entries(publishStatus.checks).map(([key, passed]) => (
                  <div key={key} className="flex items-center gap-2 py-0.5 text-xs font-mono">
                    <span className={passed ? 'text-primary' : 'text-red-400'}>{passed ? '[x]' : '[ ]'}</span>
                    <span className={passed ? 'text-text-secondary' : 'text-text-muted'}>{checkLabels[key] || key}</span>
                  </div>
                ))}
              </div>
            )}

            {publishStatus?.checks.github_connected ? (
              <div>
                <p className="text-xs text-text-muted mb-4">
                  This will push your changes and submit a pull request on the official catalog repository.
                </p>
                {error && <p className="text-xs text-red-400 font-mono mb-3">{error}</p>}
                <div className="flex justify-end gap-2">
                  <button onClick={onClose} className="bg-transparent border border-border rounded px-4 py-2 text-sm font-mono text-text-secondary cursor-pointer hover:border-primary transition-colors">Cancel</button>
                  <button
                    onClick={handlePublish}
                    disabled={publishing || !publishStatus?.ready}
                    className="bg-primary text-bg-primary rounded px-4 py-2 text-sm font-mono font-bold cursor-pointer hover:opacity-90 disabled:opacity-50"
                  >
                    {publishing ? 'Publishing...' : (publishStatus?.published && publishStatus?.pr_state === 'pr_open' ? 'Update Pull Request' : 'Submit Pull Request')}
                  </button>
                </div>
              </div>
            ) : (
              <div>
                <p className="text-xs text-text-muted mb-2">
                  GitHub is not connected. Connect GitHub in Settings to submit via pull request, or use the manual export method below.
                </p>
                <div className="flex justify-end gap-2">
                  <button onClick={onClose} className="bg-transparent border border-border rounded px-4 py-2 text-sm font-mono text-text-secondary cursor-pointer hover:border-primary transition-colors">Cancel</button>
                  <button onClick={() => { window.location.hash = '#/settings'; onClose() }} className="bg-transparent border border-border rounded px-4 py-2 text-sm font-mono text-text-secondary cursor-pointer hover:border-primary transition-colors">Go to Settings</button>
                  <button onClick={handleExportFallback} className="bg-primary text-bg-primary rounded px-4 py-2 text-sm font-mono font-bold cursor-pointer hover:opacity-90">Export + Manual Submit</button>
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

function DevValidationMsg({ msg, type }: { msg: ValidationMsg; type: 'error' | 'warning' }) {
  const color = type === 'error' ? 'border-red-400/30' : 'border-yellow-400/30'
  return (
    <div className={`border-l-2 ${color} px-2 py-1.5 my-1 text-xs font-mono`}>
      <div className="text-text-muted">{msg.file}{msg.line ? `:${msg.line}` : ''}</div>
      <div className={type === 'error' ? 'text-red-300' : 'text-yellow-300'}>{msg.message}</div>
    </div>
  )
}

function PRStateBadge({ state }: { state: string }) {
  const styles: Record<string, string> = {
    pr_open: 'border-yellow-400 text-yellow-400',
    pr_merged: 'border-primary text-primary',
    pr_closed: 'border-red-400 text-red-400',
  }
  const labels: Record<string, string> = {
    pr_open: 'PR Open',
    pr_merged: 'PR Merged',
    pr_closed: 'PR Closed',
  }
  if (!state || !labels[state]) return null
  return <span className={`text-xs font-mono px-2 py-0.5 border rounded ${styles[state] || ''}`}>{labels[state]}</span>
}

export { DevAppEditor, DevSubmitDialog, DevValidationMsg }
export default DevAppEditor
