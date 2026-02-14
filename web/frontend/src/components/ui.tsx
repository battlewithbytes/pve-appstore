import React from 'react'

export function Center({ children, className }: { children: React.ReactNode; className?: string }) {
  return <div className={`text-center py-12 text-text-muted ${className || ''}`}>{children}</div>
}

export function BackLink({ href = '#/', label = 'Back to apps' }: { href?: string; label?: string }) {
  return <a href={href} className="text-primary text-sm no-underline font-mono hover:underline">&larr; {label}</a>
}

function CornerRibbon({ label, color, index = 0 }: { label: string; color: string; index?: number }) {
  const offset = index * 20
  return (
    <div className="absolute top-0 right-0 w-28 h-28 overflow-hidden pointer-events-none rounded-tr-lg">
      <div className={`${color} text-black absolute right-[-30px] w-[130px] text-center text-[9px] font-mono font-bold uppercase tracking-wider py-[5px] rotate-45 shadow-sm leading-none`} style={{ top: 22 + offset }}>
        {label}
      </div>
    </div>
  )
}

export function RibbonStack({ ribbons }: { ribbons: { label: string; color: string }[] }) {
  if (ribbons.length === 0) return null
  const sorted = [...ribbons].sort((a, b) => a.label.length - b.label.length)
  return <>{sorted.map((r, i) => <CornerRibbon key={r.label} label={r.label} color={r.color} index={i} />)}</>
}

export function Badge({ children, className }: { children: React.ReactNode; className?: string }) {
  return <span className={`text-[11px] px-2 py-0.5 rounded font-mono ${className || ''}`}>{children}</span>
}

export function StatusDot({ running }: { running: boolean }) {
  return <span className={`inline-block w-2.5 h-2.5 rounded-full ${running ? 'bg-status-running animate-pulse-glow text-status-running' : 'bg-status-stopped'}`} />
}

export function StateBadge({ state }: { state: string }) {
  const cls = state === 'completed' ? 'bg-status-running/10 text-status-running' : state === 'failed' ? 'bg-status-stopped/10 text-status-stopped' : state === 'cancelled' ? 'bg-text-muted/10 text-text-muted' : 'bg-status-warning/10 text-status-warning'
  return <span className={`text-xs px-2.5 py-1 rounded font-mono font-semibold ${cls}`}>{state}</span>
}

export function ResourceCard({ label, value, sub, pct }: { label: string; value: string; sub?: string; pct?: number }) {
  return (
    <div className="bg-bg-card border border-border rounded-lg p-4">
      <div className="text-xs text-text-muted uppercase font-mono tracking-wider mb-2">{label}</div>
      <div className="text-xl font-bold text-text-primary font-mono">{value}</div>
      {sub && <div className="text-xs text-text-muted font-mono mt-1">/ {sub}</div>}
      {pct !== undefined && (
        <div className="mt-2 h-1.5 bg-bg-secondary rounded-full overflow-hidden">
          <div className={`h-full rounded-full transition-all duration-500 ${pct > 80 ? 'bg-status-stopped' : pct > 50 ? 'bg-status-warning' : 'bg-primary'}`} style={{ width: `${Math.min(pct, 100)}%` }} />
        </div>
      )}
    </div>
  )
}

export function InfoCard({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-bg-card border border-border rounded-lg p-5">
      <h3 className="text-xs font-semibold text-text-muted mb-3 uppercase tracking-wider font-mono">{title}</h3>
      {children}
    </div>
  )
}

export function Linkify({ text }: { text: string }) {
  const urlRegex = /(https?:\/\/[^\s<>"']+)/g
  const parts = text.split(urlRegex)
  if (parts.length === 1) return <>{text}</>
  return <>{parts.map((part, i) => urlRegex.test(part)
    ? <a key={i} href={part} target="_blank" rel="noreferrer" className="text-primary hover:underline">{part}</a>
    : part
  )}</>
}

export function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between py-1 text-sm">
      <span className="text-text-muted">{label}</span>
      <span className="text-text-secondary text-right break-all"><Linkify text={value} /></span>
    </div>
  )
}

export function SectionTitle({ children }: { children: React.ReactNode }) {
  return <h4 className="text-xs text-primary uppercase mt-5 mb-2 tracking-wider font-mono">{children}</h4>
}

export function FormRow({ label, help, description, children }: { label: string; help?: string; description?: string; children: React.ReactNode }) {
  return (
    <div className="mb-3">
      <label className="text-sm text-text-secondary block mb-1">{label}</label>
      {description && <div className="text-xs text-text-muted mb-1.5 leading-relaxed">{description}</div>}
      {children}
      {help && <div className="text-[11px] text-text-muted mt-0.5 italic">{help}</div>}
    </div>
  )
}

export function FormInput({ value, onChange, type = 'text', placeholder }: { value: string; onChange: (v: string) => void; type?: string; placeholder?: string }) {
  return <input type={type} value={value} onChange={e => onChange(e.target.value)} placeholder={placeholder} className="w-full px-3 py-2 bg-bg-secondary border border-border rounded-md text-text-primary text-sm outline-none focus:border-primary focus:ring-1 focus:ring-primary transition-colors font-mono placeholder:text-text-muted" />
}

export function FormField({ label, description, help, children }: { label: string; description?: string; help?: string; children: React.ReactNode }) {
  return (
    <div className="mb-3">
      <label className="text-xs text-text-muted font-mono mb-1 block">{label}</label>
      {description && <div className="text-[10px] text-text-muted mb-1">{description}</div>}
      {children}
      {help && <div className="text-[10px] text-text-muted mt-1">{help}</div>}
    </div>
  )
}

export function ActionButton({ label, onClick, accent, danger }: { label: string; onClick: () => void; accent?: boolean; danger?: boolean }) {
  const cls = danger
    ? 'border-status-error text-status-error hover:bg-status-error hover:text-bg-primary'
    : accent
    ? 'border-primary text-primary hover:bg-primary hover:text-bg-primary'
    : 'border-border text-text-muted hover:border-primary hover:text-primary'
  return (
    <button onClick={onClick} className={`px-3 py-1.5 bg-transparent border rounded text-xs font-mono cursor-pointer transition-colors ${cls}`}>
      {label}
    </button>
  )
}

export function CtxMenuItem({ label, onClick, danger }: { label: string; onClick: () => void; danger?: boolean }) {
  return (
    <button onClick={onClick}
      className={`w-full text-left px-4 py-2 text-sm font-mono bg-transparent border-none cursor-pointer transition-colors ${danger ? 'text-status-stopped hover:bg-status-stopped/10' : 'text-text-secondary hover:bg-bg-secondary hover:text-text-primary'}`}>
      {label}
    </button>
  )
}

export function DevStatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    draft: 'border-text-muted text-text-muted',
    validated: 'border-blue-400 text-blue-400',
    deployed: 'border-primary text-primary',
  }
  return <span className={`text-xs font-mono px-2 py-0.5 border rounded ${colors[status] || colors.draft}`}>{status}</span>
}
