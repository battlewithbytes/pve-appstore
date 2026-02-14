import type { HealthResponse } from '../types'

export function Header({ health, authed, authRequired, devMode, onLogout, onLogin }: { health: HealthResponse | null; authed: boolean; authRequired: boolean; devMode: boolean; onLogout: () => void; onLogin: () => void }) {
  return (
    <header className="bg-bg-primary border-b border-border px-6 py-3 flex items-center justify-between">
      <div className="flex items-center gap-6">
        <a href="#/" className="no-underline text-inherit flex items-center gap-3">
          <span className="text-primary text-2xl font-mono font-bold">&gt;_</span>
          <span className="text-lg font-bold text-text-primary font-mono tracking-tight">PVE App Store</span>
        </a>
        <nav className="flex gap-4">
          <a href="#/" className="text-text-secondary hover:text-primary no-underline text-sm font-mono uppercase tracking-wider transition-colors">Apps</a>
          <a href="#/installs" className="text-text-secondary hover:text-primary no-underline text-sm font-mono uppercase tracking-wider transition-colors">Installed</a>
          <a href="#/stacks" className="text-text-secondary hover:text-primary no-underline text-sm font-mono uppercase tracking-wider transition-colors">Stacks</a>
          <a href="#/catalog-stacks" className="text-text-secondary hover:text-primary no-underline text-sm font-mono uppercase tracking-wider transition-colors">Stack Templates</a>
          <a href="#/jobs" className="text-text-secondary hover:text-primary no-underline text-sm font-mono uppercase tracking-wider transition-colors">Jobs</a>
          {devMode && <a href="#/developer" className="text-yellow-400 hover:text-yellow-300 no-underline text-sm font-mono uppercase tracking-wider transition-colors">Developer</a>}
          <a href="#/backup" className="text-text-secondary hover:text-primary no-underline text-sm font-mono uppercase tracking-wider transition-colors">Backup</a>
          <a href="#/settings" className="text-text-secondary hover:text-primary no-underline text-sm font-mono uppercase tracking-wider transition-colors">Settings</a>
        </nav>
      </div>
      <div className="flex items-center gap-4 text-xs text-text-muted font-mono">
        {health && <>
          <span>node:{health.node}</span>
          <span>v{health.version}</span>
        </>}
        {authRequired && (authed ? (
          <button onClick={onLogout} className="bg-transparent border border-border rounded px-3 py-1 text-text-muted text-xs font-mono cursor-pointer hover:border-primary hover:text-primary transition-colors">logout</button>
        ) : (
          <button onClick={onLogin} className="bg-transparent border border-primary rounded px-3 py-1 text-primary text-xs font-mono cursor-pointer hover:bg-primary hover:text-bg-primary transition-colors">login</button>
        ))}
      </div>
    </header>
  )
}

export function Footer() {
  return (
    <footer className="border-t border-border px-6 py-4 mt-8">
      <div className="max-w-[1200px] mx-auto flex flex-col sm:flex-row items-center justify-between gap-2 text-xs text-text-muted font-mono">
        <span>&copy; {new Date().getFullYear()} BattleWithBytes.io</span>
        <div className="flex items-center gap-4">
          <a href="https://github.com/battlewithbytes/pve-appstore-catalog" target="_blank" rel="noreferrer" className="text-text-muted hover:text-primary transition-colors">App Catalog</a>
          <a href="https://github.com/battlewithbytes/pve-appstore" target="_blank" rel="noreferrer" className="text-text-muted hover:text-primary transition-colors">GitHub</a>
        </div>
      </div>
    </footer>
  )
}
