import { useState, useEffect, useCallback, useRef } from 'react'
import { api } from '../api'
import type { Job, LogEntry } from '../types'
import { Center, BackLink, StateBadge, InfoCard, InfoRow } from '../components/ui'

export function JobView({ id }: { id: string }) {
  const [job, setJob] = useState<Job | null>(null)
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [cancelling, setCancelling] = useState(false)
  const [cancelError, setCancelError] = useState('')
  const lastLogId = useRef(0)
  const logEndRef = useRef<HTMLDivElement>(null)
  const logContainerRef = useRef<HTMLDivElement>(null)
  const userScrolledUp = useRef(false)

  const refreshJob = useCallback(async () => {
    try {
      const [j, l] = await Promise.all([api.job(id), api.jobLogs(id, lastLogId.current)])
      setJob(j)
      if (l.logs && l.logs.length > 0) {
        setLogs(prev => [...prev, ...l.logs])
        lastLogId.current = l.last_id
      }
    } catch { /* ignore */ }
  }, [id])

  useEffect(() => {
    api.job(id).then(setJob).catch(() => {})
    api.jobLogs(id).then(d => { setLogs(d.logs || []); lastLogId.current = d.last_id })
  }, [id])

  useEffect(() => {
    if (!job || job.state === 'completed' || job.state === 'failed' || job.state === 'cancelled') return
    const interval = setInterval(refreshJob, 1500)
    return () => clearInterval(interval)
  }, [id, job?.state, refreshJob])

  useEffect(() => {
    if (!userScrolledUp.current) logEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [logs])

  const handleCancel = async () => {
    setCancelling(true)
    setCancelError('')
    try {
      await api.cancelJob(id)
      // Immediately refresh to pick up state change
      setTimeout(refreshJob, 500)
      setTimeout(refreshJob, 2000)
      setTimeout(refreshJob, 5000)
    } catch (e) {
      setCancelError(e instanceof Error ? e.message : 'Cancel failed')
      setCancelling(false)
    }
  }

  if (!job) return <Center>Loading...</Center>

  const done = job.state === 'completed' || job.state === 'failed' || job.state === 'cancelled'

  return (
    <div>
      <BackLink />
      <div className="mt-4 flex items-center gap-4">
        <h2 className="text-xl font-bold text-text-primary">{job.type === 'install' ? 'Installing' : job.type === 'uninstall' ? 'Uninstalling' : 'Reinstalling'} {job.app_name}</h2>
        <StateBadge state={job.state} />
        {!done && (
          <button onClick={handleCancel} disabled={cancelling} className="ml-auto px-4 py-1.5 text-sm font-mono rounded-lg border border-status-stopped/50 text-status-stopped hover:bg-status-stopped/10 transition-colors disabled:opacity-50">
            {cancelling ? 'Cancelling...' : 'Cancel'}
          </button>
        )}
      </div>

      {cancelError && <div className="mt-3 p-3 bg-status-stopped/10 border border-status-stopped/30 rounded-lg text-status-stopped text-sm font-mono">{cancelError}</div>}

      <div className="grid grid-cols-[repeat(auto-fit,minmax(200px,1fr))] gap-3 mt-4">
        <InfoCard title="Job Info">
          <InfoRow label="Job ID" value={job.id} />
          <InfoRow label="CTID" value={job.ctid ? String(job.ctid) : 'pending'} />
          <InfoRow label="Node" value={job.node} />
          <InfoRow label="Pool" value={job.pool} />
          {job.cores > 0 && <InfoRow label="Resources" value={`${job.cores}c / ${job.memory_mb}MB / ${job.disk_gb}GB`} />}
        </InfoCard>
        {done && job.outputs && Object.keys(job.outputs).length > 0 && (
          <InfoCard title="Outputs">
            {Object.entries(job.outputs).map(([k, v]) => <InfoRow key={k} label={k} value={v} />)}
          </InfoCard>
        )}
      </div>

      {job.error && <div className="mt-4 p-4 bg-status-stopped/10 border border-status-stopped/30 rounded-lg text-status-stopped text-sm font-mono">{job.error}</div>}

      <div ref={logContainerRef} className="mt-5 bg-bg-card border border-border rounded-lg p-4 max-h-[400px] overflow-auto" onScroll={() => {
        const el = logContainerRef.current
        if (!el) return
        const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40
        userScrolledUp.current = !atBottom
      }}>
        <h4 className="text-xs text-text-muted mb-2 uppercase font-mono tracking-wider">Logs</h4>
        {logs.length === 0 ? <div className="text-text-muted text-sm font-mono">Waiting for logs...</div> : logs.map((l, i) => (
          <div key={i} className={`text-xs font-mono py-0.5 ${l.level === 'error' ? 'text-status-stopped' : l.level === 'warn' ? 'text-status-warning' : 'text-text-secondary'}`}>
            <span className="text-text-muted">{new Date(l.timestamp).toLocaleTimeString()} </span>
            {l.message}
          </div>
        ))}
        <div ref={logEndRef} />
        {!done && !cancelling && <div className="text-status-warning text-xs mt-2 font-mono flex items-center gap-2"><span className="inline-block w-2 h-2 rounded-full bg-status-warning animate-pulse-glow" /> Running...</div>}
        {!done && cancelling && <div className="text-status-stopped text-xs mt-2 font-mono flex items-center gap-2"><span className="inline-block w-2 h-2 rounded-full bg-status-stopped animate-pulse-glow" /> Cancelling...</div>}
        {job.state === 'cancelled' && <div className="text-text-muted text-xs mt-2 font-mono flex items-center gap-2"><span className="inline-block w-2 h-2 rounded-full bg-text-muted" /> Cancelled</div>}
      </div>
    </div>
  )
}

export function JobsList() {
  const [jobs, setJobs] = useState<Job[]>([])
  const [loading, setLoading] = useState(true)
  const [clearing, setClearing] = useState(false)
  const [confirmClear, setConfirmClear] = useState(false)

  const load = () => api.jobs().then(d => { setJobs(d.jobs || []); setLoading(false) })
  useEffect(() => { load() }, [])

  const terminalStates = ['completed', 'failed', 'cancelled']
  const hasTerminalJobs = jobs.some(j => terminalStates.includes(j.state))

  const handleClear = async () => {
    setClearing(true)
    try {
      await api.clearJobs()
      setConfirmClear(false)
      await load()
    } catch { /* ignore */ }
    setClearing(false)
  }

  return (
    <div>
      <div className="flex justify-between items-center mb-5">
        <h2 className="text-xl font-bold text-text-primary font-mono">Jobs</h2>
        {hasTerminalJobs && !confirmClear && (
          <button onClick={() => setConfirmClear(true)} className="px-3 py-1.5 text-sm rounded border border-border text-text-muted hover:text-red-400 hover:border-red-400/50 transition-colors font-mono">Clear</button>
        )}
        {confirmClear && (
          <div className="flex items-center gap-2">
            <span className="text-sm text-text-muted">Delete completed/failed jobs?</span>
            <button onClick={handleClear} disabled={clearing} className="px-3 py-1.5 text-sm rounded bg-red-500/20 border border-red-500/50 text-red-400 hover:bg-red-500/30 transition-colors font-mono disabled:opacity-50">{clearing ? 'Clearing...' : 'Confirm'}</button>
            <button onClick={() => setConfirmClear(false)} className="px-3 py-1.5 text-sm rounded border border-border text-text-muted hover:text-text-primary transition-colors font-mono">Cancel</button>
          </div>
        )}
      </div>
      {loading ? <Center>Loading...</Center> : jobs.length === 0 ? <Center>No jobs yet</Center> : (
        <div className="flex flex-col gap-2">
          {jobs.map(j => (
            <a key={j.id} href={`#/job/${j.id}`} className="bg-bg-card border border-border rounded-lg px-4 py-3 no-underline text-inherit flex justify-between items-center hover:border-border-hover transition-colors">
              <div className="flex items-center gap-3">
                <span className="text-text-primary font-semibold">{j.app_name}</span>
                <span className="text-text-muted text-sm font-mono">{j.type}</span>
                {j.ctid > 0 && <span className="text-text-muted text-sm font-mono">CT {j.ctid}</span>}
              </div>
              <StateBadge state={j.state} />
            </a>
          ))}
        </div>
      )}
    </div>
  )
}
