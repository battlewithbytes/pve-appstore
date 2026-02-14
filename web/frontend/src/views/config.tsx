import { useState, useEffect } from 'react'
import { api } from '../api'
import type { ConfigDefaultsResponse, ExportResponse, ApplyPreviewResponse, InstallRequest, StackCreateRequest } from '../types'
import { Badge, InfoCard, InfoRow } from '../components/ui'

export function ConfigView({ requireAuth }: { requireAuth: (cb: () => void) => void }) {
  const [defaults, setDefaults] = useState<ConfigDefaultsResponse | null>(null)
  const [exportData, setExportData] = useState<ExportResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [applyYaml, setApplyYaml] = useState('')
  const [applyPreview, setApplyPreview] = useState<ApplyPreviewResponse | null>(null)
  const [applyError, setApplyError] = useState('')
  const [applyJobs, setApplyJobs] = useState<{ app_id: string; job_id: string }[]>([])
  const [applyStackJobs, setApplyStackJobs] = useState<{ name: string; job_id: string }[]>([])
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
    const data: Record<string, unknown> = { recipes: exportData.recipes }
    if (exportData.stacks && exportData.stacks.length > 0) data.stacks = exportData.stacks
    await navigator.clipboard.writeText(JSON.stringify(data, null, 2))
  }

  const handlePreview = () => {
    setApplyError('')
    setApplyPreview(null)
    requireAuth(async () => {
      try {
        const result = await api.configApplyPreview(applyYaml)
        if (result.errors && result.errors.length > 0) {
          setApplyError(result.errors.join('; '))
        }
        if ((!result.recipes || result.recipes.length === 0) && (!result.stacks || result.stacks.length === 0)) {
          setApplyError('No recipes or stacks found in the input.')
          return
        }
        setApplyPreview(result)
      } catch (e: unknown) {
        setApplyError(e instanceof Error ? e.message : 'Preview failed')
      }
    })
  }

  const handleApply = () => {
    if (!applyPreview) return
    requireAuth(async () => {
      setApplying(true)
      setApplyError('')
      try {
        const recipes = (applyPreview.recipes || []) as InstallRequest[]
        const stacks = (applyPreview.stacks || []).map(s => ({
          name: s.name,
          apps: s.apps,
          storage: s.storage,
          bridge: s.bridge,
          cores: s.cores,
          memory_mb: s.memory_mb,
          disk_gb: s.disk_gb,
          hostname: s.hostname,
          onboot: s.onboot,
          unprivileged: s.unprivileged,
          devices: s.devices,
          env_vars: s.env_vars,
          bind_mounts: s.bind_mounts,
          extra_mounts: s.extra_mounts,
          volume_storages: s.volume_storages,
        })) as StackCreateRequest[]
        const result = await api.configApply(recipes, stacks)
        setApplyJobs(result.jobs || [])
        setApplyStackJobs(result.stack_jobs || [])
        setApplyPreview(null)
        setApplyYaml('')
      } catch (e: unknown) {
        setApplyError(e instanceof Error ? e.message : 'Apply failed')
      }
      setApplying(false)
    })
  }

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (file) {
      const reader = new FileReader()
      reader.onload = () => setApplyYaml(reader.result as string)
      reader.readAsText(file)
    }
    e.target.value = '' // reset so same file can be re-selected
  }

  const recipeCount = applyPreview?.recipes?.length || 0
  const stackCount = applyPreview?.stacks?.length || 0
  const totalItems = recipeCount + stackCount

  return (
    <div>
      <h2 className="text-xl font-bold text-text-primary mb-5 font-mono">Backup &amp; Restore</h2>

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
              Exported {exportData.recipes.length} recipe(s){exportData.stacks && exportData.stacks.length > 0 ? ` + ${exportData.stacks.length} stack(s)` : ''} | node: {exportData.node} | {exportData.version ? `v${exportData.version.replace(/^v/, '')}` : ''} | {new Date(exportData.exported_at).toLocaleString()}
            </div>
            <pre className="bg-bg-secondary border border-border rounded-lg p-4 text-xs font-mono text-text-secondary max-h-[400px] overflow-auto whitespace-pre-wrap">
              {JSON.stringify({ recipes: exportData.recipes, ...(exportData.stacks && exportData.stacks.length > 0 ? { stacks: exportData.stacks } : {}) }, null, 2)}
            </pre>
          </div>
        )}
      </div>

      {/* Section 3: Apply / Restore */}
      <div className="mt-4 bg-bg-card border border-border rounded-lg p-5">
        <h3 className="text-xs font-semibold text-text-muted mb-3 uppercase tracking-wider font-mono">Apply / Restore</h3>
        <p className="text-xs text-text-muted mb-3">Paste exported YAML or JSON, or upload a file to restore apps and stacks.</p>
        <div className="flex gap-2 mb-3">
          <label className="px-4 py-2 text-sm font-mono border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-primary hover:text-primary transition-colors inline-flex items-center gap-2">
            Upload File (.yml / .json)
            <input type="file" accept=".yml,.yaml,.json" className="hidden" onChange={handleFileUpload} />
          </label>
        </div>
        <textarea value={applyYaml} onChange={e => setApplyYaml(e.target.value)} placeholder='Paste YAML or JSON here, or upload a file above...'
          rows={6}
          className="w-full px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono placeholder:text-text-muted resize-y" />
        {applyError && <div className="text-status-stopped text-xs mt-2 font-mono">{applyError}</div>}
        <div className="flex gap-2 mt-3">
          <button onClick={handlePreview} disabled={!applyYaml.trim()} className="px-4 py-2 text-sm font-mono border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-primary hover:text-primary transition-colors disabled:opacity-50">
            Preview
          </button>
          {applyPreview && totalItems > 0 && (
            <button onClick={handleApply} disabled={applying} className="px-4 py-2 text-sm font-mono border-none rounded-lg cursor-pointer bg-primary text-bg-primary hover:shadow-[0_0_20px_rgba(0,255,157,0.3)] transition-all disabled:opacity-50 font-semibold">
              {applying ? 'Applying...' : `Apply ${recipeCount > 0 ? `${recipeCount} Recipe(s)` : ''}${recipeCount > 0 && stackCount > 0 ? ' + ' : ''}${stackCount > 0 ? `${stackCount} Stack(s)` : ''}`}
            </button>
          )}
        </div>

        {applyPreview && totalItems > 0 && (
          <div className="mt-3 border border-border rounded-lg overflow-hidden">
            <div className="bg-bg-secondary px-3 py-2 text-xs font-mono text-text-muted uppercase tracking-wider">Preview</div>
            {(applyPreview.recipes || []).map((r, i) => (
              <div key={`r-${i}`} className="flex justify-between items-center px-3 py-2 border-t border-border">
                <div className="flex items-center gap-2">
                  <Badge className="bg-primary/10 text-primary">App</Badge>
                  <span className="text-sm text-text-primary font-semibold">{r.app_id}</span>
                  <span className="text-xs text-text-muted font-mono">{r.cores}c / {r.memory_mb}MB / {r.disk_gb}GB</span>
                </div>
                <span className="text-xs text-text-muted font-mono">{r.storage}</span>
              </div>
            ))}
            {(applyPreview.stacks || []).map((s, i) => (
              <div key={`s-${i}`} className="flex justify-between items-center px-3 py-2 border-t border-border">
                <div className="flex items-center gap-2">
                  <Badge className="bg-blue-500/10 text-blue-400">Stack</Badge>
                  <span className="text-sm text-text-primary font-semibold">{s.name}</span>
                  <span className="text-xs text-text-muted font-mono">{s.apps.length} app(s) &middot; {s.cores}c / {s.memory_mb}MB / {s.disk_gb}GB</span>
                </div>
                <span className="text-xs text-text-muted font-mono">{s.storage}</span>
              </div>
            ))}
          </div>
        )}

        {(applyJobs.length > 0 || applyStackJobs.length > 0) && (
          <div className="mt-3 border border-primary/30 rounded-lg bg-primary/10 p-3">
            <div className="text-xs text-primary font-mono mb-2">Jobs created:</div>
            {applyJobs.map(j => (
              <a key={j.job_id} href={`#/job/${j.job_id}`} className="block text-sm text-primary font-mono hover:underline py-0.5">
                {j.app_id} &rarr; {j.job_id}
              </a>
            ))}
            {applyStackJobs.map(j => (
              <a key={j.job_id} href={`#/job/${j.job_id}`} className="block text-sm text-primary font-mono hover:underline py-0.5">
                Stack: {j.name} &rarr; {j.job_id}
              </a>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
