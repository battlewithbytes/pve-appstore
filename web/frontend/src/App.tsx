import { useState, useEffect, useCallback } from 'react'
import { api } from './api'
import { useHash } from './hooks/useHash'
import { Header, Footer } from './components/layout'
import { LoginModal, LoginForm } from './components/LoginModal'
import type { HealthResponse } from './types'

// Views
import { AppList, AppDetailView } from './views/apps'
import { InstallsList } from './views/installs'
import { InstallDetailView } from './views/install-detail'
import { JobView, JobsList } from './views/jobs'
import { ConfigView } from './views/config'
import { SettingsView } from './views/settings'
import { StacksList } from './views/stacks'
import { StackDetailView } from './views/stack-detail'
import { StackCreateWizard } from './views/stack-create'
import { CatalogStacksList, CatalogStackDetailView } from './views/catalog-stacks'
import { DeveloperDashboard } from './views/developer'
import { DevStackEditor } from './views/dev-stack-editor'
import { DevAppEditor } from './DevAppEditor'

function App() {
  const hash = useHash()
  const [health, setHealth] = useState<HealthResponse | null>(null)
  const [authed, setAuthed] = useState(false)
  const [authRequired, setAuthRequired] = useState(false)
  const [authChecked, setAuthChecked] = useState(false)
  const [showLogin, setShowLogin] = useState(false)
  const [loginCallback, setLoginCallback] = useState<(() => void) | null>(null)
  const [devMode, setDevMode] = useState(false)

  useEffect(() => { api.health().then(setHealth).catch(() => {}) }, [])
  useEffect(() => { api.settings().then(s => setDevMode(s.developer.enabled)).catch(() => {}) }, [])
  useEffect(() => {
    api.authCheck().then(d => {
      setAuthed(d.authenticated)
      setAuthRequired(d.auth_required)
      setAuthChecked(true)
    }).catch(() => { setAuthChecked(true) })
  }, [])

  const requireAuth = useCallback((onSuccess: () => void) => {
    if (authed || !authRequired) { onSuccess(); return }
    setLoginCallback(() => onSuccess)
    setShowLogin(true)
  }, [authed, authRequired])

  const handleLoginSuccess = useCallback(() => {
    setAuthed(true)
    setShowLogin(false)
    if (loginCallback) { loginCallback(); setLoginCallback(null) }
  }, [loginCallback])

  const handleLogout = useCallback(async () => {
    await api.logout().catch(() => {})
    setAuthed(false)
  }, [])

  const appMatch = hash.match(/^#\/app\/(.+)$/)
  const jobMatch = hash.match(/^#\/job\/(.+)$/)
  const installMatch = hash.match(/^#\/install\/(.+)$/)
  const stackMatch = hash.match(/^#\/stack\/(.+)$/)
  const isInstalls = hash === '#/installs'
  const isStacks = hash === '#/stacks'
  const isCreateStack = hash === '#/create-stack'
  const isJobs = hash === '#/jobs'
  const isConfig = hash === '#/backup'
  const isSettings = hash === '#/settings'
  const isDeveloper = hash === '#/developer' || hash.startsWith('#/dev/')
  const devAppMatch = hash.match(/^#\/dev\/(.+)$/)
  const devStackMatch = hash.match(/^#\/dev\/stack\/(.+)$/)
  const isCatalogStacks = hash === '#/catalog-stacks'
  const catalogStackMatch = hash.match(/^#\/catalog-stack\/(.+)$/)

  let content
  if (jobMatch) content = <JobView id={jobMatch[1]} />
  else if (installMatch) content = <InstallDetailView id={installMatch[1]} requireAuth={requireAuth} />
  else if (stackMatch) content = <StackDetailView id={stackMatch[1]} requireAuth={requireAuth} />
  else if (appMatch) content = <AppDetailView id={appMatch[1]} requireAuth={requireAuth} devMode={devMode} />
  else if (isInstalls) content = <InstallsList requireAuth={requireAuth} />
  else if (isStacks) content = <StacksList requireAuth={requireAuth} />
  else if (isCreateStack) content = <StackCreateWizard requireAuth={requireAuth} />
  else if (isJobs) content = <JobsList />
  else if (isConfig) content = <ConfigView requireAuth={requireAuth} />
  else if (isSettings) content = <SettingsView requireAuth={requireAuth} onDevModeChange={setDevMode} onAuthChange={() => {
    api.authCheck().then(d => {
      setAuthed(d.authenticated)
      setAuthRequired(d.auth_required)
    }).catch(() => {})
  }} />
  else if (catalogStackMatch) content = <CatalogStackDetailView id={catalogStackMatch[1]} requireAuth={requireAuth} />
  else if (isCatalogStacks) content = <CatalogStacksList requireAuth={requireAuth} />
  else if (devStackMatch) content = <DevStackEditor id={devStackMatch[1]} requireAuth={requireAuth} />
  else if (devAppMatch) content = <DevAppEditor id={devAppMatch[1]} requireAuth={requireAuth} />
  else if (isDeveloper) content = <DeveloperDashboard requireAuth={requireAuth} />
  else content = <AppList />

  // Gate: if auth is required and user isn't logged in, show full-page login
  if (authChecked && authRequired && !authed) {
    return (
      <div className="min-h-screen flex flex-col bg-bg-primary">
        <Header health={health} authed={authed} authRequired={authRequired} devMode={devMode} hash={hash} onLogout={handleLogout} onLogin={() => {}} />
        <main className="flex-1 flex items-center justify-center px-4">
          <div className="w-full max-w-[400px]">
            <div className="text-center mb-8">
              <span className="text-primary text-4xl font-mono font-bold">&gt;_</span>
              <h1 className="text-xl font-bold text-text-primary font-mono mt-2">PVE App Store</h1>
              <p className="text-sm text-text-muted mt-1">Sign in to continue</p>
            </div>
            <div className="bg-bg-card border border-border rounded-xl p-8">
              <LoginForm onSuccess={() => {
                setAuthed(true)
                api.settings().then(s => setDevMode(s.developer.enabled)).catch(() => {})
              }} />
            </div>
          </div>
        </main>
        <Footer />
      </div>
    )
  }

  return (
    <div className="min-h-screen flex flex-col bg-bg-primary">
      <Header health={health} authed={authed} authRequired={authRequired} devMode={devMode} hash={hash} onLogout={handleLogout} onLogin={() => setShowLogin(true)} />
      <main className={`flex-1 mx-auto px-4 py-6 w-full ${devAppMatch || devStackMatch ? 'max-w-[1800px]' : 'max-w-[1200px]'}`}>
        {content}
      </main>
      <Footer />
      {showLogin && <LoginModal onSuccess={handleLoginSuccess} onClose={() => { setShowLogin(false); setLoginCallback(null) }} />}
    </div>
  )
}

export default App
