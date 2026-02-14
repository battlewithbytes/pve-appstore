import { useState, useEffect, useRef } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'
import { api } from '../api'
import type { ContainerLiveStatus } from '../types'
import { formatBytesShort } from '../lib/format'

export function TerminalModal({ installId, onClose }: { installId: string; onClose: () => void }) {
  const termRef = useRef<HTMLDivElement>(null)
  const termInstance = useRef<Terminal | null>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const [info, setInfo] = useState<{ name: string; ip: string; live?: ContainerLiveStatus } | null>(null)

  // Poll install detail for status bar
  useEffect(() => {
    let alive = true
    const poll = () => {
      api.installDetail(installId).then(d => {
        if (!alive) return
        setInfo({ name: d.app_name, ip: d.ip || '', live: d.live })
      }).catch(() => {})
    }
    poll()
    const iv = setInterval(poll, 5000)
    return () => { alive = false; clearInterval(iv) }
  }, [installId])

  useEffect(() => {
    if (!termRef.current) return

    const term = new Terminal({
      cursorBlink: true,
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 14,
      theme: {
        background: '#0A0A0A',
        foreground: '#00FF9D',
        cursor: '#00FF9D',
        selectionBackground: 'rgba(0,255,157,0.3)',
      },
    })
    termInstance.current = term

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.loadAddon(new WebLinksAddon())

    term.open(termRef.current)
    fitAddon.fit()

    // Fetch a short-lived terminal token, then connect WebSocket
    let ws: WebSocket | null = null
    let cancelled = false

    api.terminalToken().then(({ token }) => {
      if (cancelled) return

      const wsUrl = api.terminalUrl(installId, token)
      ws = new WebSocket(wsUrl)
      wsRef.current = ws

      ws.binaryType = 'arraybuffer'

      ws.onopen = () => {
        term.writeln('\x1b[32mConnected to container shell.\x1b[0m\r\n')
        ws!.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
      }

      ws.onmessage = (event) => {
        if (event.data instanceof ArrayBuffer) {
          term.write(new Uint8Array(event.data))
        } else {
          term.write(event.data)
        }
      }

      ws.onclose = () => {
        term.writeln('\r\n\x1b[31mConnection closed.\x1b[0m')
      }

      ws.onerror = () => {
        term.writeln('\r\n\x1b[31mWebSocket error.\x1b[0m')
      }

      term.onData((data) => {
        if (ws && ws.readyState === WebSocket.OPEN) {
          ws.send(data)
        }
      })
    }).catch(() => {
      term.writeln('\x1b[31mFailed to get terminal token. Are you logged in?\x1b[0m')
    })

    // Handle resize — use ResizeObserver on the container for reliable fitting
    const handleResize = () => {
      fitAddon.fit()
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
      }
    }
    const observer = new ResizeObserver(() => handleResize())
    observer.observe(termRef.current)
    window.addEventListener('resize', handleResize)

    return () => {
      cancelled = true
      observer.disconnect()
      window.removeEventListener('resize', handleResize)
      if (ws) ws.close()
      term.dispose()
    }
  }, [installId])

  const live = info?.live
  const cpuPct = live ? live.cpu * 100 : 0
  const memPct = live && live.maxmem > 0 ? (live.mem / live.maxmem) * 100 : 0

  return (
    <div className="fixed inset-0 bg-black/95 flex flex-col z-[200]">
      <div className="grid grid-cols-[1fr_auto_1fr] items-center px-4 py-2 bg-bg-card border-b border-border gap-4">
        <div className="flex items-center gap-3 min-w-0">
          <span className="text-primary font-mono text-sm whitespace-nowrap">&gt;_ {info?.name || 'Terminal'}</span>
          {info?.ip && <span className="text-text-muted font-mono text-xs">{info.ip}</span>}
        </div>
        <div className="flex items-center gap-4 text-xs font-mono">
          {live ? (
            <>
              <TerminalMiniGauge label="CPU" value={`${cpuPct.toFixed(0)}%`} pct={cpuPct} />
              <TerminalMiniGauge label="MEM" value={`${memPct.toFixed(0)}%`} pct={memPct} />
              <span className="text-text-muted">{formatBytesShort(live.netin)}&darr; {formatBytesShort(live.netout)}&uarr;</span>
            </>
          ) : (
            <span className="text-text-muted">loading...</span>
          )}
        </div>
        <div className="flex justify-end">
          <button onClick={onClose} className="text-text-muted hover:text-text-primary bg-transparent border-none cursor-pointer text-lg font-mono">&times;</button>
        </div>
      </div>
      <div ref={termRef} className="flex-1 min-h-0 overflow-hidden p-2" />
    </div>
  )
}

export function TerminalMiniGauge({ label, value, pct }: { label: string; value: string; pct: number }) {
  const color = pct > 90 ? '#ff4444' : pct > 70 ? '#ffaa00' : '#00ff9d'
  return (
    <div className="flex items-center gap-1.5">
      <span className="text-text-muted">{label}</span>
      <div className="w-12 h-1.5 bg-[#222] rounded-full overflow-hidden">
        <div className="h-full rounded-full transition-all duration-500" style={{ width: `${Math.min(pct, 100)}%`, backgroundColor: color }} />
      </div>
      <span style={{ color }}>{value}</span>
    </div>
  )
}

export function MiniBar({ label, pct }: { label: string; pct: number }) {
  const color = pct > 90 ? '#ff4444' : pct > 70 ? '#ffaa00' : '#00ff9d'
  return (
    <div className="flex items-center gap-1" title={`${label}: ${pct.toFixed(0)}%`}>
      <span className="text-[9px] text-text-muted w-6">{label}</span>
      <div className="w-8 h-1 bg-[#222] rounded-full overflow-hidden">
        <div className="h-full rounded-full" style={{ width: `${Math.min(pct, 100)}%`, backgroundColor: color }} />
      </div>
    </div>
  )
}

export function LogViewerModal({ installId, onClose }: { installId: string; onClose: () => void }) {
  const termRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!termRef.current) return

    const term = new Terminal({
      cursorBlink: false,
      disableStdin: true,
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 13,
      theme: {
        background: '#0A0A0A',
        foreground: '#9CA3AF',
        cursor: 'transparent',
        selectionBackground: 'rgba(0,255,157,0.3)',
      },
    })

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.loadAddon(new WebLinksAddon())

    term.open(termRef.current)
    fitAddon.fit()

    let ws: WebSocket | null = null
    let cancelled = false

    api.terminalToken().then(({ token }) => {
      if (cancelled) return

      const wsUrl = api.journalLogsUrl(installId, token)
      ws = new WebSocket(wsUrl)

      ws.onopen = () => {
        term.writeln('\x1b[32m--- Journal log stream started ---\x1b[0m\r')
      }

      ws.onmessage = (event) => {
        const text = typeof event.data === 'string' ? event.data : new TextDecoder().decode(event.data)
        for (const line of text.split('\n')) {
          if (line) term.writeln(line)
        }
      }

      ws.onclose = () => {
        term.writeln('\r\n\x1b[31m--- Log stream ended ---\x1b[0m')
      }

      ws.onerror = () => {
        term.writeln('\r\n\x1b[31mWebSocket error.\x1b[0m')
      }
    }).catch(() => {
      term.writeln('\x1b[31mFailed to get log token. Are you logged in?\x1b[0m')
    })

    const handleResize = () => fitAddon.fit()
    window.addEventListener('resize', handleResize)

    return () => {
      cancelled = true
      window.removeEventListener('resize', handleResize)
      if (ws) ws.close()
      term.dispose()
    }
  }, [installId])

  return (
    <div className="fixed inset-0 bg-black/95 flex flex-col z-[200]">
      <div className="flex items-center justify-between px-4 py-2 bg-bg-card border-b border-border">
        <span className="text-text-secondary font-mono text-sm">journalctl &mdash; {installId}</span>
        <button onClick={onClose} className="text-text-muted hover:text-text-primary bg-transparent border-none cursor-pointer text-lg font-mono">&times;</button>
      </div>
      <div ref={termRef} className="flex-1 p-2" />
    </div>
  )
}

export function StackTerminalModal({ stackId, onClose }: { stackId: string; onClose: () => void }) {
  const termRef = useRef<HTMLDivElement>(null)
  const termInstance = useRef<Terminal | null>(null)
  const wsRef = useRef<WebSocket | null>(null)

  useEffect(() => {
    if (!termRef.current) return

    const term = new Terminal({
      cursorBlink: true,
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 14,
      theme: { background: '#0A0A0A', foreground: '#00FF9D', cursor: '#00FF9D', selectionBackground: 'rgba(0,255,157,0.3)' },
    })
    termInstance.current = term

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.loadAddon(new WebLinksAddon())
    term.open(termRef.current)
    fitAddon.fit()

    let ws: WebSocket | null = null
    let cancelled = false

    api.terminalToken().then(({ token }) => {
      if (cancelled) return
      const wsUrl = api.stackTerminalUrl(stackId, token)
      ws = new WebSocket(wsUrl)
      wsRef.current = ws
      ws.binaryType = 'arraybuffer'

      ws.onopen = () => {
        term.writeln('\x1b[32mConnected to stack container shell.\x1b[0m\r\n')
        ws!.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }))
      }
      ws.onmessage = (event) => {
        if (event.data instanceof ArrayBuffer) term.write(new Uint8Array(event.data))
        else term.write(event.data)
      }
      ws.onclose = () => { term.writeln('\r\n\x1b[31mConnection closed.\x1b[0m') }
      ws.onerror = () => { term.writeln('\r\n\x1b[31mWebSocket error.\x1b[0m') }

      term.onData(data => { if (ws?.readyState === WebSocket.OPEN) ws.send(data) })
      term.onResize(size => { if (ws?.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ type: 'resize', cols: size.cols, rows: size.rows })) })
    }).catch(() => { term.writeln('\x1b[31mFailed to get terminal token.\x1b[0m') })

    const observer = new ResizeObserver(() => fitAddon.fit())
    observer.observe(termRef.current)

    return () => {
      cancelled = true
      observer.disconnect()
      ws?.close()
      term.dispose()
    }
  }, [stackId])

  return (
    <div className="fixed inset-0 bg-black/80 flex items-center justify-center z-[200] p-4" onClick={onClose}>
      <div className="bg-bg-card border border-border rounded-lg w-full max-w-4xl h-[70vh] flex flex-col" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-4 py-2 border-b border-border">
          <span className="text-sm font-mono text-text-primary">Stack Shell</span>
          <button onClick={onClose} className="text-text-muted hover:text-primary bg-transparent border-none cursor-pointer text-lg font-mono">&times;</button>
        </div>
        <div ref={termRef} className="flex-1 p-2" />
      </div>
    </div>
  )
}

export function StackLogViewerModal({ stackId, onClose }: { stackId: string; onClose: () => void }) {
  const termRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!termRef.current) return

    const term = new Terminal({
      cursorBlink: false, disableStdin: true,
      fontFamily: "'JetBrains Mono', monospace", fontSize: 13,
      theme: { background: '#0A0A0A', foreground: '#9CA3AF', cursor: 'transparent', selectionBackground: 'rgba(0,255,157,0.3)' },
    })

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.loadAddon(new WebLinksAddon())
    term.open(termRef.current)
    fitAddon.fit()

    let ws: WebSocket | null = null
    let cancelled = false

    api.terminalToken().then(({ token }) => {
      if (cancelled) return
      const wsUrl = api.stackJournalLogsUrl(stackId, token)
      ws = new WebSocket(wsUrl)
      ws.onopen = () => { term.writeln('\x1b[32m--- Journal log stream started ---\x1b[0m\r') }
      ws.onmessage = (event) => {
        const lines = (event.data as string).split('\n')
        for (const line of lines) {
          if (line.trim()) term.writeln(line)
        }
      }
      ws.onclose = () => { term.writeln('\r\n\x1b[31m--- Stream ended ---\x1b[0m') }
      ws.onerror = () => { term.writeln('\r\n\x1b[31mWebSocket error.\x1b[0m') }
    }).catch(() => { term.writeln('\x1b[31mFailed to get terminal token.\x1b[0m') })

    const observer = new ResizeObserver(() => fitAddon.fit())
    observer.observe(termRef.current)

    return () => {
      cancelled = true
      observer.disconnect()
      ws?.close()
      term.dispose()
    }
  }, [stackId])

  return (
    <div className="fixed inset-0 bg-black/80 flex items-center justify-center z-[200] p-4" onClick={onClose}>
      <div className="bg-bg-card border border-border rounded-lg w-full max-w-4xl h-[70vh] flex flex-col" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-4 py-2 border-b border-border">
          <span className="text-sm font-mono text-text-primary">Stack Logs (journalctl)</span>
          <button onClick={onClose} className="text-text-muted hover:text-primary bg-transparent border-none cursor-pointer text-lg font-mono">&times;</button>
        </div>
        <div ref={termRef} className="flex-1 p-2" />
      </div>
    </div>
  )
}
