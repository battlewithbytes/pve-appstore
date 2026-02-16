import { useState, useEffect } from 'react'
import { api } from '../api'
import type { BrowseEntry, MountInfo } from '../types'
import { useEscapeKey } from '../hooks/useEscapeKey'

export function DirectoryBrowser({ initialPath, onSelect, onClose }: { initialPath: string; onSelect: (path: string) => void; onClose: () => void }) {
  useEscapeKey(onClose)
  const [path, setPath] = useState(initialPath || '')
  const [entries, setEntries] = useState<BrowseEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [manualPath, setManualPath] = useState(initialPath || '')
  const [mounts, setMounts] = useState<MountInfo[]>([])
  const [newFolderName, setNewFolderName] = useState('')
  const [showNewFolder, setShowNewFolder] = useState(false)
  const [refreshKey, setRefreshKey] = useState(0)

  useEffect(() => {
    api.browseMounts().then(d => {
      const m = d.mounts || []
      setMounts(m)
      // If the current path isn't under any allowed mount, redirect to the first one
      if (m.length > 0 && !m.some(mt => path === mt.path || path.startsWith(mt.path + '/'))) {
        setPath(m[0].path)
        setManualPath(m[0].path)
      }
    }).catch(() => {})
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (!path) return // wait for mounts to resolve the initial path
    setLoading(true)
    setError('')
    api.browsePaths(path).then(d => {
      setEntries(d.entries)
      setManualPath(d.path)
      setLoading(false)
    }).catch(e => {
      setError(e instanceof Error ? e.message : 'Failed to browse')
      setEntries([])
      setLoading(false)
    })
  }, [path, refreshKey])

  const handleCreateFolder = async () => {
    if (!newFolderName.trim()) return
    try {
      await api.browseMkdir(path + '/' + newFolderName.trim())
      setNewFolderName('')
      setShowNewFolder(false)
      setRefreshKey(k => k + 1)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to create folder')
    }
  }

  const bookmarks = mounts.map(m => ({ label: m.path, path: m.path }))

  const pathSegments = path.split('/').filter(Boolean)

  return (
    <div className="fixed inset-0 bg-black/80 flex items-center justify-center z-[150]" onClick={onClose}>
      <div className="bg-bg-card border border-border rounded-xl p-6 w-full max-w-[500px] max-h-[70vh] flex flex-col" onClick={e => e.stopPropagation()}>
        <h3 className="text-sm font-bold text-text-primary mb-3 font-mono">Browse Host Path</h3>

        {/* Bookmark chips */}
        <div className="flex gap-1.5 mb-3 flex-wrap">
          {bookmarks.map(bm => (
            <button key={bm.path} onClick={() => setPath(bm.path)}
              className={`px-2.5 py-1 text-[11px] font-mono rounded-md border cursor-pointer transition-colors ${
                path.startsWith(bm.path) ? 'border-primary text-primary bg-primary/10' : 'border-border text-text-muted bg-bg-secondary hover:border-primary hover:text-primary'
              }`}>
              {bm.label}
            </button>
          ))}
        </div>

        <div className="flex gap-2 mb-3">
          <input type="text" value={manualPath} onChange={e => setManualPath(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') setPath(manualPath) }}
            className="flex-1 px-3 py-1.5 bg-bg-secondary border border-border rounded-md text-text-primary text-xs outline-none focus:border-primary font-mono" />
          <button onClick={() => setPath(manualPath)} className="px-3 py-1.5 text-xs font-mono border border-border rounded-md text-text-secondary bg-bg-secondary hover:border-primary hover:text-primary transition-colors cursor-pointer">Go</button>
        </div>

        <div className="flex items-center gap-1 mb-2 text-xs font-mono flex-wrap">
          <button onClick={() => setPath('/')} className="text-primary hover:underline bg-transparent border-none cursor-pointer p-0 font-mono text-xs">/</button>
          {pathSegments.map((seg, i) => (
            <span key={i} className="flex items-center gap-1">
              <span className="text-text-muted">/</span>
              <button onClick={() => setPath('/' + pathSegments.slice(0, i + 1).join('/'))}
                className="text-primary hover:underline bg-transparent border-none cursor-pointer p-0 font-mono text-xs">{seg}</button>
            </span>
          ))}
        </div>

        <div className="flex-1 overflow-auto border border-border rounded-md bg-bg-secondary">
          {loading ? (
            <div className="p-4 text-text-muted text-xs font-mono">Loading...</div>
          ) : error ? (
            <div className="p-4 text-status-stopped text-xs font-mono">{error}</div>
          ) : (
            <>
              {path !== '/' && (
                <button onClick={() => setPath(path.replace(/\/[^/]+\/?$/, '') || '/')}
                  className="w-full text-left px-3 py-1.5 text-xs font-mono text-primary hover:bg-primary/10 transition-colors bg-transparent border-none border-b border-border cursor-pointer flex items-center gap-2">
                  <span className="text-text-muted">..</span> <span className="text-text-secondary">(up)</span>
                </button>
              )}
              {entries.filter(e => e.is_dir).map(entry => (
                <button key={entry.path} onClick={() => setPath(entry.path)}
                  className="w-full text-left px-3 py-1.5 text-xs font-mono text-text-primary hover:bg-primary/10 transition-colors bg-transparent border-none cursor-pointer flex items-center gap-2">
                  <span className="text-primary">&#128193;</span> {entry.name}/
                </button>
              ))}
              {entries.filter(e => !e.is_dir).map(entry => (
                <div key={entry.path} className="px-3 py-1.5 text-xs font-mono text-text-muted flex items-center gap-2">
                  <span>&#128196;</span> {entry.name}
                </div>
              ))}
              {entries.length === 0 && !error && (
                <div className="p-4 text-text-muted text-xs font-mono italic">Empty directory</div>
              )}
            </>
          )}
        </div>

        {/* New Folder inline */}
        <div className="mt-2 flex items-center gap-2">
          {showNewFolder ? (
            <>
              <input type="text" value={newFolderName} onChange={e => setNewFolderName(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') handleCreateFolder(); if (e.key === 'Escape') { setShowNewFolder(false); setNewFolderName('') } }}
                placeholder="folder name" autoFocus
                className="flex-1 px-3 py-1.5 bg-bg-secondary border border-border rounded-md text-text-primary text-xs outline-none focus:border-primary font-mono placeholder:text-text-muted" />
              <button onClick={handleCreateFolder} className="px-3 py-1.5 text-xs font-mono border-none rounded-md text-bg-primary bg-primary cursor-pointer hover:shadow-[0_0_10px_rgba(0,255,157,0.3)] transition-all">Create</button>
              <button onClick={() => { setShowNewFolder(false); setNewFolderName('') }} className="px-2 py-1.5 text-xs font-mono text-text-muted bg-transparent border-none cursor-pointer hover:text-text-primary">&times;</button>
            </>
          ) : (
            <button onClick={() => setShowNewFolder(true)} className="text-primary text-xs font-mono bg-transparent border-none cursor-pointer hover:underline p-0">
              + New Folder
            </button>
          )}
        </div>

        <div className="flex gap-3 mt-4 justify-end">
          <button onClick={onClose} className="px-4 py-2 text-xs font-semibold border border-border rounded-lg cursor-pointer text-text-secondary bg-transparent hover:border-text-secondary transition-colors font-mono">Cancel</button>
          <button onClick={() => onSelect(path)} className="px-4 py-2 text-xs font-semibold border-none rounded-lg cursor-pointer bg-primary text-bg-primary hover:shadow-[0_0_20px_rgba(0,255,157,0.3)] transition-all font-mono">
            Select: {path}
          </button>
        </div>
      </div>
    </div>
  )
}
