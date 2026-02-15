import { useState, useEffect, useCallback, useRef } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { api } from '../api'
import type { Job, LogEntry } from '../types'
import { Center, BackLink, StateBadge, InfoCard, InfoRow } from '../components/ui'

/** xterm.js-based read-only log viewer for job provision output. */
function JobLogTerminal({ jobId, done }: { jobId: string; done: boolean }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const lastLogId = useRef(0)
  const writtenCount = useRef(0)

  // Initialize xterm once
  useEffect(() => {
    if (!containerRef.current) return

    const term = new Terminal({
      cursorBlink: false,
      disableStdin: true,
      convertEol: true,
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 13,
      scrollback: 10000,
      theme: {
        background: '#111111',
        foreground: '#9CA3AF',
        cursor: 'transparent',
        selectionBackground: 'rgba(0,255,157,0.3)',
        black: '#111111',
        red: '#ff4444',
        green: '#00ff9d',
        yellow: '#ffaa00',
        blue: '#60a5fa',
        magenta: '#c084fc',
        cyan: '#22d3ee',
        white: '#e5e7eb',
        brightBlack: '#6b7280',
        brightRed: '#ff6666',
        brightGreen: '#00ff9d',
        brightYellow: '#ffd700',
        brightBlue: '#93c5fd',
        brightMagenta: '#d8b4fe',
        brightCyan: '#67e8f9',
        brightWhite: '#f9fafb',
      },
    })

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.open(containerRef.current)
    fitAddon.fit()
    termRef.current = term

    const observer = new ResizeObserver(() => fitAddon.fit())
    observer.observe(containerRef.current)

    return () => {
      observer.disconnect()
      term.dispose()
      termRef.current = null
    }
  }, [])

  // Write log entries into xterm
  const writeEntries = useCallback((entries: LogEntry[]) => {
    const term = termRef.current
    if (!term || entries.length === 0) return

    for (const entry of entries) {
      const ts = new Date(entry.timestamp).toLocaleTimeString()
      const levelColor = entry.level === 'error' ? '\x1b[31m' : entry.level === 'warn' ? '\x1b[33m' : '\x1b[90m'
      const tag = entry.level === 'error' ? ' ERROR ' : entry.level === 'warn' ? ' WARN  ' : ''
      const tagStr = tag ? `${levelColor}${tag}\x1b[0m` : ''
      // Unescape historical log entries that have literal \u001b text instead of ESC byte
      const msg = entry.message.replace(/\\u001b/g, '\x1b')
      term.writeln(`\x1b[90m${ts}\x1b[0m ${tagStr}${msg}`)
    }
    writtenCount.current += entries.length
  }, [])

  // Initial load
  useEffect(() => {
    api.jobLogs(jobId).then(d => {
      if (d.logs && d.logs.length > 0) {
        writeEntries(d.logs)
        lastLogId.current = d.last_id
      }
    }).catch(() => {})
  }, [jobId, writeEntries])

  // Poll for new logs while job is running
  useEffect(() => {
    if (done) return
    const interval = setInterval(async () => {
      try {
        const d = await api.jobLogs(jobId, lastLogId.current)
        if (d.logs && d.logs.length > 0) {
          writeEntries(d.logs)
          lastLogId.current = d.last_id
        }
      } catch { /* ignore */ }
    }, 1500)
    return () => clearInterval(interval)
  }, [jobId, done, writeEntries])

  return (
    <div className="mt-5 bg-[#111111] border border-border rounded-lg overflow-hidden">
      <div className="flex items-center justify-between px-3 py-1.5 border-b border-border bg-bg-card">
        <span className="text-xs text-text-muted uppercase font-mono tracking-wider">Provision Log</span>
        {!done && <span className="inline-block w-2 h-2 rounded-full bg-status-warning animate-pulse-glow" />}
      </div>
      <div ref={containerRef} className="h-[400px]" />
    </div>
  )
}

export function JobView({ id }: { id: string }) {
  const [job, setJob] = useState<Job | null>(null)
  const [cancelling, setCancelling] = useState(false)
  const [cancelError, setCancelError] = useState('')

  const refreshJob = useCallback(async () => {
    try {
      const j = await api.job(id)
      setJob(j)
    } catch { /* ignore */ }
  }, [id])

  useEffect(() => {
    api.job(id).then(setJob).catch(() => {})
  }, [id])

  useEffect(() => {
    if (!job || job.state === 'completed' || job.state === 'failed' || job.state === 'cancelled') return
    const interval = setInterval(refreshJob, 1500)
    return () => clearInterval(interval)
  }, [id, job?.state, refreshJob])

  const handleCancel = async () => {
    setCancelling(true)
    setCancelError('')
    try {
      await api.cancelJob(id)
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

      <JobLogTerminal jobId={id} done={done} />
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
