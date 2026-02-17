import { useState, useEffect } from 'react'
import { api } from '../api'
import type { PublishStatus } from '../types'
import { useEscapeKey } from '../hooks/useEscapeKey'

function PRStateBadge({ state }: { state: string }) {
  const styles: Record<string, string> = {
    pr_open: 'border-yellow-400 text-yellow-400',
    pr_merged: 'border-primary text-primary',
    pr_closed: 'border-red-400 text-red-400',
  }
  const labels: Record<string, string> = {
    pr_open: 'PR Open',
    pr_merged: 'PR Merged',
    pr_closed: 'PR Closed',
  }
  if (!state || !labels[state]) return null
  return <span className={`text-xs font-mono px-2 py-0.5 border rounded ${styles[state] || ''}`}>{labels[state]}</span>
}

export function DevSubmitDialog({ id, appName, onClose, requireAuth, isStack }: { id: string; appName: string; onClose: () => void; requireAuth: (cb: () => void) => void; isStack?: boolean }) {
  useEscapeKey(onClose)
  const [publishStatus, setPublishStatus] = useState<PublishStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [publishing, setPublishing] = useState(false)
  const [prUrl, setPrUrl] = useState('')
  const [publishAction, setPublishAction] = useState<'created' | 'updated' | ''>('')
  const [error, setError] = useState('')

  useEffect(() => {
    const fetchStatus = isStack ? api.devStackPublishStatus(id) : api.devPublishStatus(id)
    fetchStatus
      .then(s => { setPublishStatus(s); if (s.pr_url) setPrUrl(s.pr_url) })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [id, isStack])

  const handlePublish = () => {
    requireAuth(async () => {
      setPublishing(true)
      setError('')
      try {
        const result = isStack ? await api.devPublishStack(id) : await api.devPublish(id)
        setPrUrl(result.pr_url)
        setPublishAction((result.action as 'created' | 'updated') || 'created')
      } catch (e: unknown) {
        setError(e instanceof Error ? e.message : 'Publish failed')
      }
      setPublishing(false)
    })
  }

  const handleExportFallback = () => {
    requireAuth(() => {
      const form = document.createElement('form')
      form.method = 'POST'
      form.action = isStack ? api.devExportStackUrl(id) : api.devExportUrl(id)
      form.target = '_blank'
      document.body.appendChild(form)
      form.submit()
      document.body.removeChild(form)

      const kind = isStack ? 'Stack' : 'App'
      const title = encodeURIComponent(`New ${kind}: ${appName}`)
      const body = encodeURIComponent(`## ${kind} Submission\n\n**${kind} ID:** ${id}\n**${kind} Name:** ${appName}\n\nPlease attach the exported zip file to this issue.`)
      window.open(`https://github.com/battlewithbytes/pve-appstore-catalog/issues/new?title=${title}&body=${body}`, '_blank')
      onClose()
    })
  }

  const checkLabels: Record<string, string> = {
    github_connected: 'GitHub connected',
    validation_passed: 'Manifest validates',
    test_installed: 'Test install exists',
    apps_published: 'All apps published',
  }

  return (
    <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-50" onClick={onClose}>
      <div className="bg-bg-card border border-border rounded-lg p-6 w-full max-w-lg" onClick={(e) => e.stopPropagation()}>
        <h3 className="text-lg font-bold text-text-primary font-mono mb-4">Submit to Catalog</h3>

        {prUrl ? (
          <div>
            <div className="flex items-center gap-2 mb-4">
              <span className="text-primary text-lg">[OK]</span>
              <span className="text-sm font-mono text-text-primary">{publishAction === 'updated' ? 'Pull request updated!' : 'Pull request created!'}</span>
              {publishStatus?.pr_state && <PRStateBadge state={publishStatus.pr_state} />}
            </div>
            <a href={prUrl} target="_blank" rel="noopener noreferrer" className="text-sm font-mono text-primary underline break-all">{prUrl}</a>
            <div className="flex justify-end mt-4">
              <button onClick={onClose} className="bg-primary text-bg-primary rounded px-4 py-2 text-sm font-mono font-bold cursor-pointer hover:opacity-90">Done</button>
            </div>
          </div>
        ) : loading ? (
          <p className="text-text-muted font-mono text-sm">Checking publish readiness...</p>
        ) : (
          <div>
            {publishStatus && (
              <div className="mb-4">
                <span className="text-xs text-text-muted font-mono uppercase mb-2 block">Publish Checklist</span>
                {Object.entries(publishStatus.checks).map(([key, passed]) => (
                  <div key={key} className="flex items-center gap-2 py-0.5 text-xs font-mono">
                    <span className={passed ? 'text-primary' : 'text-red-400'}>{passed ? '[x]' : '[ ]'}</span>
                    <span className={passed ? 'text-text-secondary' : 'text-text-muted'}>{checkLabels[key] || key}</span>
                  </div>
                ))}
              </div>
            )}

            {publishStatus?.checks.github_connected ? (
              <div>
                <p className="text-xs text-text-muted mb-4">
                  This will push your changes and submit a pull request on the official catalog repository.
                </p>
                {error && <p className="text-xs text-red-400 font-mono mb-3">{error}</p>}
                <div className="flex justify-end gap-2">
                  <button onClick={onClose} className="bg-transparent border border-border rounded px-4 py-2 text-sm font-mono text-text-secondary cursor-pointer hover:border-primary transition-colors">Cancel</button>
                  <button
                    onClick={handlePublish}
                    disabled={publishing || !publishStatus?.ready}
                    className="bg-primary text-bg-primary rounded px-4 py-2 text-sm font-mono font-bold cursor-pointer hover:opacity-90 disabled:opacity-50"
                  >
                    {publishing ? 'Publishing...' : (publishStatus?.published && publishStatus?.pr_state === 'pr_open' ? 'Update Pull Request' : 'Submit Pull Request')}
                  </button>
                </div>
              </div>
            ) : (
              <div>
                <p className="text-xs text-text-muted mb-2">
                  GitHub is not connected. Connect GitHub in Settings to submit via pull request, or use the manual export method below.
                </p>
                <div className="flex justify-end gap-2">
                  <button onClick={onClose} className="bg-transparent border border-border rounded px-4 py-2 text-sm font-mono text-text-secondary cursor-pointer hover:border-primary transition-colors">Cancel</button>
                  <button onClick={() => { window.location.hash = '#/settings'; onClose() }} className="bg-transparent border border-border rounded px-4 py-2 text-sm font-mono text-text-secondary cursor-pointer hover:border-primary transition-colors">Go to Settings</button>
                  <button onClick={handleExportFallback} className="bg-primary text-bg-primary rounded px-4 py-2 text-sm font-mono font-bold cursor-pointer hover:opacity-90">Export + Manual Submit</button>
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
