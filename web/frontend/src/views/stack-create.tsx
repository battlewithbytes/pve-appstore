import { useState, useEffect } from 'react'
import { api } from '../api'
import type { AppSummary, AppDetail, StackCreateRequest, StackValidateResponse } from '../types'
import { FormField, FormInput } from '../components/ui'

export function StackCreateWizard({ requireAuth }: { requireAuth: (cb: () => void) => void }) {
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
  const [macAddress, setMacAddress] = useState('')
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
          mac_address: macAddress || undefined,
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
          <div className="mb-4">
            <FormField label="MAC Address (optional)">
              <FormInput value={macAddress} onChange={setMacAddress} placeholder="e.g. BC:24:11:AB:CD:EF (blank = auto)" />
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
