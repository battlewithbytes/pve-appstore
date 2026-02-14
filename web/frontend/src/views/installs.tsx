import { useState, useEffect, useCallback } from 'react'
import { api } from '../api'
import type { InstallListItem, StackListItem, StackApp } from '../types'
import { Center, StatusDot, CtxMenuItem } from '../components/ui'
import { formatUptime } from '../lib/format'
import { TerminalModal, LogViewerModal, MiniBar, StackTerminalModal, StackLogViewerModal } from '../components/terminal'
import { StackContextMenu } from './stacks'

export function InstallsList({ requireAuth }: { requireAuth: (cb: () => void) => void }) {
  const [installs, setInstalls] = useState<InstallListItem[]>([])
  const [stacks, setStacks] = useState<StackListItem[]>([])
  const [loading, setLoading] = useState(true)
  const [contextMenu, setContextMenu] = useState<{ install: InstallListItem; x: number; y: number } | null>(null)
  const [stackMenu, setStackMenu] = useState<{ stack: StackListItem; x: number; y: number } | null>(null)
  const [showTerminal, setShowTerminal] = useState<string | null>(null)
  const [showStackTerminal, setShowStackTerminal] = useState<string | null>(null)
  const [showLogs, setShowLogs] = useState<string | null>(null)
  const [showStackLogs, setShowStackLogs] = useState<string | null>(null)
  const [actionLoading, setActionLoading] = useState<string | null>(null)

  const fetchInstalls = useCallback(async () => {
    try {
      const [instData, stackData] = await Promise.all([api.installs(), api.stacks()])
      setInstalls(instData.installs || [])
      setStacks(stackData.stacks || [])
    } catch { /* ignore */ }
    setLoading(false)
  }, [])

  useEffect(() => { fetchInstalls() }, [fetchInstalls])
  useEffect(() => {
    const interval = setInterval(fetchInstalls, 10000)
    return () => clearInterval(interval)
  }, [fetchInstalls])

  const handleAction = async (action: string, installId: string) => {
    setActionLoading(installId)
    try {
      switch (action) {
        case 'start': await api.startContainer(installId); break
        case 'stop': await api.stopContainer(installId); break
        case 'restart': await api.restartContainer(installId); break
        case 'uninstall': {
          const job = await api.uninstall(installId)
          window.location.hash = `#/job/${job.id}`
          return
        }
        case 'purge': {
          if (!confirm('Delete this install record? Any preserved volumes will NOT be removed from storage.')) break
          await api.purgeInstall(installId)
          break
        }
      }
      setTimeout(fetchInstalls, 1000)
      setTimeout(fetchInstalls, 4000)
    } catch (e) {
      alert(e instanceof Error ? e.message : `${action} failed`)
    }
    setActionLoading(null)
  }

  const handleStackAction = async (action: string, stackId: string) => {
    setActionLoading(`stack-${stackId}`)
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
      setTimeout(fetchInstalls, 1000)
      setTimeout(fetchInstalls, 4000)
    } catch (e) {
      alert(e instanceof Error ? e.message : `${action} failed`)
    }
    setActionLoading(null)
  }

  const resolveUrl = (inst: InstallListItem, url: string) =>
    inst.ip ? url.replace(/\{\{ip\}\}/g, inst.ip) : url

  const getServiceUrls = (inst: InstallListItem) =>
    inst.outputs
      ? Object.entries(inst.outputs).filter(([, v]) => v.startsWith('http')).map(([k, v]) => ({ key: k, url: resolveUrl(inst, v) }))
      : []

  const gridCols = 'grid-cols-[40px_1.5fr_90px_160px_110px_50px_70px_90px_36px]'

  const getStackAppUrls = (stack: StackListItem, app: StackApp) => {
    if (!app.outputs) return []
    return Object.entries(app.outputs)
      .filter(([, v]) => v.startsWith('http'))
      .map(([k, v]) => ({ key: k, url: stack.ip ? v.replace(/\{\{ip\}\}/g, stack.ip) : v }))
  }

  const isEmpty = installs.length === 0 && stacks.length === 0

  return (
    <div>
      <h2 className="text-xl font-bold text-text-primary mb-5 font-mono">Installed Apps</h2>
      {loading ? <Center>Loading...</Center> : isEmpty ? <Center>No apps installed</Center> : (
        <div className="bg-bg-card border border-border rounded-lg overflow-x-auto">
          {/* Table header */}
          <div className={`grid ${gridCols} gap-2 px-4 py-2 bg-bg-secondary text-[10px] text-text-muted font-mono uppercase tracking-wider border-b border-border items-center`}>
            <span></span>
            <span>App</span>
            <span>Status</span>
            <span>Network</span>
            <span>Resources</span>
            <span>Boot</span>
            <span>Uptime</span>
            <span>Created</span>
            <span></span>
          </div>
          {/* Install rows */}
          {installs.map(inst => {
            const isRunning = inst.status === 'running'
            const isUninstalled = inst.status === 'uninstalled'
            const urls = getServiceUrls(inst)
            const isLoading = actionLoading === inst.id
            return (
              <div key={inst.id}
                className={`grid ${gridCols} gap-2 px-4 py-3 border-b border-border items-center hover:bg-bg-secondary/50 cursor-pointer transition-colors ${isUninstalled ? 'opacity-60' : ''} ${isLoading ? 'opacity-50 pointer-events-none' : ''}`}
                onClick={() => window.location.hash = `#/install/${inst.id}`}>
                {/* Icon */}
                <div className="w-8 h-8 rounded bg-bg-secondary overflow-hidden flex items-center justify-center flex-shrink-0">
                  <img src={`/api/apps/${inst.app_id}/icon`} alt="" className="w-7 h-7 rounded" onError={e => { (e.target as HTMLImageElement).style.display = 'none' }} />
                </div>
                {/* Name+Version+CTID+Bars */}
                <div className="min-w-0">
                  <div className="flex items-center gap-1.5 flex-wrap">
                    <span className="text-sm font-semibold text-text-primary truncate">{inst.app_name}</span>
                    {inst.app_version && <span className="text-[10px] text-text-muted font-mono">v{inst.app_version}</span>}
                    {inst.update_available && (
                      <span className="text-[9px] bg-status-warning/20 text-status-warning px-1.5 py-0.5 rounded font-mono">update</span>
                    )}
                  </div>
                  {inst.ctid > 0 && <div className="text-[10px] text-text-muted font-mono">CT {inst.ctid}</div>}
                  {isRunning && inst.live && (
                    <div className="flex items-center gap-2 mt-1">
                      <MiniBar label="CPU" pct={inst.live.cpu * 100} />
                      <MiniBar label="Mem" pct={inst.live.maxmem > 0 ? (inst.live.mem / inst.live.maxmem) * 100 : 0} />
                      <MiniBar label="Disk" pct={inst.live.maxdisk > 0 ? (inst.live.disk / inst.live.maxdisk) * 100 : 0} />
                    </div>
                  )}
                </div>
                {/* Status */}
                <div className="flex items-center gap-1.5">
                  {isUninstalled ? (
                    <span className="text-[10px] font-mono text-status-warning">uninstalled</span>
                  ) : (
                    <>
                      <StatusDot running={isRunning} />
                      <span className={`text-xs font-mono ${isRunning ? 'text-status-running' : 'text-status-stopped'}`}>{inst.status}</span>
                    </>
                  )}
                </div>
                {/* Network: IP + URLs */}
                <div className="min-w-0 text-xs font-mono text-text-secondary">
                  {inst.ip && <div className="truncate">{inst.ip}</div>}
                  {urls.slice(0, 2).map(u => (
                    <a key={u.key} href={u.url} target="_blank" rel="noreferrer"
                      onClick={e => e.stopPropagation()}
                      className="text-primary hover:underline block truncate text-[10px]">{u.url.replace(/^https?:\/\//, '')}</a>
                  ))}
                  {urls.length > 2 && <span className="text-[10px] text-text-muted">+{urls.length - 2} more</span>}
                </div>
                {/* Resources */}
                <span className="text-xs font-mono text-text-muted">{inst.cores}c / {inst.memory_mb}MB / {inst.disk_gb}GB</span>
                {/* Boot */}
                <span className="text-xs font-mono text-text-muted">{inst.onboot ? 'On' : 'Off'}</span>
                {/* Uptime */}
                <span className="text-xs font-mono text-text-muted">
                  {isRunning && inst.uptime ? formatUptime(inst.uptime) : '-'}
                </span>
                {/* Created */}
                <span className="text-[10px] font-mono text-text-muted">
                  {new Date(inst.created_at).toLocaleDateString()}
                </span>
                {/* Actions button */}
                <button
                  onClick={e => { e.stopPropagation(); setContextMenu({ install: inst, x: e.clientX, y: e.clientY }) }}
                  className="text-text-muted hover:text-primary bg-transparent border-none cursor-pointer text-base font-mono p-0 leading-none"
                  title="Actions">&#x22EE;</button>
              </div>
            )
          })}
          {/* Stack rows */}
          {stacks.map(stack => {
            const isRunning = stack.status === 'running'
            const isStackLoading = actionLoading === `stack-${stack.id}`
            return (
              <div key={`stack-${stack.id}`}>
                {/* Stack header row */}
                <div
                  className={`grid ${gridCols} gap-2 px-4 py-3 border-b border-border items-center hover:bg-bg-secondary/50 cursor-pointer transition-colors bg-bg-secondary/30 ${isStackLoading ? 'opacity-50 pointer-events-none' : ''}`}
                  onClick={() => window.location.hash = `#/stack/${stack.id}`}>
                  {/* Icon */}
                  <div className="w-8 h-8 rounded bg-primary/10 flex items-center justify-center flex-shrink-0">
                    <span className="text-primary text-sm font-mono font-bold">S</span>
                  </div>
                  {/* Name+Bars */}
                  <div className="min-w-0">
                    <div className="flex items-center gap-1.5 flex-wrap">
                      <span className="text-sm font-semibold text-text-primary truncate">{stack.name}</span>
                      <span className="text-[9px] bg-primary/15 text-primary px-1.5 py-0.5 rounded font-mono">stack</span>
                      <span className="text-[10px] text-text-muted font-mono">{stack.apps.length} app{stack.apps.length !== 1 ? 's' : ''}</span>
                    </div>
                    {stack.ctid > 0 && <div className="text-[10px] text-text-muted font-mono">CT {stack.ctid}</div>}
                    {isRunning && stack.live && (
                      <div className="flex items-center gap-2 mt-1">
                        <MiniBar label="CPU" pct={stack.live.cpu * 100} />
                        <MiniBar label="Mem" pct={stack.live.maxmem > 0 ? (stack.live.mem / stack.live.maxmem) * 100 : 0} />
                        <MiniBar label="Disk" pct={stack.live.maxdisk > 0 ? (stack.live.disk / stack.live.maxdisk) * 100 : 0} />
                      </div>
                    )}
                  </div>
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
                  {/* Actions button */}
                  <button
                    onClick={e => { e.stopPropagation(); setStackMenu({ stack, x: e.clientX, y: e.clientY }) }}
                    className="text-text-muted hover:text-primary bg-transparent border-none cursor-pointer text-base font-mono p-0 leading-none"
                    title="Actions">&#x22EE;</button>
                </div>
                {/* Indented app rows within stack */}
                {stack.apps.map(app => {
                  const appUrls = getStackAppUrls(stack, app)
                  const appDisplayStatus = app.status === 'completed' ? 'installed' : app.status
                  return (
                    <div key={`stack-${stack.id}-${app.app_id}`}
                      className={`grid ${gridCols} gap-2 px-4 py-2 border-b border-border/50 items-center pl-12 bg-bg-primary/50 cursor-pointer hover:bg-bg-secondary/30 transition-colors`}
                      onClick={() => window.location.hash = `#/app/${app.app_id}`}>
                      {/* Icon */}
                      <div className="w-6 h-6 rounded bg-bg-secondary overflow-hidden flex items-center justify-center flex-shrink-0">
                        <img src={`/api/apps/${app.app_id}/icon`} alt="" className="w-5 h-5 rounded" onError={e => { (e.target as HTMLImageElement).style.display = 'none' }} />
                      </div>
                      {/* Name */}
                      <div className="min-w-0">
                        <div className="flex items-center gap-1.5">
                          <span className="text-xs text-text-secondary truncate">{app.app_name}</span>
                          {app.app_version && <span className="text-[9px] text-text-muted font-mono">v{app.app_version}</span>}
                        </div>
                      </div>
                      {/* Status */}
                      <span className={`text-[10px] font-mono ${app.status === 'completed' ? 'text-status-running' : app.status === 'failed' ? 'text-status-stopped' : 'text-text-muted'}`}>
                        {appDisplayStatus}
                      </span>
                      {/* URLs */}
                      <div className="min-w-0 text-xs font-mono">
                        {appUrls.slice(0, 2).map(u => (
                          <a key={u.key} href={u.url} target="_blank" rel="noreferrer"
                            onClick={e => e.stopPropagation()}
                            className="text-primary hover:underline block truncate text-[10px]">{u.url.replace(/^https?:\/\//, '')}</a>
                        ))}
                      </div>
                      {/* Resources — shared with stack */}
                      <span className="text-[10px] font-mono text-text-muted/50">—</span>
                      {/* Boot */}
                      <span className="text-[10px] font-mono text-text-muted/50">—</span>
                      {/* Uptime */}
                      <span className="text-[10px] font-mono text-text-muted/50">—</span>
                      {/* Created */}
                      <span className="text-[10px] font-mono text-text-muted/50">—</span>
                      {/* Actions spacer */}
                      <span></span>
                    </div>
                  )
                })}
              </div>
            )
          })}
        </div>
      )}

      {/* Context Menu — Installs */}
      {contextMenu && (
        <InstallContextMenu
          install={contextMenu.install}
          x={contextMenu.x}
          y={contextMenu.y}
          onClose={() => setContextMenu(null)}
          requireAuth={requireAuth}
          onAction={(action, id) => { setContextMenu(null); requireAuth(() => handleAction(action, id)) }}
          onShell={id => { setContextMenu(null); requireAuth(() => setShowTerminal(id)) }}
          onLogs={id => { setContextMenu(null); setShowLogs(id) }}
        />
      )}

      {/* Context Menu — Stacks */}
      {stackMenu && (
        <StackContextMenu
          stack={stackMenu.stack}
          x={stackMenu.x}
          y={stackMenu.y}
          onClose={() => setStackMenu(null)}
          onAction={(action, id) => { setStackMenu(null); requireAuth(() => handleStackAction(action, id)) }}
          onShell={id => { setStackMenu(null); requireAuth(() => setShowStackTerminal(id)) }}
          onLogs={id => { setStackMenu(null); setShowStackLogs(id) }}
        />
      )}

      {showTerminal && <TerminalModal installId={showTerminal} onClose={() => setShowTerminal(null)} />}
      {showStackTerminal && <StackTerminalModal stackId={showStackTerminal} onClose={() => setShowStackTerminal(null)} />}
      {showLogs && <LogViewerModal installId={showLogs} onClose={() => setShowLogs(null)} />}
      {showStackLogs && <StackLogViewerModal stackId={showStackLogs} onClose={() => setShowStackLogs(null)} />}
    </div>
  )
}

export function InstallContextMenu({ install, x, y, onClose, onAction, onShell, onLogs }: {
  install: InstallListItem;
  x: number; y: number;
  onClose: () => void;
  requireAuth: (cb: () => void) => void;
  onAction: (action: string, id: string) => void;
  onShell: (id: string) => void;
  onLogs: (id: string) => void;
}) {
  const isRunning = install.status === 'running'
  const isStopped = install.status === 'stopped'
  const isUninstalled = install.status === 'uninstalled'

  const menuY = Math.min(y, window.innerHeight - 320)
  const menuX = Math.min(x, window.innerWidth - 200)

  return (
    <>
      <div className="fixed inset-0 z-[299]" onClick={onClose} />
      <div style={{ position: 'fixed', top: menuY, left: menuX, zIndex: 300 }}
        className="bg-bg-card border border-border rounded-lg shadow-lg py-1 min-w-[170px]">
        {isStopped && (
          <CtxMenuItem label="Start" onClick={() => onAction('start', install.id)} />
        )}
        {isRunning && (
          <>
            <CtxMenuItem label="Stop" onClick={() => onAction('stop', install.id)} />
            <CtxMenuItem label="Restart" onClick={() => onAction('restart', install.id)} />
          </>
        )}
        {(isRunning || isStopped) && <div className="border-t border-border my-1" />}
        {isRunning && (
          <>
            <CtxMenuItem label="Logs" onClick={() => onLogs(install.id)} />
            <CtxMenuItem label="Shell" onClick={() => onShell(install.id)} />
            <div className="border-t border-border my-1" />
          </>
        )}
        <CtxMenuItem label="Details" onClick={() => { onClose(); window.location.hash = `#/install/${install.id}` }} />
        <CtxMenuItem label="App Store Page" onClick={() => { onClose(); window.location.hash = `#/app/${install.app_id}` }} />
        {!isUninstalled && (
          <>
            <div className="border-t border-border my-1" />
            <CtxMenuItem label="Remove" danger onClick={() => onAction('uninstall', install.id)} />
          </>
        )}
        {isUninstalled && (
          <>
            <div className="border-t border-border my-1" />
            <CtxMenuItem label="Delete Record" danger onClick={() => onAction('purge', install.id)} />
          </>
        )}
      </div>
    </>
  )
}
