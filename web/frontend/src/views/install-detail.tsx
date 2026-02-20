import { useState, useEffect, useCallback } from 'react'
import { api } from '../api'
import type { InstallDetail, AppDetail, MountPoint, EditRequest, ReconfigureRequest, ConfigDefaultsResponse, DevicePassthrough, GPUInfo, GPUDriverStatus } from '../types'
import { Center, BackLink, Badge, StatusDot, ResourceCard, InfoCard, InfoRow } from '../components/ui'
import { formatUptime, formatBytes, formatBytesShort } from '../lib/format'
import { TerminalModal } from '../components/terminal'
import { useEscapeKey } from '../hooks/useEscapeKey'

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
  const [showConfigureDialog, setShowConfigureDialog] = useState(false)
  const [configuring, setConfiguring] = useState(false)

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

  const handleUninstall = (keepVolumeNames: string[], deleteBindPaths: string[]) => {
    requireAuth(async () => {
      if (!detail) return
      setUninstalling(true)
      setShowUninstallDialog(false)
      try {
        const j = await api.uninstall(
          detail.id,
          keepVolumeNames.length > 0 ? keepVolumeNames : undefined,
          deleteBindPaths.length > 0 ? deleteBindPaths : undefined,
        )
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

  const handleConfigure = (req: EditRequest, mode: 'live' | 'rebuild') => {
    requireAuth(async () => {
      if (!detail) return
      setConfiguring(true)
      setShowConfigureDialog(false)
      try {
        if (mode === 'live') {
          const reconfReq: ReconfigureRequest = {}
          if (req.cores) reconfReq.cores = req.cores
          if (req.memory_mb) reconfReq.memory_mb = req.memory_mb
          if (req.inputs) reconfReq.inputs = req.inputs
          await api.reconfigure(detail.id, reconfReq)
          fetchDetail()
        } else {
          const j = await api.editInstall(detail.id, req)
          window.location.hash = `#/job/${j.id}`
        }
      } catch (e: unknown) {
        alert(e instanceof Error ? e.message : 'Configure failed')
      } finally {
        setConfiguring(false)
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
            <button onClick={() => setShowConfigureDialog(true)} disabled={configuring} className="px-4 py-2 text-sm font-mono border border-border rounded-lg cursor-pointer text-text-primary bg-transparent hover:border-primary hover:text-primary transition-colors disabled:opacity-50">
              {configuring ? 'Applying...' : 'Configure'}
            </button>
          )}
          {isRunning && (
            <button onClick={() => setShowTerminal(true)} className="px-4 py-2 text-sm font-mono border border-border rounded-lg cursor-pointer text-text-primary bg-transparent hover:border-primary hover:text-primary transition-colors">
              &gt;_ Shell
            </button>
          )}
          {!isUninstalled && (
            <button onClick={() => hasVolumes ? setShowUninstallDialog(true) : handleUninstall([], [])} disabled={uninstalling} className="px-4 py-2 text-sm font-mono border border-status-stopped/30 rounded-lg cursor-pointer text-status-stopped bg-status-stopped/10 hover:bg-status-stopped/20 transition-colors disabled:opacity-50">
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

      {/* Configure dialog (unified edit + reconfigure) */}
      {showConfigureDialog && detail && (
        <ConfigureDialog
          detail={detail}
          appInfo={appInfo}
          isRunning={isRunning}
          onConfirm={handleConfigure}
          onCancel={() => setShowConfigureDialog(false)}
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
  onConfirm: (keepVolumeNames: string[], deleteBindPaths: string[]) => void; onCancel: () => void;
}) {
  useEscapeKey(onCancel)
  const volumes = mountPoints.filter(mp => mp.type === 'volume' || !mp.type)
  const binds = mountPoints.filter(mp => mp.type === 'bind')
  const [kept, setKept] = useState<Set<string>>(() => new Set(volumes.map(v => v.name)))
  const [deleteBinds, setDeleteBinds] = useState<Set<string>>(new Set())

  const allKept = volumes.length > 0 && kept.size === volumes.length
  const noneKept = volumes.length === 0 || kept.size === 0

  const toggleAll = () => {
    if (allKept) setKept(new Set())
    else setKept(new Set(volumes.map(v => v.name)))
  }

  const toggle = (name: string) => {
    const next = new Set(kept)
    if (next.has(name)) next.delete(name); else next.add(name)
    setKept(next)
  }

  const toggleBind = (hostPath: string) => {
    const next = new Set(deleteBinds)
    if (next.has(hostPath)) next.delete(hostPath); else next.add(hostPath)
    setDeleteBinds(next)
  }

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-[100]">
      <div className="bg-bg-card border border-border rounded-xl p-8 w-full max-w-[480px] max-h-[80vh] overflow-y-auto">
        <h2 className="text-lg font-bold text-text-primary mb-2 font-mono">Uninstall {appName}</h2>
        <p className="text-sm text-text-secondary mb-4">This will destroy container CT {ctid}.</p>

        {volumes.length > 0 && (
          <div className="mb-4">
            <label className="flex items-center gap-3 cursor-pointer p-3 bg-bg-secondary rounded-lg mb-2">
              <input type="checkbox" checked={allKept} ref={el => { if (el) el.indeterminate = !allKept && !noneKept }} onChange={toggleAll} className="w-4 h-4 accent-primary" />
              <div>
                <span className="text-sm text-text-primary font-medium">Keep managed volumes</span>
                <p className="text-xs text-text-muted mt-0.5">
                  {allKept ? `All ${volumes.length} volume(s) preserved` : noneKept ? 'All volumes will be destroyed' : `${kept.size} of ${volumes.length} volume(s) preserved`}
                </p>
              </div>
            </label>

            <div className="space-y-1">
              {volumes.map(mp => {
                const isKept = kept.has(mp.name)
                return (
                  <label key={mp.index} className={`flex items-center gap-2.5 text-xs font-mono px-3 py-2 rounded cursor-pointer transition-colors ${isKept ? 'bg-primary/10 text-primary hover:bg-primary/15' : 'bg-status-stopped/10 text-status-stopped hover:bg-status-stopped/15'}`}>
                    <input type="checkbox" checked={isKept} onChange={() => toggle(mp.name)} className="w-3.5 h-3.5 accent-primary" />
                    <span className="flex-1">{mp.name} <span className="text-text-muted">({mp.mount_path})</span></span>
                    <span className="text-[10px] uppercase font-bold">{isKept ? 'keep' : 'destroy'}</span>
                  </label>
                )
              })}
            </div>

            {noneKept && volumes.length > 0 && (
              <div className="mt-3 p-2.5 bg-status-stopped/10 border border-status-stopped/30 rounded text-status-stopped text-xs font-mono">
                Warning: This will permanently delete all data in these volumes.
              </div>
            )}
          </div>
        )}

        {binds.length > 0 && (
          <div className="mb-4">
            <h3 className="text-xs font-semibold text-text-muted mb-2 uppercase tracking-wider font-mono">Bind Mounts (host directories)</h3>
            <p className="text-xs text-text-muted mb-2">These directories live on the host filesystem. By default they are kept.</p>
            <div className="space-y-1">
              {binds.map(mp => {
                const willDelete = deleteBinds.has(mp.host_path || '')
                return (
                  <label key={mp.index} className={`flex items-center gap-2.5 text-xs font-mono px-3 py-2 rounded cursor-pointer transition-colors ${willDelete ? 'bg-status-stopped/10 text-status-stopped hover:bg-status-stopped/15' : 'bg-bg-secondary text-text-secondary hover:bg-bg-secondary/80'}`}>
                    <input type="checkbox" checked={willDelete} onChange={() => toggleBind(mp.host_path || '')} className="w-3.5 h-3.5 accent-[#ef4444]" />
                    <div className="flex-1 min-w-0">
                      <div>{mp.name} <span className="text-text-muted">({mp.mount_path})</span></div>
                      <div className="text-text-muted truncate">{mp.host_path}</div>
                    </div>
                    <span className={`text-[10px] uppercase font-bold shrink-0 ${willDelete ? 'text-status-stopped' : 'text-text-muted'}`}>{willDelete ? 'delete' : 'keep'}</span>
                  </label>
                )
              })}
            </div>

            {deleteBinds.size > 0 && (
              <div className="mt-3 p-2.5 bg-status-stopped/10 border border-status-stopped/30 rounded text-status-stopped text-xs font-mono">
                Warning: {deleteBinds.size} host director{deleteBinds.size === 1 ? 'y' : 'ies'} will be permanently deleted (rm -rf).
              </div>
            )}
          </div>
        )}

        <div className="flex gap-3 justify-end">
          <button onClick={onCancel} className="px-5 py-2.5 text-sm font-semibold border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-text-secondary transition-colors font-mono">Cancel</button>
          <button onClick={() => onConfirm(Array.from(kept), Array.from(deleteBinds))} className="px-5 py-2.5 text-sm font-semibold border-none rounded-lg cursor-pointer bg-status-stopped text-white hover:opacity-90 transition-all font-mono">
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
  useEscapeKey(onCancel)
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

// --- Configure Dialog (unified edit + reconfigure) ---

export function ConfigureDialog({ detail, appInfo, isRunning, onConfirm, onCancel }: {
  detail: InstallDetail; appInfo: AppDetail | null;
  isRunning: boolean; onConfirm: (req: EditRequest, mode: 'live' | 'rebuild') => void; onCancel: () => void;
}) {
  useEscapeKey(onCancel)
  const [cores, setCores] = useState(String(detail.cores))
  const [memoryMB, setMemoryMB] = useState(String(detail.memory_mb))
  const [diskGB, setDiskGB] = useState(String(detail.disk_gb))
  const [bridge, setBridge] = useState(detail.bridge)
  const [inputs, setInputs] = useState<Record<string, string>>({ ...(detail.inputs || {}) })
  const [devices, setDevices] = useState<DevicePassthrough[]>(detail.devices || [])
  const [configDefaults, setConfigDefaults] = useState<ConfigDefaultsResponse | null>(null)
  const [availableGPUs, setAvailableGPUs] = useState<GPUInfo[]>([])
  const [driverStatus, setDriverStatus] = useState<GPUDriverStatus | null>(null)
  const [selectedGPUs, setSelectedGPUs] = useState<Set<string>>(new Set(
    (detail.devices || []).map(d => d.path).filter(p => !p.includes('nvidiactl') && !p.includes('nvidia-uvm'))
  ))

  useEffect(() => {
    api.configDefaults().then(setConfigDefaults).catch(() => {})
  }, [])

  const hasGPUSection = !!(appInfo?.gpu && (appInfo.gpu.required !== undefined || appInfo.gpu.supported?.length || appInfo.gpu.profiles?.length || appInfo.gpu.notes))
  useEffect(() => {
    if (hasGPUSection) {
      api.listGPUs().then(data => {
        setAvailableGPUs(data.gpus)
        setDriverStatus(data.driver_status)
      }).catch(() => {})
    }
  }, [hasGPUSection])

  const bridgeOptions = configDefaults?.bridges || [detail.bridge]
  const appInputs = appInfo?.inputs || []
  const origDevices = detail.devices || []

  // Detect which fields changed
  const coresChanged = Number(cores) !== detail.cores
  const memChanged = Number(memoryMB) !== detail.memory_mb
  const diskChanged = Number(diskGB) !== detail.disk_gb && Number(diskGB) > detail.disk_gb
  const bridgeChanged = bridge !== detail.bridge

  const devicesChanged = (() => {
    const cur = devices.filter(d => d.path.trim())
    if (cur.length !== origDevices.length) return true
    return cur.some((d, i) => d.path !== origDevices[i]?.path)
  })()

  const inputChanges = (() => {
    const changed: Record<string, string> = {}
    for (const key of Object.keys(inputs)) {
      if (inputs[key] !== (detail.inputs?.[key] || '')) changed[key] = inputs[key]
    }
    return changed
  })()
  const hasInputChanges = Object.keys(inputChanges).length > 0

  // Check if any non-reconfigurable input changed
  const hasNonReconfigurableInputChange = hasInputChanges && appInputs.some(
    inp => !inp.reconfigurable && inputChanges[inp.key] !== undefined
  )

  const hasChanges = coresChanged || memChanged || diskChanged || bridgeChanged || hasInputChanges || devicesChanged

  // Determine mode: live (reconfigure) vs rebuild (edit)
  const needsRebuild = diskChanged || bridgeChanged || devicesChanged || hasNonReconfigurableInputChange
  const mode: 'live' | 'rebuild' = needsRebuild ? 'rebuild' : 'live'

  const buildRequest = (): EditRequest => {
    const req: EditRequest = {}
    if (coresChanged) req.cores = Number(cores)
    if (memChanged) req.memory_mb = Number(memoryMB)
    if (diskChanged) req.disk_gb = Number(diskGB)
    if (bridgeChanged) req.bridge = bridge
    if (hasInputChanges) {
      if (mode === 'live') {
        // Only send reconfigurable inputs for live mode
        const reconfInputs: Record<string, string> = {}
        for (const inp of appInputs.filter(i => i.reconfigurable)) {
          if (inputChanges[inp.key] !== undefined) reconfInputs[inp.key] = inputChanges[inp.key]
        }
        if (Object.keys(reconfInputs).length > 0) req.inputs = reconfInputs
      } else {
        req.inputs = inputChanges
      }
    }
    if (devicesChanged) {
      req.devices = devices.filter(d => d.path.trim())
    }
    return req
  }

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-[100]" onClick={onCancel}>
      <div className="bg-bg-card border border-border rounded-xl p-8 w-full max-w-[520px] max-h-[80vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
        <h2 className="text-lg font-bold text-text-primary mb-2 font-mono">Configure {detail.app_name}</h2>
        <p className="text-sm text-text-secondary mb-4">
          Adjust resources, settings, and GPU passthrough for CT {detail.ctid}.
        </p>

        {/* Dynamic mode banner */}
        {hasChanges && (
          needsRebuild ? (
            <div className="mb-4 p-2.5 bg-status-warning/10 border border-status-warning/30 rounded text-status-warning text-xs font-mono">
              Container will be rebuilt to apply these changes.{isRunning ? ' It will be stopped during this operation.' : ''} Data volumes and MAC address (IP) will be preserved.
            </div>
          ) : (
            <div className="mb-4 p-2.5 bg-primary/10 border border-primary/30 rounded text-primary text-xs font-mono">
              Changes will be applied live — no downtime.
            </div>
          )
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
            <label className="block text-xs text-text-muted font-mono mb-1">
              Disk (GB) — can only grow
              {diskChanged && <span className="ml-2 text-status-warning">(requires rebuild)</span>}
            </label>
            <input type="number" min={detail.disk_gb} value={diskGB} onChange={e => setDiskGB(e.target.value)}
              className="w-full bg-bg-secondary border border-border rounded-lg px-3 py-2 text-sm text-text-primary font-mono focus:border-primary outline-none" />
          </div>
          <div>
            <label className="block text-xs text-text-muted font-mono mb-1">
              Bridge
              {bridgeChanged && <span className="ml-2 text-status-warning">(requires rebuild)</span>}
            </label>
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
              {appInputs.map(inp => {
                const changed = inputs[inp.key] !== (detail.inputs?.[inp.key] || '')
                return (
                  <div key={inp.key}>
                    <label className="block text-xs text-text-muted font-mono mb-1">
                      {inp.label || inp.key}
                      {!inp.reconfigurable && changed && <span className="ml-2 text-status-warning">(requires rebuild)</span>}
                    </label>
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
                )
              })}
            </div>
          </div>
        )}

        {/* GPU / Device Passthrough — detected GPUs */}
        {hasGPUSection && (
          <div className="mb-4">
            <h3 className="text-xs font-semibold text-text-muted mb-2 uppercase tracking-wider font-mono">GPU Passthrough</h3>
            {appInfo?.gpu?.notes && <div className="text-xs text-text-muted mb-2 font-mono border-l-2 border-primary/30 pl-2">{appInfo.gpu.notes}</div>}
            {/* Driver warnings */}
            {driverStatus && availableGPUs.some(g => g.type === 'nvidia') && !driverStatus.nvidia_driver_loaded && (
              <div className="mb-2 p-2 bg-status-stopped/10 border border-status-stopped/30 rounded text-status-stopped text-xs font-mono">
                NVIDIA kernel driver not loaded. Install nvidia-driver on the host.
              </div>
            )}
            {driverStatus && availableGPUs.some(g => g.type === 'nvidia') && driverStatus.nvidia_driver_loaded && !driverStatus.nvidia_libs_found && (
              <div className="mb-2 p-2 bg-status-warning/10 border border-status-warning/30 rounded text-status-warning text-xs font-mono">
                NVIDIA userspace libraries not found on host.
              </div>
            )}
            {devicesChanged && <div className="text-status-warning text-[10px] font-mono mb-2">(device changes require rebuild)</div>}
            {availableGPUs.length > 0 ? (
              <div className="space-y-1.5 mb-2">
                {availableGPUs.map(gpu => {
                  const checked = selectedGPUs.has(gpu.path)
                  return (
                    <label key={gpu.path} className="flex items-center gap-2 text-xs text-text-primary cursor-pointer group">
                      <input type="checkbox" checked={checked}
                        onChange={() => {
                          const next = new Set(selectedGPUs)
                          if (checked) next.delete(gpu.path); else next.add(gpu.path)
                          setSelectedGPUs(next)
                          const devs: DevicePassthrough[] = []
                          const seen = new Set<string>()
                          for (const path of next) {
                            const g = availableGPUs.find(x => x.path === path)
                            if (!g) continue
                            if (g.type === 'intel' || g.type === 'amd') {
                              if (!seen.has(path)) { devs.push({ path, gid: 44, mode: '0666' }); seen.add(path) }
                            } else if (g.type === 'nvidia') {
                              if (!seen.has(path)) { devs.push({ path }); seen.add(path) }
                              if (!seen.has('/dev/nvidiactl')) { devs.push({ path: '/dev/nvidiactl' }); seen.add('/dev/nvidiactl') }
                              if (!seen.has('/dev/nvidia-uvm')) { devs.push({ path: '/dev/nvidia-uvm' }); seen.add('/dev/nvidia-uvm') }
                            } else {
                              if (!seen.has(path)) { devs.push({ path }); seen.add(path) }
                            }
                          }
                          setDevices(devs)
                        }}
                        className="w-3.5 h-3.5 accent-primary" />
                      <span className={`inline-block px-1.5 py-0.5 rounded text-[10px] font-bold uppercase ${
                        gpu.type === 'nvidia' ? 'bg-green-500/20 text-green-400' :
                        gpu.type === 'intel' ? 'bg-blue-500/20 text-blue-400' :
                        gpu.type === 'amd' ? 'bg-red-500/20 text-red-400' :
                        'bg-gray-500/20 text-gray-400'
                      }`}>{gpu.type}</span>
                      <span className="font-mono group-hover:text-primary transition-colors">{gpu.name}</span>
                      <span className="text-text-muted font-mono">({gpu.path})</span>
                    </label>
                  )
                })}
              </div>
            ) : (
              <p className="text-xs text-text-muted mb-2">No GPUs detected. You can add custom device paths below.</p>
            )}
            {devices.map((dev, i) => (
              <div key={i} className="flex gap-2 mb-1.5 items-center">
                <input type="text" value={dev.path} onChange={e => setDevices(p => p.map((x, j) => j === i ? { ...x, path: e.target.value } : x))} placeholder="/dev/dri/renderD128"
                  className="flex-1 px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary font-mono placeholder:text-text-muted" readOnly={availableGPUs.some(g => g.path === dev.path || dev.path === '/dev/nvidiactl' || dev.path === '/dev/nvidia-uvm')} />
                {!availableGPUs.some(g => g.path === dev.path) && dev.path !== '/dev/nvidiactl' && dev.path !== '/dev/nvidia-uvm' && (
                  <button type="button" onClick={() => setDevices(p => p.filter((_, j) => j !== i))}
                    className="text-status-stopped text-sm bg-transparent border-none cursor-pointer hover:text-status-stopped/80 leading-none px-1">&times;</button>
                )}
              </div>
            ))}
            <button type="button" onClick={() => setDevices(p => [...p, { path: '' }])}
              className="text-primary text-xs font-mono bg-transparent border-none cursor-pointer hover:underline p-0">
              + Add Device
            </button>
          </div>
        )}

        <div className="flex gap-3 justify-end">
          <button onClick={onCancel} className="px-5 py-2.5 text-sm font-semibold border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-text-secondary transition-colors font-mono">Cancel</button>
          <button onClick={() => onConfirm(buildRequest(), mode)} disabled={!hasChanges} className="px-5 py-2.5 text-sm font-semibold border-none rounded-lg cursor-pointer bg-primary text-bg-primary hover:shadow-[0_0_20px_rgba(0,255,157,0.3)] transition-all font-mono disabled:opacity-50 disabled:cursor-not-allowed">
            {needsRebuild ? 'Rebuild & Apply' : 'Apply Live'}
          </button>
        </div>
      </div>
    </div>
  )
}
