import { useState, useEffect } from 'react'
import { api } from '../api'
import type { Settings, SettingsUpdate, DiscoverResponse, GitHubStatus, GitHubRepoInfo, UpdateStatus } from '../types'
import { Center, InfoCard } from '../components/ui'

function RepoInfoCard() {
  const [info, setInfo] = useState<GitHubRepoInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [deleting, setDeleting] = useState<string | null>(null)

  const fetchInfo = () => {
    api.devGitHubRepoInfo()
      .then(data => { setInfo(data); setLoading(false) })
      .catch(e => { setError(e instanceof Error ? e.message : 'Failed to load'); setLoading(false) })
  }

  useEffect(() => { fetchInfo() }, [])

  const deleteBranch = (branch: string) => {
    if (!confirm(`Delete remote branch "${branch}" from your fork? This cannot be undone.`)) return
    setDeleting(branch)
    api.devGitHubDeleteBranch(branch)
      .then(() => {
        // Remove from local state immediately
        if (info) {
          const updated = { ...info, branches: (info.branches || []).filter(b => b.name !== branch) }
          setInfo(updated)
        }
      })
      .catch(e => alert(e instanceof Error ? e.message : 'Failed to delete branch'))
      .finally(() => setDeleting(null))
  }

  if (loading) return <InfoCard title="Repository Info"><p className="text-sm text-text-muted font-mono">Loading...</p></InfoCard>
  if (error) return <InfoCard title="Repository Info"><p className="text-sm text-red-400 font-mono">{error}</p></InfoCard>
  if (!info) return null

  const prDot = (state: string) => {
    if (state === 'pr_open') return <span className="inline-block w-2 h-2 rounded-full bg-green-400" />
    if (state === 'pr_merged') return <span className="inline-block w-2 h-2 rounded-full bg-purple-400" />
    if (state === 'pr_closed') return <span className="inline-block w-2 h-2 rounded-full bg-gray-500" />
    return <span className="text-text-muted">—</span>
  }

  const prLabel = (state: string) => {
    if (state === 'pr_open') return <span className="text-green-400">open</span>
    if (state === 'pr_merged') return <span className="text-purple-400">merged</span>
    if (state === 'pr_closed') return <span className="text-gray-500">closed</span>
    return null
  }

  const branches = info.branches || []

  return (
    <InfoCard title="Repository Info">
      <div className="space-y-4">
        {/* Upstream & Fork */}
        <div className="space-y-2">
          <div className="flex items-center gap-3">
            <span className="text-xs text-text-muted font-mono uppercase w-20 shrink-0">Upstream</span>
            <a href={info.upstream.url} target="_blank" rel="noreferrer" className="text-sm font-mono text-primary hover:underline truncate">{info.upstream.url.replace(/^https?:\/\//, '')}</a>
            <span className="text-xs text-text-muted font-mono">({info.upstream.branch})</span>
          </div>
          <div className="flex items-center gap-3">
            <span className="text-xs text-text-muted font-mono uppercase w-20 shrink-0">Fork</span>
            <a href={info.fork.url} target="_blank" rel="noreferrer" className="text-sm font-mono text-primary hover:underline truncate">{info.fork.full_name}</a>
          </div>
        </div>

        {/* Local Paths */}
        <div className="space-y-2">
          <h4 className="text-xs text-text-muted font-mono uppercase">Local Paths</h4>
          <div className="flex items-center gap-3">
            <span className="text-xs text-text-muted font-mono w-20 shrink-0">Catalog</span>
            <span className="text-sm font-mono text-text-secondary">{info.local.catalog_path}</span>
          </div>
          <div className="flex items-center gap-3">
            <span className="text-xs text-text-muted font-mono w-20 shrink-0">Dev Apps</span>
            <span className="text-sm font-mono text-text-secondary">{info.local.dev_apps_path}</span>
          </div>
        </div>

        {/* Branches */}
        <div className="space-y-2">
          <h4 className="text-xs text-text-muted font-mono uppercase">Branches ({branches.length})</h4>
          {branches.length === 0 ? (
            <p className="text-sm text-text-muted">No app branches yet.</p>
          ) : (
            <div className="border border-border rounded overflow-hidden">
              <table className="w-full text-sm font-mono">
                <tbody>
                  {branches.map(b => (
                    <tr key={b.name} className="border-b border-border last:border-b-0 hover:bg-white/5">
                      <td className="px-3 py-2">
                        <a href={`#/dev/${b.app_id}`} className="text-primary hover:underline">{b.name}</a>
                      </td>
                      <td className="px-3 py-2 w-24">
                        {b.pr_state ? (
                          <span className="inline-flex items-center gap-1.5">{prDot(b.pr_state)} {prLabel(b.pr_state)}</span>
                        ) : (
                          <span className="text-text-muted">—</span>
                        )}
                      </td>
                      <td className="px-3 py-2 w-24 text-right">
                        {b.pr_url && (
                          <a href={b.pr_url} target="_blank" rel="noreferrer" className="text-primary hover:underline text-xs">View PR &rarr;</a>
                        )}
                      </td>
                      <td className="px-3 py-2 w-10 text-right">
                        <button
                          onClick={() => deleteBranch(b.name)}
                          disabled={deleting === b.name}
                          className="text-red-400/60 hover:text-red-400 cursor-pointer transition-colors disabled:opacity-50"
                          title={`Delete branch ${b.name}`}
                        >
                          {deleting === b.name ? '...' : '\u00D7'}
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </InfoCard>
  )
}

// --- Settings View ---

function SettingsView({ requireAuth, onDevModeChange, onUpdateApplied, onAuthChange }: { requireAuth: (cb: () => void) => void; onDevModeChange?: (enabled: boolean) => void; onUpdateApplied?: () => void; onAuthChange?: () => void }) {
  const [settings, setSettings] = useState<Settings | null>(null)
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState('')
  const [activeTab, setActiveTab] = useState(() => {
    const hash = window.location.hash
    const tabMatch = hash.match(/[?&]tab=([^&]+)/)
    return tabMatch ? tabMatch[1] : 'general'
  })
  const [ghStatus, setGhStatus] = useState<GitHubStatus | null>(null)
  const [ghLoading, setGhLoading] = useState(false)
  const [ghToken, setGhToken] = useState('')
  const [discovered, setDiscovered] = useState<DiscoverResponse | null>(null)
  const [showStorageAdd, setShowStorageAdd] = useState(false)
  const [showBridgeAdd, setShowBridgeAdd] = useState(false)
  const [authMode, setAuthMode] = useState('')
  const [authPass, setAuthPass] = useState('')
  const [authPassConfirm, setAuthPassConfirm] = useState('')
  const [authMsg, setAuthMsg] = useState('')
  const [authSaving, setAuthSaving] = useState(false)
  const [updateStatus, setUpdateStatus] = useState<UpdateStatus | null>(null)
  const [updateChecking, setUpdateChecking] = useState(false)
  const [updateApplying, setUpdateApplying] = useState(false)
  const [updateMsg, setUpdateMsg] = useState('')

  useEffect(() => { api.settings().then(s => { setSettings(s); setAuthMode(s.auth.mode) }).catch(() => {}) }, [])

  // Fetch available storages/bridges when on general tab
  useEffect(() => {
    if (activeTab === 'general') {
      api.discoverResources().then(setDiscovered).catch(() => {})
    }
  }, [activeTab])

  // Fetch update status when on service tab
  useEffect(() => {
    if (activeTab === 'service') {
      setUpdateChecking(true)
      api.checkUpdate()
        .then(s => { setUpdateStatus(s); setUpdateChecking(false) })
        .catch(() => setUpdateChecking(false))
    }
  }, [activeTab])

  // Fetch GitHub status when on developer tab
  useEffect(() => {
    if (activeTab === 'developer' && settings?.developer.enabled) {
      api.devGitHubStatus().then(setGhStatus).catch(() => {})
    }
  }, [activeTab, settings?.developer.enabled])

  const save = (update: SettingsUpdate) => {
    requireAuth(async () => {
      setSaving(true)
      setMsg('')
      try {
        const updated = await api.updateSettings(update)
        setSettings(updated)
        onDevModeChange?.(updated.developer.enabled)
        setMsg('Settings saved')
        setTimeout(() => setMsg(''), 2000)
      } catch (e: unknown) {
        setMsg(`Error: ${e instanceof Error ? e.message : 'unknown'}`)
      }
      setSaving(false)
    })
  }

  const connectGitHub = () => {
    const token = ghToken.trim()
    if (!token) {
      setMsg('Error: Enter a GitHub Personal Access Token')
      return
    }
    requireAuth(async () => {
      setGhLoading(true)
      try {
        const result = await api.devGitHubConnect(token)
        setGhStatus({ connected: true, user: result.user, fork: result.fork })
        setGhToken('')
        setMsg('GitHub connected successfully!')
        setTimeout(() => setMsg(''), 3000)
      } catch (e: unknown) {
        setMsg(`Error: ${e instanceof Error ? e.message : 'Failed to connect'}`)
      }
      setGhLoading(false)
    })
  }

  const disconnectGitHub = () => {
    requireAuth(async () => {
      try {
        await api.devGitHubDisconnect()
        setGhStatus({ connected: false })
        setMsg('GitHub disconnected')
        setTimeout(() => setMsg(''), 2000)
      } catch (e: unknown) {
        setMsg(`Error: ${e instanceof Error ? e.message : 'Failed'}`)
      }
    })
  }

  if (!settings) return <Center className="py-16"><span className="text-text-muted font-mono">Loading settings...</span></Center>

  const tabs = [
    { id: 'general', label: 'General' },
    { id: 'developer', label: 'Developer' },
    { id: 'catalog', label: 'Catalog' },
    { id: 'gpu', label: 'GPU' },
    { id: 'service', label: 'Service' },
  ]

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-xl font-bold text-text-primary font-mono">Settings</h2>
        {msg && <span className={`text-sm font-mono ${msg.startsWith('Error') ? 'text-red-400' : 'text-primary'}`}>{msg}</span>}
      </div>

      <div className="flex gap-4">
        {/* Vertical tabs */}
        <div className="w-40 shrink-0">
          <div className="border border-border rounded-lg overflow-hidden">
            {tabs.map(tab => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={`w-full text-left px-4 py-2.5 text-sm font-mono cursor-pointer transition-colors border-b border-border last:border-b-0 ${activeTab === tab.id ? 'bg-primary/10 text-primary border-l-2 border-l-primary' : 'text-text-secondary hover:text-text-primary hover:bg-white/5'}`}
              >
                {tab.label}
              </button>
            ))}
          </div>
        </div>

        {/* Tab content */}
        <div className="flex-1 space-y-4">
          {activeTab === 'general' && (
            <>
              <InfoCard title="Default Container Resources">
                <p className="text-xs text-text-muted mb-3">Applied when installing apps that don't specify their own defaults.</p>
                <div className="grid grid-cols-3 gap-4">
                  <SettingsNumberField label="CPU Cores" value={settings.defaults.cores} onSave={(v) => save({ defaults: { cores: v } })} min={1} max={64} />
                  <SettingsNumberField label="Memory (MB)" value={settings.defaults.memory_mb} onSave={(v) => save({ defaults: { memory_mb: v } })} min={128} max={131072} step={128} />
                  <SettingsNumberField label="Disk (GB)" value={settings.defaults.disk_gb} onSave={(v) => save({ defaults: { disk_gb: v } })} min={1} max={10000} />
                </div>
              </InfoCard>

              <InfoCard title="Storages">
                <p className="text-xs text-text-muted mb-3">Proxmox storages available for container rootfs and volumes.</p>
                <div className="flex flex-wrap gap-2">
                  {settings.storages.map(s => {
                    const meta = discovered?.storages.find(d => d.id === s)
                    return (
                      <span key={s} className="inline-flex items-center gap-1.5 bg-bg-primary border border-border rounded px-2.5 py-1 text-sm font-mono text-text-primary">
                        {s}
                        {meta && <span className="text-text-muted text-xs">({meta.type})</span>}
                        <button
                          onClick={() => {
                            if (settings.storages.length <= 1) return
                            save({ storages: settings.storages.filter(x => x !== s) })
                          }}
                          disabled={settings.storages.length <= 1}
                          className={`ml-1 text-xs leading-none cursor-pointer ${settings.storages.length <= 1 ? 'text-border cursor-not-allowed' : 'text-text-muted hover:text-red-400'}`}
                          title={settings.storages.length <= 1 ? 'Cannot remove the last storage' : `Remove ${s}`}
                        >&times;</button>
                      </span>
                    )
                  })}
                  {(() => {
                    const available = discovered?.storages.filter(d => !settings.storages.includes(d.id)) || []
                    if (available.length === 0) return null
                    return showStorageAdd ? (
                      <select
                        autoFocus
                        className="bg-bg-primary border border-primary rounded px-2 py-1 text-sm font-mono text-text-primary outline-none"
                        onChange={(e) => {
                          if (e.target.value) {
                            save({ storages: [...settings.storages, e.target.value] })
                          }
                          setShowStorageAdd(false)
                        }}
                        onBlur={() => setShowStorageAdd(false)}
                        defaultValue=""
                      >
                        <option value="" disabled>Select storage...</option>
                        {available.map(d => (
                          <option key={d.id} value={d.id}>{d.id} ({d.type})</option>
                        ))}
                      </select>
                    ) : (
                      <button
                        onClick={() => setShowStorageAdd(true)}
                        className="inline-flex items-center gap-1 border border-dashed border-border rounded px-2.5 py-1 text-sm font-mono text-text-muted hover:text-primary hover:border-primary cursor-pointer transition-colors"
                      >+ Add</button>
                    )
                  })()}
                </div>
              </InfoCard>

              <InfoCard title="Network Bridges">
                <p className="text-xs text-text-muted mb-3">Network bridges assigned to new containers.</p>
                <div className="flex flex-wrap gap-2">
                  {settings.bridges.map(b => (
                    <span key={b} className="inline-flex items-center gap-1.5 bg-bg-primary border border-border rounded px-2.5 py-1 text-sm font-mono text-text-primary">
                      {b}
                      <button
                        onClick={() => {
                          if (settings.bridges.length <= 1) return
                          save({ bridges: settings.bridges.filter(x => x !== b) })
                        }}
                        disabled={settings.bridges.length <= 1}
                        className={`ml-1 text-xs leading-none cursor-pointer ${settings.bridges.length <= 1 ? 'text-border cursor-not-allowed' : 'text-text-muted hover:text-red-400'}`}
                        title={settings.bridges.length <= 1 ? 'Cannot remove the last bridge' : `Remove ${b}`}
                      >&times;</button>
                    </span>
                  ))}
                  {(() => {
                    const available = discovered?.bridges.filter(b => !settings.bridges.includes(b.name)) || []
                    if (available.length === 0) return null
                    return showBridgeAdd ? (
                      <select
                        autoFocus
                        className="bg-bg-primary border border-primary rounded px-2 py-1 text-sm font-mono text-text-primary outline-none"
                        onChange={(e) => {
                          if (e.target.value) {
                            save({ bridges: [...settings.bridges, e.target.value] })
                          }
                          setShowBridgeAdd(false)
                        }}
                        onBlur={() => setShowBridgeAdd(false)}
                        defaultValue=""
                      >
                        <option value="" disabled>Select bridge...</option>
                        {available.map(b => (
                          <option key={b.name} value={b.name}>{b.name}{b.comment ? ` — ${b.comment}` : ''}</option>
                        ))}
                      </select>
                    ) : (
                      <button
                        onClick={() => setShowBridgeAdd(true)}
                        className="inline-flex items-center gap-1 border border-dashed border-border rounded px-2.5 py-1 text-sm font-mono text-text-muted hover:text-primary hover:border-primary cursor-pointer transition-colors"
                      >+ Add</button>
                    )
                  })()}
                </div>
              </InfoCard>
            </>
          )}

          {activeTab === 'developer' && (
            <>
              <InfoCard title="Developer Mode">
                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-sm text-text-secondary">Enable developer mode to create, edit, and test custom apps.</p>
                    <p className="text-xs text-text-muted mt-1">When enabled, a Developer tab appears in the navigation bar.</p>
                  </div>
                  <button
                    onClick={() => save({ developer: { enabled: !settings.developer.enabled } })}
                    disabled={saving}
                    className={`relative w-12 h-6 rounded-full transition-colors ${settings.developer.enabled ? 'bg-primary' : 'bg-border'} cursor-pointer`}
                  >
                    <span className={`absolute top-0.5 left-0.5 w-5 h-5 rounded-full bg-white transition-transform ${settings.developer.enabled ? 'translate-x-6' : ''}`} />
                  </button>
                </div>
              </InfoCard>

              {settings.developer.enabled && (
                <>
                  <InfoCard title="GitHub Connection">
                    {ghStatus?.connected ? (
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-3">
                          {ghStatus.user?.avatar_url && (
                            <img src={ghStatus.user.avatar_url} alt="" className="w-8 h-8 rounded-full" />
                          )}
                          <div>
                            <span className="text-sm font-mono text-text-primary">{ghStatus.user?.login || 'Connected'}</span>
                            {ghStatus.user?.name && <span className="text-xs text-text-muted ml-2">({ghStatus.user.name})</span>}
                            {ghStatus.fork && (
                              <p className="text-xs text-text-muted mt-0.5">Fork: {ghStatus.fork.full_name}</p>
                            )}
                          </div>
                        </div>
                        <button
                          onClick={disconnectGitHub}
                          className="bg-transparent border border-red-500/50 rounded px-3 py-1.5 text-xs font-mono text-red-400 cursor-pointer hover:border-red-500 hover:bg-red-500/10 transition-colors"
                        >Disconnect</button>
                      </div>
                    ) : (
                      <div className="space-y-3">
                        <p className="text-sm text-text-secondary">Connect your GitHub account to submit apps via pull request.</p>
                        <div>
                          <label className="text-xs text-text-muted font-mono uppercase block mb-1">Personal Access Token</label>
                          <input
                            type="password"
                            value={ghToken}
                            onChange={(e) => setGhToken(e.target.value)}
                            placeholder="ghp_... or github_pat_..."
                            className="w-full bg-bg-primary border border-border rounded px-3 py-1.5 text-sm font-mono text-text-primary outline-none focus:border-primary"
                          />
                          <div className="mt-1.5 space-y-1.5">
                            <p className="text-xs text-text-muted">
                              <a href="https://github.com/settings/tokens/new?scopes=public_repo,read:user&description=PVE+App+Store" target="_blank" rel="noreferrer" className="text-primary hover:underline">
                                Create a classic token
                              </a>
                              {' '}with exactly these scopes:
                            </p>
                            <div className="flex gap-2">
                              <span className="text-xs font-mono bg-bg-primary border border-border rounded px-1.5 py-0.5 text-text-primary">public_repo</span>
                              <span className="text-xs font-mono bg-bg-primary border border-border rounded px-1.5 py-0.5 text-text-primary">read:user</span>
                            </div>
                            <p className="text-xs text-yellow-500/80">
                              Do not grant additional scopes. <span className="font-mono">public_repo</span> allows forking, pushing, and creating PRs on public repos only. <span className="font-mono">read:user</span> provides read-only access to your profile. Your token is encrypted (AES-256-GCM) and stored locally in the SQLite database — it is never sent anywhere except the GitHub API.
                            </p>
                          </div>
                        </div>
                        <button
                          onClick={connectGitHub}
                          disabled={ghLoading || !ghToken.trim()}
                          className="bg-primary text-bg-primary rounded px-4 py-1.5 text-xs font-mono font-bold cursor-pointer hover:opacity-90 disabled:opacity-50"
                        >{ghLoading ? 'Connecting...' : 'Connect GitHub'}</button>
                      </div>
                    )}
                  </InfoCard>

                  {ghStatus?.connected && <RepoInfoCard />}
                </>
              )}
            </>
          )}

          {activeTab === 'catalog' && (
            <InfoCard title="Catalog">
              <div className="flex items-center gap-4">
                <label className="text-xs text-text-muted font-mono uppercase w-20">Refresh</label>
                <select
                  value={settings.catalog.refresh}
                  onChange={(e) => save({ catalog: { refresh: e.target.value } })}
                  className="bg-bg-primary border border-border rounded px-3 py-1.5 text-sm font-mono text-text-primary"
                >
                  <option value="daily">Daily</option>
                  <option value="weekly">Weekly</option>
                  <option value="manual">Manual</option>
                </select>
              </div>
            </InfoCard>
          )}

          {activeTab === 'gpu' && (
            <InfoCard title="GPU Passthrough">
              <div className="flex items-center justify-between mb-3">
                <div>
                  <p className="text-sm text-text-secondary">Enable GPU passthrough for containers.</p>
                </div>
                <button
                  onClick={() => save({ gpu: { enabled: !settings.gpu.enabled, policy: settings.gpu.policy } })}
                  disabled={saving}
                  className={`relative w-12 h-6 rounded-full transition-colors ${settings.gpu.enabled ? 'bg-primary' : 'bg-border'} cursor-pointer`}
                >
                  <span className={`absolute top-0.5 left-0.5 w-5 h-5 rounded-full bg-white transition-transform ${settings.gpu.enabled ? 'translate-x-6' : ''}`} />
                </button>
              </div>
              {settings.gpu.enabled && (
                <div className="flex items-center gap-4">
                  <label className="text-xs text-text-muted font-mono uppercase w-20">Policy</label>
                  <select
                    value={settings.gpu.policy}
                    onChange={(e) => save({ gpu: { enabled: settings.gpu.enabled, policy: e.target.value } })}
                    className="bg-bg-primary border border-border rounded px-3 py-1.5 text-sm font-mono text-text-primary"
                  >
                    <option value="none">None</option>
                    <option value="allow">Allow</option>
                    <option value="allowlist">Allowlist</option>
                  </select>
                </div>
              )}
            </InfoCard>
          )}

          {activeTab === 'service' && (
            <>
              <InfoCard title="Updates">
                {updateChecking ? (
                  <p className="text-sm text-text-muted font-mono">Checking for updates...</p>
                ) : updateStatus ? (
                  <div className="space-y-3">
                    <div className="flex items-center gap-4">
                      <div>
                        <span className="text-xs text-text-muted font-mono uppercase">Current</span>
                        <p className="text-sm font-mono text-text-primary">v{updateStatus.current.replace(/^v/, '')}</p>
                      </div>
                      <div>
                        <span className="text-xs text-text-muted font-mono uppercase">Latest</span>
                        <p className="text-sm font-mono text-text-primary">v{updateStatus.latest.replace(/^v/, '')}</p>
                      </div>
                      {updateStatus.available && (
                        <span className="text-xs font-mono font-bold text-bg-primary bg-primary rounded-full px-2.5 py-0.5">
                          v{updateStatus.latest.replace(/^v/, '')} available
                        </span>
                      )}
                    </div>
                    {updateStatus.checked_at && (
                      <p className="text-xs text-text-muted font-mono">Checked: {new Date(updateStatus.checked_at).toLocaleString()}</p>
                    )}
                    {updateMsg && (
                      <p className={`text-sm font-mono ${updateMsg.startsWith('Error') ? 'text-red-400' : 'text-primary'}`}>{updateMsg}</p>
                    )}
                    {updateStatus.available ? (
                      <div className="flex items-center gap-3">
                        <button
                          onClick={() => {
                            requireAuth(async () => {
                              setUpdateApplying(true)
                              setUpdateMsg('')
                              try {
                                await api.applyUpdate()
                                setUpdateMsg('Updating... waiting for restart')
                                // Poll health endpoint to detect restart
                                const poll = setInterval(async () => {
                                  try {
                                    const h = await api.health()
                                    if (h.version !== updateStatus.current) {
                                      clearInterval(poll)
                                      setUpdateApplying(false)
                                      setUpdateMsg(`Updated to v${h.version.replace(/^v/, '')}!`)
                                      setUpdateStatus(prev => prev ? { ...prev, current: h.version, available: false, release: undefined } : null)
                                      onUpdateApplied?.()
                                    }
                                  } catch { /* server restarting */ }
                                }, 2000)
                                // Timeout after 60s
                                setTimeout(() => {
                                  clearInterval(poll)
                                  setUpdateApplying(false)
                                  if (updateMsg === 'Updating... waiting for restart') {
                                    setUpdateMsg('Update may have succeeded. Refresh the page.')
                                  }
                                }, 60000)
                              } catch (e: unknown) {
                                const msg = e instanceof Error ? e.message : 'unknown error'
                                if (msg.includes('update script') || msg.includes('self-update')) {
                                  setUpdateMsg(`Error: ${msg}`)
                                } else {
                                  setUpdateMsg(`Error: ${msg}`)
                                }
                                setUpdateApplying(false)
                              }
                            })
                          }}
                          disabled={updateApplying}
                          className="bg-primary text-bg-primary rounded px-4 py-1.5 text-xs font-mono font-bold cursor-pointer hover:opacity-90 disabled:opacity-50"
                        >
                          {updateApplying ? 'Updating...' : 'Update Now'}
                        </button>
                        {updateStatus.release?.url && (
                          <a href={updateStatus.release.url} target="_blank" rel="noreferrer" className="text-xs font-mono text-primary hover:underline">
                            Release Notes
                          </a>
                        )}
                      </div>
                    ) : (
                      <p className="text-sm text-text-muted font-mono">Running the latest version.</p>
                    )}
                  </div>
                ) : (
                  <p className="text-sm text-text-muted font-mono">Could not check for updates.</p>
                )}
              </InfoCard>

              <InfoCard title="Port">
                <div className="text-sm font-mono">
                  <span className="text-text-muted">Port:</span> <span className="text-text-primary">{settings.service.port}</span>
                </div>
                <p className="text-xs text-text-muted mt-2">To change the port:</p>
                <code className="block text-xs font-mono text-text-secondary bg-bg-primary rounded px-3 py-2 mt-1">sudo pve-appstore config set-port &lt;port&gt;</code>
              </InfoCard>

              <InfoCard title="Authentication">
                <div className="space-y-4">
                  <div className="flex items-center gap-4">
                    <label className="text-xs text-text-muted font-mono uppercase w-20">Mode</label>
                    <select
                      value={authMode}
                      onChange={(e) => { setAuthMode(e.target.value); setAuthMsg('') }}
                      className="bg-bg-primary border border-border rounded px-3 py-1.5 text-sm font-mono text-text-primary"
                    >
                      <option value="none">None</option>
                      <option value="password">Password</option>
                    </select>
                  </div>

                  {authMode === 'password' && (
                    <div className="space-y-3">
                      {settings.auth.mode === 'password' && (
                        <p className="text-xs text-text-muted">Leave password fields blank to keep the current password.</p>
                      )}
                      <div>
                        <label className="text-xs text-text-muted font-mono uppercase block mb-1">New Password</label>
                        <input
                          type="password"
                          value={authPass}
                          onChange={(e) => { setAuthPass(e.target.value); setAuthMsg('') }}
                          placeholder={settings.auth.mode === 'password' ? 'Leave blank to keep current' : 'Min 8 characters'}
                          className="w-full bg-bg-primary border border-border rounded px-3 py-1.5 text-sm font-mono text-text-primary outline-none focus:border-primary"
                        />
                      </div>
                      <div>
                        <label className="text-xs text-text-muted font-mono uppercase block mb-1">Confirm Password</label>
                        <input
                          type="password"
                          value={authPassConfirm}
                          onChange={(e) => { setAuthPassConfirm(e.target.value); setAuthMsg('') }}
                          placeholder="Repeat password"
                          className="w-full bg-bg-primary border border-border rounded px-3 py-1.5 text-sm font-mono text-text-primary outline-none focus:border-primary"
                        />
                      </div>
                    </div>
                  )}

                  {authMsg && <p className={`text-sm font-mono ${authMsg.startsWith('Error') ? 'text-red-400' : 'text-primary'}`}>{authMsg}</p>}

                  <button
                    onClick={() => {
                      // Validate
                      if (authMode === 'password') {
                        const needPassword = settings.auth.mode !== 'password' || authPass !== '' || authPassConfirm !== ''
                        if (needPassword) {
                          if (!authPass) { setAuthMsg('Error: Password is required'); return }
                          if (authPass.length < 8) { setAuthMsg('Error: Password must be at least 8 characters'); return }
                          if (authPass !== authPassConfirm) { setAuthMsg('Error: Passwords do not match'); return }
                        }
                      }

                      const update: { mode: string; password?: string } = { mode: authMode }
                      if (authMode === 'password' && authPass) {
                        update.password = authPass
                      }

                      requireAuth(async () => {
                        setAuthSaving(true)
                        setAuthMsg('')
                        try {
                          const updated = await api.updateSettings({ auth: update })
                          setSettings(updated)
                          setAuthMode(updated.auth.mode)
                          setAuthPass('')
                          setAuthPassConfirm('')
                          setAuthMsg('Authentication settings saved')
                          onAuthChange?.()
                          setTimeout(() => setAuthMsg(''), 3000)
                        } catch (e: unknown) {
                          setAuthMsg(`Error: ${e instanceof Error ? e.message : 'unknown'}`)
                        }
                        setAuthSaving(false)
                      })
                    }}
                    disabled={authSaving || (authMode === settings.auth.mode && authMode === 'none')}
                    className="bg-primary text-bg-primary rounded px-4 py-1.5 text-xs font-mono font-bold cursor-pointer hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    {authSaving ? 'Saving...' : 'Save Auth'}
                  </button>
                </div>
              </InfoCard>
            </>
          )}
        </div>
      </div>
    </div>
  )
}

function SettingsNumberField({ label, value, onSave, min, max, step }: { label: string; value: number; onSave: (v: number) => void; min?: number; max?: number; step?: number }) {
  const [val, setVal] = useState(String(value))
  const [editing, setEditing] = useState(false)

  useEffect(() => { setVal(String(value)) }, [value])

  const commit = () => {
    const n = parseInt(val, 10)
    if (!isNaN(n) && n !== value) onSave(n)
    setEditing(false)
  }

  return (
    <div>
      <label className="text-xs text-text-muted font-mono uppercase">{label}</label>
      {editing ? (
        <input
          type="number"
          value={val}
          onChange={(e) => setVal(e.target.value)}
          onBlur={commit}
          onKeyDown={(e) => e.key === 'Enter' && commit()}
          min={min}
          max={max}
          step={step || 1}
          className="w-full bg-bg-primary border border-primary rounded px-3 py-1.5 text-sm font-mono text-text-primary mt-1"
          autoFocus
        />
      ) : (
        <div
          onClick={() => setEditing(true)}
          className="w-full border border-border rounded px-3 py-1.5 text-sm font-mono text-text-primary mt-1 cursor-pointer hover:border-primary transition-colors"
        >{value}</div>
      )}
    </div>
  )
}

export { SettingsView, SettingsNumberField, RepoInfoCard }
