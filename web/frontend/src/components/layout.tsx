import type { HealthResponse } from '../types'

function NavLink({ href, hash, label, isDev }: { href: string; hash: string; label: string; isDev?: boolean }) {
  // Match active: exact match, or prefix match for sub-routes (e.g. #/app/foo matches #/)
  const isActive = href === '#/'
    ? (hash === '#/' || hash === '' || hash === '#' || hash.startsWith('#/app/'))
    : hash === href || hash.startsWith(href + '/')
  const base = 'no-underline text-sm font-mono uppercase tracking-wider transition-colors'
  const color = isActive
    ? (isDev ? 'text-yellow-300 border-b-2 border-yellow-400 pb-0.5' : 'text-primary border-b-2 border-primary pb-0.5')
    : (isDev ? 'text-yellow-400/60 hover:text-yellow-300' : 'text-text-secondary hover:text-primary')
  return <a href={href} className={`${base} ${color}`}>{label}</a>
}

export function Header({ health, authed, authRequired, devMode, hash, onLogout, onLogin, updateAvailable }: { health: HealthResponse | null; authed: boolean; authRequired: boolean; devMode: boolean; hash: string; onLogout: () => void; onLogin: () => void; updateAvailable?: boolean }) {
  return (
    <header className="bg-bg-primary border-b border-border px-6 py-3 flex items-center justify-between">
      <div className="flex items-center gap-6">
        <a href="#/" className="no-underline text-inherit flex items-center gap-3">
          <span className="text-primary text-2xl font-mono font-bold">&gt;_</span>
          <span className="text-lg font-bold text-text-primary font-mono tracking-tight">PVE App Store</span>
        </a>
        <nav className="flex gap-4 items-center">
          <NavLink href="#/" hash={hash} label="Apps" />
          <NavLink href="#/installs" hash={hash} label="Installed" />
          <NavLink href="#/stacks" hash={hash} label="Stacks" />
          <NavLink href="#/catalog-stacks" hash={hash} label="Stack Templates" />
          <NavLink href="#/jobs" hash={hash} label="Jobs" />
          {devMode && <NavLink href="#/developer" hash={hash} label="Developer" isDev />}
          <NavLink href="#/backup" hash={hash} label="Backup" />
          <NavLink href="#/settings" hash={hash} label="Settings" />
        </nav>
      </div>
      <div className="flex items-center gap-4 text-xs text-text-muted font-mono">
        {health && <>
          <span>node:{health.node}</span>
          <span className="inline-flex items-center gap-1.5">
            v{health.version}
            {updateAvailable && (
              <a href="#/settings" className="relative flex h-2 w-2" title="Update available">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-primary opacity-75" />
                <span className="relative inline-flex rounded-full h-2 w-2 bg-primary" />
              </a>
            )}
          </span>
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
