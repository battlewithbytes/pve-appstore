import { useState, useEffect, useCallback } from 'react'
import { api } from '../api'
import type { InstallDetail, AppDetail, MountPoint, EditRequest, ReconfigureRequest, ConfigDefaultsResponse } from '../types'
import { Center, BackLink, Badge, StatusDot, ResourceCard, InfoCard, InfoRow } from '../components/ui'
import { formatUptime, formatBytes, formatBytesShort } from '../lib/format'
import { TerminalModal } from '../components/terminal'

export function InstallDetailView({ id, requireAuth }: { id: string; requireAuth: (cb: () => void) => void }) {
  const [detail, setDetail] = useState<InstallDetail | null>(null)
  const [appInfo, setAppInfo] = useState<AppDetail | null>(null)
  const [error, setError] = useState('')
  const [uninstalling, setUninstalling] = useState(false)
  const [reinstalling, setReinstalling] = useState(false)
  const [showTerminal, setShowTerminal] = useState(false)
  const [showUninstallDialog, setShowUninstallDialog] = useState(false)
  const [showUpdateDialog, setShowUpdateDialog] = useState(false)
  const [updating, setUpdating] = useState(false)
  const [showEditDialog, setShowEditDialog] = useState(false)
  const [editing, setEditing] = useState(false)
  const [showReconfigureDialog, setShowReconfigureDialog] = useState(false)
  const [reconfiguring, setReconfiguring] = useState(false)

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

  const handlePurge = () => {
    requireAuth(async () => {
      if (!detail) return
      if (!confirm('Delete this install record? Any preserved volumes will NOT be removed from storage.')) return
      try {
        await api.purgeInstall(detail.id)
        window.location.hash = '#/installs'
      } catch (e: unknown) {
        alert(e instanceof Error ? e.message : 'Delete failed')
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

  const handleEdit = (req: EditRequest) => {
    requireAuth(async () => {
      if (!detail) return
      setEditing(true)
      setShowEditDialog(false)
      try {
        const j = await api.editInstall(detail.id, req)
        window.location.hash = `#/job/${j.id}`
      } catch (e: unknown) {
        alert(e instanceof Error ? e.message : 'Edit failed')
        setEditing(false)
      }
    })
  }

  const handleReconfigure = (req: ReconfigureRequest) => {
    requireAuth(async () => {
      if (!detail) return
      setReconfiguring(true)
      setShowReconfigureDialog(false)
      try {
        await api.reconfigure(detail.id, req)
        fetchDetail()
      } catch (e: unknown) {
        alert(e instanceof Error ? e.message : 'Reconfigure failed')
      } finally {
        setReconfiguring(false)
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
          {isUninstalled && (
            <button onClick={handlePurge} className="px-4 py-2 text-sm font-mono border border-status-stopped/30 rounded-lg cursor-pointer text-status-stopped bg-status-stopped/10 hover:bg-status-stopped/20 transition-colors">
              Delete Record
            </button>
          )}
          {!isUninstalled && (
            <button onClick={() => setShowReconfigureDialog(true)} disabled={reconfiguring} className="px-4 py-2 text-sm font-mono border border-border rounded-lg cursor-pointer text-text-primary bg-transparent hover:border-primary hover:text-primary transition-colors disabled:opacity-50">
              {reconfiguring ? 'Applying...' : 'Reconfigure'}
            </button>
          )}
          {!isUninstalled && (
            <button onClick={() => setShowEditDialog(true)} disabled={editing} className="px-4 py-2 text-sm font-mono border border-border rounded-lg cursor-pointer text-text-primary bg-transparent hover:border-primary hover:text-primary transition-colors disabled:opacity-50">
              {editing ? 'Editing...' : 'Edit'}
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

      {/* Edit dialog */}
      {showEditDialog && detail && (
        <EditDialog
          detail={detail}
          appInfo={appInfo}
          bridges={[]}
          isRunning={isRunning}
          onConfirm={handleEdit}
          onCancel={() => setShowEditDialog(false)}
        />
      )}

      {/* Reconfigure dialog */}
      {showReconfigureDialog && detail && (
        <ReconfigureDialog
          detail={detail}
          appInfo={appInfo}
          onConfirm={handleReconfigure}
          onCancel={() => setShowReconfigureDialog(false)}
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
          {detail.mac_address && <InfoRow label="MAC Address" value={detail.mac_address} />}
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

export function UninstallDialog({ appName, ctid, mountPoints, onConfirm, onCancel }: {
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

export function UpdateDialog({ appName, ctid, currentVersion, newVersion, isRunning, onConfirm, onCancel }: {
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

// --- Edit Dialog ---

export function EditDialog({ detail, appInfo, isRunning, onConfirm, onCancel }: {
  detail: InstallDetail; appInfo: AppDetail | null; bridges: string[];
  isRunning: boolean; onConfirm: (req: EditRequest) => void; onCancel: () => void;
}) {
  const [cores, setCores] = useState(String(detail.cores))
  const [memoryMB, setMemoryMB] = useState(String(detail.memory_mb))
  const [diskGB, setDiskGB] = useState(String(detail.disk_gb))
  const [bridge, setBridge] = useState(detail.bridge)
  const [inputs, setInputs] = useState<Record<string, string>>({ ...(detail.inputs || {}) })
  const [configDefaults, setConfigDefaults] = useState<ConfigDefaultsResponse | null>(null)

  useEffect(() => {
    api.configDefaults().then(setConfigDefaults).catch(() => {})
  }, [])

  const bridgeOptions = configDefaults?.bridges || [detail.bridge]

  const appInputs = appInfo?.inputs || []

  const hasChanges = () => {
    if (Number(cores) !== detail.cores) return true
    if (Number(memoryMB) !== detail.memory_mb) return true
    if (Number(diskGB) !== detail.disk_gb) return true
    if (bridge !== detail.bridge) return true
    for (const key of Object.keys(inputs)) {
      if (inputs[key] !== (detail.inputs?.[key] || '')) return true
    }
    return false
  }

  const buildRequest = (): EditRequest => {
    const req: EditRequest = {}
    if (Number(cores) !== detail.cores) req.cores = Number(cores)
    if (Number(memoryMB) !== detail.memory_mb) req.memory_mb = Number(memoryMB)
    if (Number(diskGB) !== detail.disk_gb) req.disk_gb = Number(diskGB)
    if (bridge !== detail.bridge) req.bridge = bridge
    const changedInputs: Record<string, string> = {}
    let hasInputChanges = false
    for (const key of Object.keys(inputs)) {
      if (inputs[key] !== (detail.inputs?.[key] || '')) {
        changedInputs[key] = inputs[key]
        hasInputChanges = true
      }
    }
    if (hasInputChanges) req.inputs = changedInputs
    return req
  }

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-[100]" onClick={onCancel}>
      <div className="bg-bg-card border border-border rounded-xl p-8 w-full max-w-[520px] max-h-[80vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
        <h2 className="text-lg font-bold text-text-primary mb-2 font-mono">Edit {detail.app_name}</h2>
        <p className="text-sm text-text-secondary mb-4">
          Modify settings and recreate container CT {detail.ctid}.
        </p>

        {isRunning && (
          <div className="mb-4 p-2.5 bg-status-warning/10 border border-status-warning/30 rounded text-status-warning text-xs font-mono">
            Container is running and will be stopped during this operation.
          </div>
        )}

        {/* Resource fields */}
        <div className="space-y-3 mb-4">
          <div>
            <label className="block text-xs text-text-muted font-mono mb-1">CPU Cores</label>
            <input type="number" min={1} value={cores} onChange={e => setCores(e.target.value)}
              className="w-full bg-bg-secondary border border-border rounded-lg px-3 py-2 text-sm text-text-primary font-mono focus:border-primary outline-none" />
          </div>
          <div>
            <label className="block text-xs text-text-muted font-mono mb-1">Memory (MB)</label>
            <input type="number" min={128} step={128} value={memoryMB} onChange={e => setMemoryMB(e.target.value)}
              className="w-full bg-bg-secondary border border-border rounded-lg px-3 py-2 text-sm text-text-primary font-mono focus:border-primary outline-none" />
          </div>
          <div>
            <label className="block text-xs text-text-muted font-mono mb-1">Disk (GB) — can only grow</label>
            <input type="number" min={detail.disk_gb} value={diskGB} onChange={e => setDiskGB(e.target.value)}
              className="w-full bg-bg-secondary border border-border rounded-lg px-3 py-2 text-sm text-text-primary font-mono focus:border-primary outline-none" />
          </div>
          <div>
            <label className="block text-xs text-text-muted font-mono mb-1">Bridge</label>
            <select value={bridge} onChange={e => setBridge(e.target.value)}
              className="w-full bg-bg-secondary border border-border rounded-lg px-3 py-2 text-sm text-text-primary font-mono focus:border-primary outline-none">
              {bridgeOptions.map(b => <option key={b} value={b}>{b}</option>)}
            </select>
          </div>
        </div>

        {/* App inputs */}
        {appInputs.length > 0 && (
          <div className="mb-4">
            <h3 className="text-xs font-semibold text-text-muted mb-2 uppercase tracking-wider font-mono">App Settings</h3>
            <div className="space-y-3">
              {appInputs.map(inp => (
                <div key={inp.key}>
                  <label className="block text-xs text-text-muted font-mono mb-1">{inp.label || inp.key}</label>
                  {inp.type === 'select' && inp.validation?.enum ? (
                    <select value={inputs[inp.key] || ''} onChange={e => setInputs({ ...inputs, [inp.key]: e.target.value })}
                      className="w-full bg-bg-secondary border border-border rounded-lg px-3 py-2 text-sm text-text-primary font-mono focus:border-primary outline-none">
                      {inp.validation.enum.map(v => <option key={v} value={v}>{v}</option>)}
                    </select>
                  ) : inp.type === 'boolean' ? (
                    <select value={inputs[inp.key] || 'false'} onChange={e => setInputs({ ...inputs, [inp.key]: e.target.value })}
                      className="w-full bg-bg-secondary border border-border rounded-lg px-3 py-2 text-sm text-text-primary font-mono focus:border-primary outline-none">
                      <option value="true">true</option>
                      <option value="false">false</option>
                    </select>
                  ) : (
                    <input
                      type={inp.type === 'number' ? 'number' : inp.type === 'secret' ? 'password' : 'text'}
                      value={inputs[inp.key] || ''}
                      onChange={e => setInputs({ ...inputs, [inp.key]: e.target.value })}
                      className="w-full bg-bg-secondary border border-border rounded-lg px-3 py-2 text-sm text-text-primary font-mono focus:border-primary outline-none"
                    />
                  )}
                  {inp.help && <p className="text-xs text-text-muted mt-0.5">{inp.help}</p>}
                </div>
              ))}
            </div>
          </div>
        )}

        <p className="text-xs text-text-muted mb-4 font-mono">
          Data volumes and MAC address (IP) will be preserved.
        </p>

        <div className="flex gap-3 justify-end">
          <button onClick={onCancel} className="px-5 py-2.5 text-sm font-semibold border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-text-secondary transition-colors font-mono">Cancel</button>
          <button onClick={() => onConfirm(buildRequest())} disabled={!hasChanges()} className="px-5 py-2.5 text-sm font-semibold border-none rounded-lg cursor-pointer bg-primary text-bg-primary hover:shadow-[0_0_20px_rgba(0,255,157,0.3)] transition-all font-mono disabled:opacity-50 disabled:cursor-not-allowed">
            Apply Changes
          </button>
        </div>
      </div>
    </div>
  )
}

// --- Reconfigure Dialog ---

export function ReconfigureDialog({ detail, appInfo, onConfirm, onCancel }: {
  detail: InstallDetail; appInfo: AppDetail | null;
  onConfirm: (req: ReconfigureRequest) => void; onCancel: () => void;
}) {
  const [cores, setCores] = useState(String(detail.cores))
  const [memoryMB, setMemoryMB] = useState(String(detail.memory_mb))
  const [inputs, setInputs] = useState<Record<string, string>>({ ...(detail.inputs || {}) })

  const reconfigurableInputs = (appInfo?.inputs || []).filter(inp => inp.reconfigurable)

  const hasChanges = () => {
    if (Number(cores) !== detail.cores) return true
    if (Number(memoryMB) !== detail.memory_mb) return true
    for (const inp of reconfigurableInputs) {
      if (inputs[inp.key] !== (detail.inputs?.[inp.key] || '')) return true
    }
    return false
  }

  const buildRequest = (): ReconfigureRequest => {
    const req: ReconfigureRequest = {}
    if (Number(cores) !== detail.cores) req.cores = Number(cores)
    if (Number(memoryMB) !== detail.memory_mb) req.memory_mb = Number(memoryMB)
    const changedInputs: Record<string, string> = {}
    let hasInputChanges = false
    for (const inp of reconfigurableInputs) {
      if (inputs[inp.key] !== (detail.inputs?.[inp.key] || '')) {
        changedInputs[inp.key] = inputs[inp.key]
        hasInputChanges = true
      }
    }
    if (hasInputChanges) req.inputs = changedInputs
    return req
  }

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-[100]" onClick={onCancel}>
      <div className="bg-bg-card border border-border rounded-xl p-8 w-full max-w-[520px] max-h-[80vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
        <h2 className="text-lg font-bold text-text-primary mb-2 font-mono">Reconfigure {detail.app_name}</h2>
        <p className="text-sm text-text-secondary mb-4">
          Apply changes in-place without recreating the container.
        </p>

        <div className="mb-4 p-2.5 bg-primary/10 border border-primary/30 rounded text-primary text-xs font-mono">
          Changes are applied live — no downtime or container rebuild required.
        </div>

        {/* Resource fields */}
        <div className="space-y-3 mb-4">
          <div>
            <label className="block text-xs text-text-muted font-mono mb-1">CPU Cores</label>
            <input type="number" min={1} value={cores} onChange={e => setCores(e.target.value)}
              className="w-full bg-bg-secondary border border-border rounded-lg px-3 py-2 text-sm text-text-primary font-mono focus:border-primary outline-none" />
          </div>
          <div>
            <label className="block text-xs text-text-muted font-mono mb-1">Memory (MB)</label>
            <input type="number" min={128} step={128} value={memoryMB} onChange={e => setMemoryMB(e.target.value)}
              className="w-full bg-bg-secondary border border-border rounded-lg px-3 py-2 text-sm text-text-primary font-mono focus:border-primary outline-none" />
          </div>
        </div>

        {/* Reconfigurable app inputs */}
        {reconfigurableInputs.length > 0 && (
          <div className="mb-4">
            <h3 className="text-xs font-semibold text-text-muted mb-2 uppercase tracking-wider font-mono">App Settings</h3>
            <div className="space-y-3">
              {reconfigurableInputs.map(inp => (
                <div key={inp.key}>
                  <label className="block text-xs text-text-muted font-mono mb-1">{inp.label || inp.key}</label>
                  {inp.type === 'select' && inp.validation?.enum ? (
                    <select value={inputs[inp.key] || ''} onChange={e => setInputs({ ...inputs, [inp.key]: e.target.value })}
                      className="w-full bg-bg-secondary border border-border rounded-lg px-3 py-2 text-sm text-text-primary font-mono focus:border-primary outline-none">
                      {inp.validation.enum.map(v => <option key={v} value={v}>{v}</option>)}
                    </select>
                  ) : inp.type === 'boolean' ? (
                    <select value={inputs[inp.key] || 'false'} onChange={e => setInputs({ ...inputs, [inp.key]: e.target.value })}
                      className="w-full bg-bg-secondary border border-border rounded-lg px-3 py-2 text-sm text-text-primary font-mono focus:border-primary outline-none">
                      <option value="true">true</option>
                      <option value="false">false</option>
                    </select>
                  ) : (
                    <input
                      type={inp.type === 'number' ? 'number' : inp.type === 'secret' ? 'password' : 'text'}
                      value={inputs[inp.key] || ''}
                      onChange={e => setInputs({ ...inputs, [inp.key]: e.target.value })}
                      className="w-full bg-bg-secondary border border-border rounded-lg px-3 py-2 text-sm text-text-primary font-mono focus:border-primary outline-none"
                    />
                  )}
                  {inp.help && <p className="text-xs text-text-muted mt-0.5">{inp.help}</p>}
                </div>
              ))}
            </div>
          </div>
        )}

        <div className="flex gap-3 justify-end">
          <button onClick={onCancel} className="px-5 py-2.5 text-sm font-semibold border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-text-secondary transition-colors font-mono">Cancel</button>
          <button onClick={() => onConfirm(buildRequest())} disabled={!hasChanges()} className="px-5 py-2.5 text-sm font-semibold border-none rounded-lg cursor-pointer bg-primary text-bg-primary hover:shadow-[0_0_20px_rgba(0,255,157,0.3)] transition-all font-mono disabled:opacity-50 disabled:cursor-not-allowed">
            Apply
          </button>
        </div>
      </div>
    </div>
  )
}
