import { useState, useEffect, useCallback } from 'react'
import { api } from '../api'
import type { DevStack, ValidationResult } from '../types'
import { Center, DevStatusBadge } from '../components/ui'
import { CodeEditor } from '../CodeEditor'
import { DevSubmitDialog, DevValidationMsg } from '../DevAppEditor'

function DevStackEditor({ id, requireAuth }: { id: string; requireAuth: (cb: () => void) => void }) {
  const [stack, setStack] = useState<DevStack | null>(null)
  const [manifest, setManifest] = useState('')
  const [dirty, setDirty] = useState(false)
  const [validation, setValidation] = useState<ValidationResult | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [showSubmit, setShowSubmit] = useState(false)

  const fetchStack = useCallback(async () => {
    try {
      const data = await api.devGetStack(id)
      setStack(data)
      setManifest(data.manifest || '')
    } catch (e: unknown) { setError(e instanceof Error ? e.message : 'Not found') }
  }, [id])

  useEffect(() => { fetchStack() }, [fetchStack])

  const handleSave = () => {
    requireAuth(async () => {
      setSaving(true)
      try {
        await api.devSaveStackManifest(id, manifest)
        setDirty(false)
        fetchStack()
      } catch (e: unknown) { setError(e instanceof Error ? e.message : 'Save failed') }
      setSaving(false)
    })
  }

  const handleValidate = () => {
    requireAuth(async () => {
      try {
        const result = await api.devValidateStack(id)
        setValidation(result)
      } catch (e: unknown) { setError(e instanceof Error ? e.message : 'Validation failed') }
    })
  }

  const handleDeploy = () => {
    requireAuth(async () => {
      try {
        await api.devDeployStack(id)
        fetchStack()
      } catch (e: unknown) { alert(e instanceof Error ? e.message : 'Deploy failed') }
    })
  }

  const handleUndeploy = () => {
    requireAuth(async () => {
      try {
        await api.devUndeployStack(id)
        fetchStack()
      } catch (e: unknown) { alert(e instanceof Error ? e.message : 'Undeploy failed') }
    })
  }

  const handleExport = () => {
    requireAuth(() => {
      const form = document.createElement('form')
      form.method = 'POST'
      form.action = api.devExportStackUrl(id)
      form.target = '_blank'
      document.body.appendChild(form)
      form.submit()
      document.body.removeChild(form)
    })
  }

  if (error && !stack) return <Center className="py-16"><span className="text-red-400 font-mono text-sm">{error}</span></Center>
  if (!stack) return <Center className="py-16"><span className="text-text-muted font-mono">Loading...</span></Center>

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-3">
          <a href="#/developer" className="text-text-muted hover:text-primary text-sm font-mono">&larr; Dashboard</a>
          <h2 className="text-lg font-bold text-text-primary font-mono">{stack.name || id}</h2>
          <DevStatusBadge status={stack.status} />
          <span className="text-xs text-text-muted font-mono">{stack.app_count} app{stack.app_count !== 1 ? 's' : ''}</span>
        </div>
        <div className="flex gap-2">
          <button onClick={handleExport} className="bg-transparent border border-border rounded px-3 py-1.5 text-xs font-mono text-text-secondary cursor-pointer hover:border-primary transition-colors">Export</button>
          <button onClick={() => setShowSubmit(true)} className="bg-transparent border border-border rounded px-3 py-1.5 text-xs font-mono text-text-secondary cursor-pointer hover:border-primary transition-colors">Publish</button>
          {stack.deployed ? (
            <button onClick={handleUndeploy} className="bg-transparent border border-yellow-400 rounded px-3 py-1.5 text-xs font-mono text-yellow-400 cursor-pointer hover:opacity-80">Undeploy</button>
          ) : (
            <button onClick={handleDeploy} className="bg-primary text-bg-primary rounded px-3 py-1.5 text-xs font-mono font-bold cursor-pointer hover:opacity-90">Deploy</button>
          )}
        </div>
      </div>

      {error && <p className="text-xs text-red-400 font-mono mb-3">{error}</p>}

      {/* Stack Manifest Editor */}
      <div className="border border-border rounded-lg overflow-hidden mb-4">
        <div className="flex items-center justify-between bg-bg-card px-4 py-2 border-b border-border">
          <span className="text-xs font-mono text-text-muted">stack.yml</span>
          <div className="flex gap-2">
            <button onClick={handleValidate} className="bg-transparent border border-border rounded px-3 py-1 text-xs font-mono text-text-secondary cursor-pointer hover:border-primary transition-colors">Validate</button>
            <button onClick={handleSave} disabled={saving || !dirty} className="bg-primary text-bg-primary rounded px-3 py-1 text-xs font-mono font-bold cursor-pointer hover:opacity-90 disabled:opacity-50">{saving ? 'Saving...' : dirty ? 'Save *' : 'Save'}</button>
          </div>
        </div>
        <CodeEditor value={manifest} onChange={(v) => { setManifest(v); setDirty(true) }} filename="stack.yml" onSave={handleSave} />
      </div>

      {/* Validation Results */}
      {validation && (
        <div className="border border-border rounded-lg p-4 mb-4 bg-bg-card">
          <div className="flex items-center gap-2 mb-3">
            <span className={`text-sm font-mono font-bold ${validation.valid ? 'text-primary' : 'text-red-400'}`}>
              {validation.valid ? 'Valid' : 'Invalid'}
            </span>
          </div>
          {validation.errors.length > 0 && validation.errors.map((e, i) => <DevValidationMsg key={i} msg={e} type="error" />)}
          {validation.warnings.length > 0 && validation.warnings.map((w, i) => <DevValidationMsg key={i} msg={w} type="warning" />)}
          {validation.checklist && (
            <div className="mt-3 border-t border-border pt-3">
              {validation.checklist.map((c, i) => (
                <div key={i} className="flex items-center gap-2 py-0.5 text-xs font-mono">
                  <span className={c.passed ? 'text-primary' : 'text-red-400'}>{c.passed ? '[x]' : '[ ]'}</span>
                  <span className="text-text-secondary">{c.label}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {showSubmit && <DevSubmitDialog id={id} appName={stack.name || id} onClose={() => setShowSubmit(false)} requireAuth={requireAuth} isStack />}
    </div>
  )
}

export { DevStackEditor }
