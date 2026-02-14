import { useState, useEffect, useCallback } from 'react'
import { api } from '../api'
import type { StackListItem, StackDetail, EditRequest, ConfigDefaultsResponse } from '../types'
import { Center, StatusDot, CtxMenuItem } from '../components/ui'
import { formatUptime } from '../lib/format'
import { StackTerminalModal, StackLogViewerModal } from '../components/terminal'

export function StackEditDialog({ detail, isRunning, onConfirm, onCancel }: {
  detail: StackDetail;
  isRunning: boolean; onConfirm: (req: EditRequest) => void; onCancel: () => void;
}) {
  const [cores, setCores] = useState(String(detail.cores))
  const [memoryMB, setMemoryMB] = useState(String(detail.memory_mb))
  const [diskGB, setDiskGB] = useState(String(detail.disk_gb))
  const [bridge, setBridge] = useState(detail.bridge)
  const [configDefaults, setConfigDefaults] = useState<ConfigDefaultsResponse | null>(null)

  useEffect(() => {
    api.configDefaults().then(setConfigDefaults).catch(() => {})
  }, [])

  const bridgeOptions = configDefaults?.bridges || [detail.bridge]

  const hasChanges = () => {
    if (Number(cores) !== detail.cores) return true
    if (Number(memoryMB) !== detail.memory_mb) return true
    if (Number(diskGB) !== detail.disk_gb) return true
    if (bridge !== detail.bridge) return true
    return false
  }

  const buildRequest = (): EditRequest => {
    const req: EditRequest = {}
    if (Number(cores) !== detail.cores) req.cores = Number(cores)
    if (Number(memoryMB) !== detail.memory_mb) req.memory_mb = Number(memoryMB)
    if (Number(diskGB) !== detail.disk_gb) req.disk_gb = Number(diskGB)
    if (bridge !== detail.bridge) req.bridge = bridge
    return req
  }

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-[100]" onClick={onCancel}>
      <div className="bg-bg-card border border-border rounded-xl p-8 w-full max-w-[520px] max-h-[80vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
        <h2 className="text-lg font-bold text-text-primary mb-2 font-mono">Edit Stack: {detail.name}</h2>
        <p className="text-sm text-text-secondary mb-4">
          Modify resource settings and recreate stack CT {detail.ctid}.
        </p>

        {isRunning && (
          <div className="mb-4 p-2.5 bg-status-warning/10 border border-status-warning/30 rounded text-status-warning text-xs font-mono">
            Stack is running and will be stopped during this operation.
          </div>
        )}

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

export function StacksList({ requireAuth }: { requireAuth: (cb: () => void) => void }) {
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

export function StackContextMenu({ stack, x, y, onClose, onAction, onShell, onLogs }: {
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
