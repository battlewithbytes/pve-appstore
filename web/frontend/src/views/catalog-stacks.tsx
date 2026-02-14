import { useState, useEffect } from 'react'
import Markdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { api } from '../api'
import type { CatalogStack } from '../types'
import { Center } from '../components/ui'

export function CatalogStacksList(_props: { requireAuth: (cb: () => void) => void }) {
  const [stacks, setStacks] = useState<CatalogStack[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    api.catalogStacks().then(d => setStacks(d.stacks || [])).catch(() => {}).finally(() => setLoading(false))
  }, [])

  if (loading) return <Center className="py-16"><span className="text-text-muted font-mono">Loading...</span></Center>

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-xl font-bold text-text-primary font-mono">Stack Templates</h2>
        <p className="text-xs text-text-muted font-mono">Pre-configured multi-app stacks for one-click install</p>
      </div>

      {stacks.length === 0 ? (
        <div className="border border-dashed border-border rounded-lg p-12 text-center">
          <p className="text-text-muted font-mono">No stack templates available in the catalog yet.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {stacks.map(stack => (
            <a key={stack.id} href={`#/catalog-stack/${stack.id}`} className="no-underline">
              <div className="border border-border rounded-lg p-4 hover:border-primary/50 transition-colors cursor-pointer bg-bg-card">
                <h3 className="text-sm font-bold text-text-primary font-mono mb-1">{stack.name}</h3>
                <p className="text-xs text-text-muted font-mono mb-2">v{stack.version} &middot; {stack.apps.length} app{stack.apps.length !== 1 ? 's' : ''}</p>
                <p className="text-xs text-text-secondary line-clamp-2 mb-3">{stack.description}</p>
                <div className="flex flex-wrap gap-1">
                  {stack.categories?.map(cat => (
                    <span key={cat} className="text-[10px] font-mono px-1.5 py-0.5 border border-border rounded text-text-muted">{cat}</span>
                  ))}
                </div>
              </div>
            </a>
          ))}
        </div>
      )}
    </div>
  )
}

export function CatalogStackDetailView({ id, requireAuth }: { id: string; requireAuth: (cb: () => void) => void }) {
  const [stack, setStack] = useState<CatalogStack | null>(null)
  const [readme, setReadme] = useState('')
  const [loading, setLoading] = useState(true)
  const [installing, setInstalling] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    api.catalogStack(id)
      .then(d => { setStack(d.stack); setReadme(d.readme || '') })
      .catch(() => setError('Stack not found'))
      .finally(() => setLoading(false))
  }, [id])

  const handleInstall = () => {
    requireAuth(async () => {
      setInstalling(true)
      try {
        const job = await api.installCatalogStack(id)
        window.location.hash = `#/job/${job.id}`
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : 'Install failed')
        setInstalling(false)
      }
    })
  }

  if (loading) return <Center className="py-16"><span className="text-text-muted font-mono">Loading...</span></Center>
  if (error || !stack) return <Center className="py-16"><span className="text-red-400 font-mono text-sm">{error || 'Not found'}</span></Center>

  return (
    <div>
      <a href="#/catalog-stacks" className="text-text-muted hover:text-primary text-sm font-mono mb-4 inline-block">&larr; All Stack Templates</a>

      <div className="border border-border rounded-lg p-6 bg-bg-card mb-6">
        <h2 className="text-xl font-bold text-text-primary font-mono mb-2">{stack.name}</h2>
        <p className="text-sm text-text-secondary mb-4">{stack.description}</p>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-4">
          <div>
            <span className="text-xs text-text-muted font-mono block">Version</span>
            <span className="text-sm font-mono text-text-primary">{stack.version}</span>
          </div>
          <div>
            <span className="text-xs text-text-muted font-mono block">Apps</span>
            <span className="text-sm font-mono text-text-primary">{stack.apps.length}</span>
          </div>
          <div>
            <span className="text-xs text-text-muted font-mono block">Default Resources</span>
            <span className="text-sm font-mono text-text-primary">{stack.lxc.defaults.cores}c / {stack.lxc.defaults.memory_mb}MB / {stack.lxc.defaults.disk_gb}GB</span>
          </div>
          <div>
            <span className="text-xs text-text-muted font-mono block">OS Template</span>
            <span className="text-sm font-mono text-text-primary truncate">{stack.lxc.ostemplate.split('/').pop()}</span>
          </div>
        </div>

        <div className="mb-4">
          <span className="text-xs text-text-muted font-mono block mb-2">Component Apps</span>
          <div className="flex flex-wrap gap-2">
            {stack.apps.map((app, i) => (
              <span key={i} className="text-xs font-mono px-2 py-1 border border-border rounded text-text-secondary">{app.app_id}</span>
            ))}
          </div>
        </div>

        <button
          onClick={handleInstall}
          disabled={installing}
          className="bg-primary text-bg-primary rounded px-6 py-2 text-sm font-mono font-bold cursor-pointer hover:opacity-90 disabled:opacity-50"
        >
          {installing ? 'Installing...' : 'Install Stack'}
        </button>
      </div>

      {readme && (
        <div className="border border-border rounded-lg p-6 bg-bg-card prose prose-invert max-w-none">
          <Markdown remarkPlugins={[remarkGfm]}>{readme}</Markdown>
        </div>
      )}
    </div>
  )
}
