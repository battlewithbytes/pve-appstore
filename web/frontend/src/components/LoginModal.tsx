import { useState } from 'react'
import { api } from '../api'
import type { AppDetail } from '../types'
import { FormInput } from './ui'

export function LoginModal({ onSuccess, onClose }: { onSuccess: () => void; onClose: () => void }) {
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true); setError('')
    try {
      await api.login(password)
      onSuccess()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Login failed')
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-[200]">
      <form onSubmit={handleSubmit} className="bg-bg-card border border-border rounded-xl p-8 w-full max-w-[380px]">
        <h2 className="text-lg font-bold text-text-primary mb-2 font-mono">Login Required</h2>
        <p className="text-sm text-text-muted mb-5">Enter your password to perform this action.</p>
        <FormInput value={password} onChange={setPassword} type="password" />
        {error && <div className="text-status-stopped text-sm mt-2 font-mono">{error}</div>}
        <div className="flex gap-3 mt-5 justify-end">
          <button type="button" onClick={onClose} className="px-5 py-2.5 text-sm font-semibold border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-text-secondary transition-colors font-mono">Cancel</button>
          <button type="submit" disabled={loading || !password} className="px-5 py-2.5 text-sm font-semibold border-none rounded-lg cursor-pointer bg-primary text-bg-primary hover:shadow-[0_0_20px_rgba(0,255,157,0.3)] transition-all disabled:opacity-50 font-mono">
            {loading ? 'Logging in...' : 'Login'}
          </button>
        </div>
      </form>
    </div>
  )
}

export function TestInstallModal({ app, ctid, onConfirm, onClose }: { app: AppDetail; ctid?: number; onConfirm: (keepVolumes: string[]) => void; onClose: () => void }) {
  const bindVolumes = (app.volumes || []).filter(v => v.type === 'bind')
  const managedVolumes = (app.volumes || []).filter(v => (v.type || 'volume') === 'volume')

  const [kept, setKept] = useState<Record<string, boolean>>(() => {
    const init: Record<string, boolean> = {}
    for (const v of managedVolumes) init[v.name] = true
    return init
  })
  const toggleVolume = (name: string) => setKept(prev => ({ ...prev, [name]: !prev[name] }))
  const allChecked = managedVolumes.every(v => kept[v.name])
  const noneChecked = managedVolumes.every(v => !kept[v.name])
  const toggleAll = () => {
    const newVal = !allChecked
    const next: Record<string, boolean> = {}
    for (const v of managedVolumes) next[v.name] = newVal
    setKept(next)
  }

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-[200]" onClick={onClose}>
      <div className="bg-bg-card border border-border rounded-xl p-8 w-full max-w-[520px]" onClick={e => e.stopPropagation()}>
        <h2 className="text-lg font-bold text-yellow-400 mb-1 font-mono flex items-center gap-2">
          <span className="text-xl">&#9888;</span> Test Install
        </h2>
        <p className="text-sm text-text-muted mb-5">
          This will replace the existing install{ctid ? ` (CT ${ctid})` : ''} with a fresh container using your dev version of <span className="text-text-primary font-semibold">{app.name}</span>.
        </p>

        <div className="space-y-3 mb-6">
          <div className="flex items-start gap-3 p-3 rounded-lg bg-red-500/5 border border-red-500/20">
            <span className="text-red-400 text-lg mt-0.5">&#10005;</span>
            <div>
              <div className="text-sm font-semibold text-red-400 mb-0.5">Container destroyed</div>
              <div className="text-xs text-text-muted">The existing container, OS, installed packages, and all config files (e.g. <span className="font-mono">/etc</span>) will be destroyed.</div>
            </div>
          </div>

          {managedVolumes.length > 0 && (
            <div className="p-3 rounded-lg border border-border">
              <div className="flex items-center justify-between mb-2">
                <div className="text-sm font-semibold text-text-primary">Managed volumes</div>
                <button onClick={toggleAll} className="text-xs text-text-muted hover:text-text-secondary cursor-pointer bg-transparent border-none font-mono">
                  {allChecked ? 'Uncheck all' : 'Check all'}
                </button>
              </div>
              <div className="space-y-1.5">
                {managedVolumes.map(v => (
                  <label key={v.name} className="flex items-center gap-2.5 p-2 rounded-lg cursor-pointer hover:bg-bg-secondary/50 transition-colors">
                    <input
                      type="checkbox"
                      checked={!!kept[v.name]}
                      onChange={() => toggleVolume(v.name)}
                      className="w-4 h-4 accent-primary cursor-pointer"
                    />
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-mono text-text-primary">{v.name}</span>
                        <span className="text-xs text-text-muted font-mono">{v.mount_path}</span>
                      </div>
                      {v.description && <div className="text-xs text-text-muted mt-0.5">{v.description}</div>}
                    </div>
                    <span className={`text-xs font-mono px-1.5 py-0.5 rounded ${kept[v.name] ? 'text-primary bg-primary/10' : 'text-red-400 bg-red-500/10'}`}>
                      {kept[v.name] ? 'keep' : 'wipe'}
                    </span>
                  </label>
                ))}
              </div>
              <div className="text-xs text-text-muted mt-2">
                {noneChecked
                  ? 'All volumes will be destroyed and recreated fresh — verifies a clean first-time install.'
                  : 'Checked volumes will be reattached to the new container. Unchecked volumes will be destroyed and recreated.'}
              </div>
            </div>
          )}

          {bindVolumes.length > 0 && (
            <div className="flex items-start gap-3 p-3 rounded-lg bg-primary/5 border border-primary/20">
              <span className="text-primary text-lg mt-0.5">&#10003;</span>
              <div>
                <div className="text-sm font-semibold text-primary mb-0.5">Bind mounts safe</div>
                <div className="text-xs text-text-muted">Host-path bind mounts are unaffected — data stays on the host filesystem.</div>
              </div>
            </div>
          )}

          <div className="flex items-start gap-3 p-3 rounded-lg bg-blue-500/5 border border-blue-500/20">
            <span className="text-blue-400 text-lg mt-0.5">&#9432;</span>
            <div>
              <div className="text-sm font-semibold text-blue-400 mb-0.5">Fresh container</div>
              <div className="text-xs text-text-muted">Your dev install script will run on a new container. Previous resource settings will be pre-filled in the wizard.</div>
            </div>
          </div>
        </div>

        <div className="flex gap-3 justify-end">
          <button onClick={onClose} className="px-5 py-2.5 text-sm font-semibold border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-text-secondary transition-colors font-mono">Cancel</button>
          <button onClick={() => onConfirm(managedVolumes.filter(v => kept[v.name]).map(v => v.name))} className="px-5 py-2.5 text-sm font-semibold border-none rounded-lg cursor-pointer bg-yellow-400 text-bg-primary hover:shadow-[0_0_20px_rgba(250,204,21,0.3)] transition-all font-mono">Replace &amp; Install</button>
        </div>
      </div>
    </div>
  )
}
