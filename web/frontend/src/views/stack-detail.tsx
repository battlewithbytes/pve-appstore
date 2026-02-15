import { useState, useCallback, useEffect } from 'react'
import { api } from '../api'
import type { StackDetail, StackApp, MountPoint, EditRequest } from '../types'
import { Center, StatusDot, ResourceCard, ActionButton } from '../components/ui'
import { formatUptime, formatBytes, formatBytesShort } from '../lib/format'
import { StackTerminalModal, StackLogViewerModal } from '../components/terminal'
import { StackEditDialog } from './stacks'

export function StackDetailView({ id, requireAuth }: { id: string; requireAuth: (cb: () => void) => void }) {
  const [detail, setDetail] = useState<StackDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showTerminal, setShowTerminal] = useState(false)
  const [showLogs, setShowLogs] = useState(false)
  const [showEditDialog, setShowEditDialog] = useState(false)
  const [editing, setEditing] = useState(false)

  const fetchDetail = useCallback(async () => {
    try {
      const d = await api.stackDetail(id)
      setDetail(d)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load')
    }
    setLoading(false)
  }, [id])

  useEffect(() => { fetchDetail() }, [fetchDetail])
  useEffect(() => {
    const interval = setInterval(fetchDetail, 5000)
    return () => clearInterval(interval)
  }, [fetchDetail])

  const handleAction = async (action: string) => {
    try {
      switch (action) {
        case 'start': await api.startStack(id); break
        case 'stop': await api.stopStack(id); break
        case 'restart': await api.restartStack(id); break
        case 'uninstall': {
          const job = await api.uninstallStack(id)
          window.location.hash = `#/job/${job.id}`
          return
        }
      }
      setTimeout(fetchDetail, 1000)
    } catch (e) {
      alert(e instanceof Error ? e.message : `${action} failed`)
    }
  }

  const handleStackEdit = (req: EditRequest) => {
    requireAuth(async () => {
      if (!detail) return
      setEditing(true)
      setShowEditDialog(false)
      try {
        const j = await api.editStack(detail.id, req)
        window.location.hash = `#/job/${j.id}`
      } catch (e: unknown) {
        alert(e instanceof Error ? e.message : 'Edit failed')
        setEditing(false)
      }
    })
  }

  if (loading) return <Center>Loading...</Center>
  if (error || !detail) return <Center className="text-status-error">{error || 'Stack not found'}</Center>

  const isRunning = detail.status === 'running'

  return (
    <div>
      <a href="#/stacks" className="text-primary text-xs font-mono no-underline hover:underline mb-4 inline-block">&larr; Back to Stacks</a>

      {/* Header */}
      <div className="flex items-start justify-between mb-6">
        <div>
          <h2 className="text-xl font-bold text-text-primary mb-1 font-mono">{detail.name}</h2>
          <div className="flex items-center gap-3 text-xs font-mono text-text-muted">
            <span>CT {detail.ctid}</span>
            {detail.ip && <span>{detail.ip}</span>}
            {detail.mac_address && <span>MAC {detail.mac_address}</span>}
            <span className="flex items-center gap-1"><StatusDot running={isRunning} /><span className={isRunning ? 'text-status-running' : 'text-status-stopped'}>{detail.status}</span></span>
            {isRunning && detail.live?.uptime ? <span>up {formatUptime(detail.live.uptime)}</span> : null}
          </div>
        </div>
        <div className="flex gap-2">
          {detail.status === 'stopped' && <ActionButton label="Start" onClick={() => requireAuth(() => handleAction('start'))} />}
          {isRunning && (
            <>
              <ActionButton label="Stop" onClick={() => requireAuth(() => handleAction('stop'))} />
              <ActionButton label="Restart" onClick={() => requireAuth(() => handleAction('restart'))} />
              <ActionButton label="Shell" onClick={() => requireAuth(() => setShowTerminal(true))} accent />
              <ActionButton label="Logs" onClick={() => requireAuth(() => setShowLogs(true))} />
            </>
          )}
          <ActionButton label={editing ? 'Editing...' : 'Edit'} onClick={() => setShowEditDialog(true)} />
          <ActionButton label="Remove" onClick={() => requireAuth(() => handleAction('uninstall'))} danger />
        </div>
      </div>

      {/* Stack Edit Dialog */}
      {showEditDialog && detail && (
        <StackEditDialog
          detail={detail}
          isRunning={isRunning}
          onConfirm={handleStackEdit}
          onCancel={() => setShowEditDialog(false)}
        />
      )}

      {/* Resource Cards */}
      {isRunning && detail.live && (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mb-6">
          <ResourceCard label="CPU" value={`${(detail.live.cpu * 100).toFixed(1)}%`} sub={`${detail.live.cpus} cores`} />
          <ResourceCard label="Memory" value={formatBytes(detail.live.mem)} sub={`/ ${formatBytes(detail.live.maxmem)}`} />
          <ResourceCard label="Disk" value={formatBytes(detail.live.disk)} sub={`/ ${formatBytes(detail.live.maxdisk)}`} />
          <ResourceCard label="Network" value={`${formatBytesShort(detail.live.netin)} in`} sub={`${formatBytesShort(detail.live.netout)} out`} />
        </div>
      )}

      {/* Contained Apps */}
      <div className="bg-bg-card border border-border rounded-lg p-5 mb-4">
        <h3 className="text-sm font-bold text-text-primary mb-3 font-mono">Contained Apps ({detail.apps.length})</h3>
        <div className="grid grid-cols-[40px_1fr_100px_80px_1fr] gap-2 px-2 py-1 text-[10px] text-text-muted font-mono uppercase tracking-wider border-b border-border">
          <span>#</span><span>App</span><span>Version</span><span>Status</span><span>Key Outputs</span>
        </div>
        {detail.apps.map((app: StackApp, i: number) => (
          <div key={app.app_id} className="grid grid-cols-[40px_1fr_100px_80px_1fr] gap-2 px-2 py-2 border-b border-border items-center">
            <span className="text-xs font-mono text-text-muted">{i + 1}</span>
            <div className="flex items-center gap-2">
              <div className="w-6 h-6 rounded bg-bg-secondary overflow-hidden flex items-center justify-center">
                <img src={`/api/apps/${app.app_id}/icon`} alt="" className="w-5 h-5 rounded" onError={e => { (e.target as HTMLImageElement).style.display = 'none' }} />
              </div>
              <span className="text-sm text-text-primary">{app.app_name}</span>
            </div>
            <span className="text-xs font-mono text-text-muted">{app.app_version}</span>
            <span className={`text-xs font-mono ${app.status === 'completed' ? 'text-status-running' : app.status === 'failed' ? 'text-status-error' : 'text-text-muted'}`}>
              {app.status}{app.error ? ` (${app.error})` : ''}
            </span>
            <div className="text-xs font-mono text-text-muted truncate">
              {app.outputs && Object.entries(app.outputs).slice(0, 2).map(([k, v]) => (
                <span key={k} className="mr-3">{k}: {v}</span>
              ))}
            </div>
          </div>
        ))}
      </div>

      {/* All Outputs */}
      {detail.apps.some(a => a.outputs && Object.keys(a.outputs).length > 0) && (
        <div className="bg-bg-card border border-border rounded-lg p-5 mb-4">
          <h3 className="text-sm font-bold text-text-primary mb-3 font-mono">Outputs</h3>
          <div className="grid grid-cols-2 gap-2">
            {detail.apps.map(app => app.outputs && Object.entries(app.outputs).map(([k, v]) => (
              <div key={`${app.app_id}-${k}`} className="text-sm font-mono">
                <span className="text-text-muted">{app.app_id}.{k}:</span>{' '}
                {v.startsWith('http') ? <a href={v} target="_blank" rel="noreferrer" className="text-primary hover:underline">{v}</a> : <span className="text-text-primary">{v}</span>}
              </div>
            )))}
          </div>
        </div>
      )}

      {/* Mounts */}
      {detail.mount_points && detail.mount_points.length > 0 && (
        <div className="bg-bg-card border border-border rounded-lg p-5 mb-4">
          <h3 className="text-sm font-bold text-text-primary mb-3 font-mono">Mounts</h3>
          {detail.mount_points.map((mp: MountPoint) => (
            <div key={mp.index} className="flex items-center gap-3 py-1 text-sm font-mono">
              <span className="text-text-muted">mp{mp.index}</span>
              <span className="text-primary">{mp.mount_path}</span>
              <span className="text-text-muted text-xs">({mp.type}{mp.host_path ? `: ${mp.host_path}` : ''}{mp.volume_id ? `: ${mp.volume_id}` : ''})</span>
            </div>
          ))}
        </div>
      )}

      {showTerminal && <StackTerminalModal stackId={id} onClose={() => setShowTerminal(false)} />}
      {showLogs && <StackLogViewerModal stackId={id} onClose={() => setShowLogs(false)} />}
    </div>
  )
}
