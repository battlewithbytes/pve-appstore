import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import { api } from './api'
import { CodeEditor } from './CodeEditor'
import type { DevApp, ValidationResult } from './types'
import { DevStatusBadge, Center, BackLink } from './components/ui'
import { parseStructure, StructurePanel } from './components/StructurePanel'
import { SdkReferencePanel } from './components/SdkReferencePanel'
import { YamlReferencePanel } from './components/YamlReferencePanel'
import { DevSubmitDialog } from './components/DevSubmitDialog'
import { DevValidationMsg } from './components/DevValidationMsg'
import { FileTree } from './components/FileTree'

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
  const [gotoLine, setGotoLine] = useState(0)
  const iconUploadRef = useRef<HTMLInputElement>(null)

  const protectedFiles = useMemo(() => new Set(['app.yml', 'provision/install.py']), [])
  const coreFileSet = useMemo(() => new Set(['app.yml', 'provision/install.py', 'README.md', 'icon.png']), [])

  const navigateToFile = useCallback((file: string, search?: string) => {
    setActiveFile(file)
    if (search && file === 'app.yml') {
      const lines = manifest.split('\n')
      const needle = `${search}:`
      let found = 0
      for (let i = 0; i < lines.length; i++) {
        if (lines[i].trimStart().startsWith(needle)) {
          found = i + 1
          break
        }
      }
      setGotoLine(found > 0 ? found + Math.random() * 0.001 : 0)
    }
  }, [manifest])

  const handleGotoLine = useCallback((file: string, line: number) => {
    setActiveFile(file)
    setGotoLine(line + Math.random() * 0.001)
  }, [])

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

  // --- Unsaved changes guard ---
  const isDirty = app != null && (
    manifest !== app.manifest ||
    script !== app.script ||
    readme !== app.readme
  )
  const dirtyRef = useRef(false)
  dirtyRef.current = isDirty

  useEffect(() => {
    const handler = (e: BeforeUnloadEvent) => {
      if (dirtyRef.current) e.preventDefault()
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [])

  useEffect(() => {
    const editorHash = `#/dev/${id}`
    const handler = () => {
      if (dirtyRef.current && !window.location.hash.startsWith(editorHash)) {
        if (!confirm('You have unsaved changes. Leave anyway?')) {
          window.location.hash = editorHash
        }
      }
    }
    window.addEventListener('hashchange', handler)
    return () => window.removeEventListener('hashchange', handler)
  }, [id])

  const saveFile = useCallback((file: string, content: string) => {
    requireAuth(async () => {
      setSaving(true)
      setSaveMsg('')
      try {
        if (file === 'app.yml') await api.devSaveManifest(id, content)
        else if (file === 'provision/install.py') await api.devSaveScript(id, content)
        else await api.devSaveFile(id, file, content)
        setApp(prev => {
          if (!prev) return prev
          if (file === 'app.yml') return { ...prev, manifest: content }
          if (file === 'provision/install.py') return { ...prev, script: content }
          if (file === 'README.md') return { ...prev, readme: content }
          return prev
        })
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
        setIconKey(k => k + 1)
      } catch (e: unknown) {
        alert(e instanceof Error ? e.message : 'Failed to set icon')
      }
    })
  }

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
        setExtraFiles(prev => {
          const next = { ...prev }
          delete next[filePath]
          return next
        })
        const updated = await api.devGetApp(id)
        setApp(updated)
        setManifest(updated.manifest)
        setScript(updated.script)
        setReadme(updated.readme)
        if (activeFile === filePath) setActiveFile('app.yml')
      } catch (e: unknown) {
        alert(e instanceof Error ? e.message : 'Delete failed')
      }
    })
  }, [id, requireAuth, activeFile, protectedFiles])

  const handleRenameFile = useCallback((oldPath: string, newPath: string) => {
    requireAuth(async () => {
      try {
        await api.devRenameFile(id, oldPath, newPath)
        setSaveMsg('Renamed')
        setTimeout(() => setSaveMsg(''), 1500)
        // Fetch content at the new path to ensure editor shows it
        let content = ''
        if (!coreFileSet.has(newPath)) {
          try { content = (await api.devGetFile(id, newPath)).content } catch { /* empty */ }
        }
        setExtraFiles(prev => {
          const next = { ...prev }
          delete next[oldPath]
          if (!coreFileSet.has(newPath)) next[newPath] = content
          return next
        })
        const updated = await api.devGetApp(id)
        setApp(updated)
        setManifest(updated.manifest)
        setScript(updated.script)
        setReadme(updated.readme)
        setActiveFile(newPath)
      } catch (e: unknown) {
        alert(e instanceof Error ? e.message : 'Rename failed')
      }
    })
  }, [id, requireAuth, coreFileSet])

  const handleMoveFile = useCallback((filePath: string, targetDir: string) => {
    const fileName = filePath.includes('/') ? filePath.split('/').pop()! : filePath
    const newPath = targetDir ? `${targetDir}/${fileName}` : fileName
    if (newPath === filePath) return
    requireAuth(async () => {
      try {
        await api.devRenameFile(id, filePath, newPath)
        setSaveMsg('Moved')
        setTimeout(() => setSaveMsg(''), 1500)
        // Fetch content at the new path to ensure editor shows it
        let content = ''
        if (!coreFileSet.has(newPath)) {
          try { content = (await api.devGetFile(id, newPath)).content } catch { /* empty */ }
        }
        setExtraFiles(prev => {
          const next = { ...prev }
          delete next[filePath]
          if (!coreFileSet.has(newPath)) next[newPath] = content
          return next
        })
        const updated = await api.devGetApp(id)
        setApp(updated)
        setManifest(updated.manifest)
        setScript(updated.script)
        setReadme(updated.readme)
        setActiveFile(newPath)
      } catch (e: unknown) {
        alert(e instanceof Error ? e.message : 'Move failed')
      }
    })
  }, [id, requireAuth, coreFileSet])

  const handleNewFile = useCallback(async (name: string) => {
    try {
      await api.devSaveFile(id, name, '')
      setExtraFiles(prev => ({ ...prev, [name]: '' }))
      const updated = await api.devGetApp(id)
      setApp(updated)
      setActiveFile(name)
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to create file')
    }
  }, [id])

  const handleUploadIcon = useCallback((file: File) => {
    requireAuth(async () => {
      try {
        const result = await api.devUploadFile(id, 'icon.png', file)
        setSaveMsg(result.resized ? 'Icon uploaded (resized)' : 'Icon uploaded')
        setTimeout(() => setSaveMsg(''), 2000)
        setIconKey(k => k + 1)
        const updated = await api.devGetApp(id)
        setApp(updated)
        setActiveFile('icon.png')
      } catch (err: unknown) { alert(err instanceof Error ? err.message : 'Upload failed') }
    })
  }, [id, requireAuth])

  const currentContent = activeFile === 'app.yml' ? manifest : activeFile === 'provision/install.py' ? script : activeFile === 'README.md' ? readme : (extraFiles[activeFile] ?? '')
  const structureItems = useMemo(() => parseStructure(currentContent, activeFile), [currentContent, activeFile])

  if (!app) return <Center className="py-16"><span className="text-text-muted font-mono">Loading...</span></Center>

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
          <button onClick={() => { requireAuth(async () => { const newId = prompt('New app ID:', id); if (!newId || newId === id) return; try { await api.devRenameApp(id, newId); window.location.hash = `#/dev/${newId}` } catch (e: unknown) { alert(e instanceof Error ? e.message : 'Rename failed') } }) }} className="bg-transparent border border-border rounded px-3 py-1.5 text-xs font-mono text-text-secondary cursor-pointer hover:border-primary hover:text-primary transition-colors">Rename</button>
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

      {/* Hidden input for icon upload from preview */}
      <input
        ref={iconUploadRef}
        type="file"
        className="hidden"
        onChange={e => {
          const f = e.target.files?.[0]
          if (f) handleUploadIcon(f)
          e.target.value = ''
        }}
      />

      {/* Main editor area */}
      <div className="flex gap-4" style={{ height: 'calc(100vh - 280px)' }}>
        {/* File tree + Structure */}
        <div className="w-[20%] min-w-48 border border-border rounded-lg overflow-hidden shrink-0 flex flex-col">
          <FileTree
            files={app.files || []}
            activeFile={activeFile}
            protectedFiles={protectedFiles}
            onSelectFile={selectFile}
            onDeleteFile={handleDeleteFile}
            onRenameFile={handleRenameFile}
            onMoveFile={handleMoveFile}
            onNewFile={handleNewFile}
            onUploadFile={handleUploadFile}
            onUploadIcon={handleUploadIcon}
          />
          <StructurePanel items={structureItems} activeFile={activeFile} onGotoLine={handleGotoLine} />
        </div>

        {/* Editor / Icon Preview */}
        <div className="flex-1 flex flex-col border border-border rounded-lg overflow-hidden">
          <div className="bg-bg-card px-4 py-2 border-b border-border flex items-center justify-between">
            <span className="text-xs font-mono text-text-muted">{activeFile}{isDirty ? ' *' : ''}</span>
            <div className="flex items-center gap-2">
              {isDirty && !saveMsg && <span className="text-xs font-mono text-yellow-400">unsaved</span>}
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
                onClick={() => iconUploadRef.current?.click()}
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
                gotoLine={gotoLine}
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
                  {validation.errors.map((e, i) => <DevValidationMsg key={i} msg={e} type="error" onNavigate={navigateToFile} onGotoLine={handleGotoLine} />)}
                </div>
              )}
              {validation.warnings.length > 0 && (
                <div className="mb-3">
                  <span className="text-xs font-mono text-yellow-400 font-bold px-1">Warnings ({validation.warnings.length})</span>
                  {validation.warnings.map((e, i) => <DevValidationMsg key={i} msg={e} type="warning" onNavigate={navigateToFile} onGotoLine={handleGotoLine} />)}
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

export { DevAppEditor, DevSubmitDialog, DevValidationMsg }
export default DevAppEditor
