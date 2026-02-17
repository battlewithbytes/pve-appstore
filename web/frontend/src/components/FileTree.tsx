import { useState, useCallback, useRef, useMemo } from 'react'
import type { DevFile } from '../types'

interface FileTreeProps {
  files: DevFile[]
  activeFile: string
  protectedFiles: Set<string>
  onSelectFile: (path: string) => void
  onDeleteFile: (path: string) => void
  onRenameFile: (oldPath: string, newPath: string) => void
  onMoveFile: (path: string, targetDir: string) => void
  onNewFile: (name: string) => void
  onUploadFile: (file: File) => void
  onUploadIcon: (file: File) => void
}

export function FileTree({ files, activeFile, protectedFiles, onSelectFile, onDeleteFile, onRenameFile, onMoveFile, onNewFile, onUploadFile, onUploadIcon }: FileTreeProps) {
  const [showAddMenu, setShowAddMenu] = useState(false)
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; file: string } | null>(null)
  const [renameTarget, setRenameTarget] = useState<string | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const [showNewFile, setShowNewFile] = useState(false)
  const [newFileName, setNewFileName] = useState('')
  const [moveFile, setMoveFile] = useState<string | null>(null)
  const [customDir, setCustomDir] = useState('')
  const uploadInputRef = useRef<HTMLInputElement>(null)
  const uploadModeRef = useRef<'general' | 'icon'>('general')

  const dismissContextMenu = useCallback(() => setContextMenu(null), [])

  const handleContextMenu = useCallback((e: React.MouseEvent, file: string) => {
    e.preventDefault()
    setContextMenu({ x: e.clientX, y: e.clientY, file })
  }, [])

  const handleRenameSubmit = useCallback((oldPath: string, newPath: string) => {
    const trimmed = newPath.trim()
    if (!trimmed || trimmed === oldPath) {
      setRenameTarget(null)
      return
    }
    onRenameFile(oldPath, trimmed)
    setRenameTarget(null)
  }, [onRenameFile])

  // Directories derived from actual files (no hardcoded values)
  const appDirs = useMemo(() => {
    const dirs = new Set<string>()
    for (const f of files) {
      if (f.is_dir) dirs.add(f.path)
      // Also extract parent dirs from file paths
      if (f.path.includes('/')) {
        dirs.add(f.path.substring(0, f.path.lastIndexOf('/')))
      }
    }
    return Array.from(dirs).sort()
  }, [files])

  const getMoveTargets = useCallback((filePath: string) => {
    const currentDir = filePath.includes('/') ? filePath.substring(0, filePath.lastIndexOf('/')) : ''
    const targets: { label: string; dir: string }[] = []
    if (currentDir) {
      targets.push({ label: '/ (root)', dir: '' })
    }
    for (const dir of appDirs) {
      if (dir !== currentDir) {
        targets.push({ label: `${dir}/`, dir })
      }
    }
    return targets
  }, [appDirs])

  const handleMove = useCallback((dir: string) => {
    if (moveFile) {
      onMoveFile(moveFile, dir)
      setMoveFile(null)
      setCustomDir('')
    }
  }, [moveFile, onMoveFile])

  // Build sorted file list: core files first, extras in middle, README + icon at end
  const coreFileSet = useMemo(() => new Set(['app.yml', 'provision/install.py', 'README.md', 'icon.png']), [])
  const hasIcon = useMemo(() => files.some(f => f.path === 'icon.png'), [files])
  const allFiles = useMemo(() => {
    const core = ['app.yml', 'provision/install.py']
    const extra = files
      .filter(f => !f.is_dir && !coreFileSet.has(f.path) && !f.path.startsWith('.') && !f.path.endsWith('.png') && !f.path.endsWith('.jpg') && !f.path.endsWith('.ico'))
      .map(f => f.path)
      .sort()
    return [...core, ...extra, 'README.md', ...(hasIcon ? ['icon.png'] : [])]
  }, [files, coreFileSet, hasIcon])

  return (
    <>
      {/* Context menu overlay */}
      {contextMenu && (
        <>
          <div className="fixed inset-0 z-40" onClick={dismissContextMenu} onContextMenu={e => { e.preventDefault(); dismissContextMenu() }} />
          <div className="fixed z-50 bg-bg-card border border-border rounded-lg shadow-lg overflow-hidden min-w-[140px]" style={{ left: contextMenu.x, top: contextMenu.y }}>
            {!protectedFiles.has(contextMenu.file) && contextMenu.file !== '' && (
              <button
                className="w-full text-left px-3 py-2 text-xs font-mono text-text-secondary hover:bg-white/5 hover:text-text-primary cursor-pointer transition-colors"
                onClick={() => { dismissContextMenu(); setRenameTarget(contextMenu.file); setRenameValue(contextMenu.file) }}
              >Rename</button>
            )}
            {!protectedFiles.has(contextMenu.file) && contextMenu.file !== '' && (
              <button
                className="w-full text-left px-3 py-2 text-xs font-mono text-text-secondary hover:bg-white/5 hover:text-text-primary cursor-pointer transition-colors"
                onClick={() => { dismissContextMenu(); setMoveFile(contextMenu.file); setCustomDir('') }}
              >Move to...</button>
            )}
            {!protectedFiles.has(contextMenu.file) && (
              <button
                className="w-full text-left px-3 py-2 text-xs font-mono text-red-400 hover:bg-red-500/10 cursor-pointer transition-colors border-t border-border"
                onClick={() => { dismissContextMenu(); onDeleteFile(contextMenu.file) }}
              >Delete</button>
            )}
            {contextMenu.file === 'icon.png' && (
              <button
                className="w-full text-left px-3 py-2 text-xs font-mono text-text-secondary hover:bg-white/5 hover:text-text-primary cursor-pointer transition-colors border-t border-border"
                onClick={() => { dismissContextMenu(); uploadModeRef.current = 'icon'; uploadInputRef.current?.click() }}
              >Replace</button>
            )}
            <button
              className="w-full text-left px-3 py-2 text-xs font-mono text-text-secondary hover:bg-white/5 hover:text-text-primary cursor-pointer transition-colors border-t border-border"
              onClick={() => { dismissContextMenu(); setShowNewFile(true); setNewFileName('') }}
            >New file</button>
            <button
              className="w-full text-left px-3 py-2 text-xs font-mono text-text-secondary hover:bg-white/5 hover:text-text-primary cursor-pointer transition-colors border-t border-border"
              onClick={() => { dismissContextMenu(); uploadModeRef.current = 'general'; uploadInputRef.current?.click() }}
            >Upload file</button>
          </div>
        </>
      )}

      {/* Move dialog */}
      {moveFile && (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onClick={() => setMoveFile(null)}>
          <div className="bg-bg-card border border-border rounded-lg p-4 w-72" onClick={e => e.stopPropagation()}>
            <div className="text-xs font-mono text-text-muted mb-1">Move file</div>
            <div className="text-sm font-mono text-text-primary mb-3 break-all">{moveFile}</div>
            {getMoveTargets(moveFile).length > 0 && (
              <div className="mb-3">
                <div className="text-xs font-mono text-text-muted mb-1.5">Quick pick</div>
                <div className="flex flex-wrap gap-1.5">
                  {getMoveTargets(moveFile).map(t => (
                    <button
                      key={t.dir}
                      className="px-2.5 py-1 text-xs font-mono rounded border border-border text-text-secondary hover:border-primary hover:text-primary cursor-pointer transition-colors bg-transparent"
                      onClick={() => handleMove(t.dir)}
                    >{t.label}</button>
                  ))}
                </div>
              </div>
            )}
            <form onSubmit={e => { e.preventDefault(); const d = customDir.trim(); if (d) handleMove(d) }}>
              <div className="text-xs font-mono text-text-muted mb-1.5">Custom directory</div>
              <div className="flex gap-2">
                <input
                  autoFocus
                  value={customDir}
                  onChange={e => setCustomDir(e.target.value)}
                  placeholder="path/to/dir"
                  className="flex-1 bg-bg-primary border border-border rounded px-2 py-1 text-xs font-mono text-text-primary outline-none focus:border-primary"
                  onKeyDown={e => { if (e.key === 'Escape') setMoveFile(null) }}
                />
                <button
                  type="submit"
                  disabled={!customDir.trim()}
                  className="bg-primary text-bg-primary rounded px-3 py-1 text-xs font-mono font-bold cursor-pointer hover:opacity-90 disabled:opacity-50"
                >Move</button>
              </div>
            </form>
            <div className="flex justify-end mt-3">
              <button
                onClick={() => setMoveFile(null)}
                className="text-xs font-mono text-text-muted hover:text-text-secondary cursor-pointer bg-transparent border-0"
              >Cancel</button>
            </div>
          </div>
        </div>
      )}

      {/* Header */}
      <div className="bg-bg-card px-3 py-2 border-b border-border flex items-center justify-between">
        <span className="text-xs text-text-muted font-mono uppercase">Files</span>
        <div className="relative">
          <button
            onClick={() => setShowAddMenu(!showAddMenu)}
            className="text-text-muted hover:text-primary text-sm font-mono cursor-pointer transition-colors leading-none"
            title="Add file"
          >+</button>
          {showAddMenu && (
            <>
              <div className="fixed inset-0 z-10" onClick={() => setShowAddMenu(false)} />
              <div className="absolute right-0 top-full mt-1 z-20 bg-bg-card border border-border rounded-lg shadow-lg overflow-hidden min-w-[120px]">
                <button
                  className="w-full text-left px-3 py-2 text-xs font-mono text-text-secondary hover:bg-white/5 hover:text-text-primary cursor-pointer transition-colors"
                  onClick={() => { setShowAddMenu(false); setShowNewFile(true); setNewFileName('') }}
                >New file</button>
                <button
                  className="w-full text-left px-3 py-2 text-xs font-mono text-text-secondary hover:bg-white/5 hover:text-text-primary cursor-pointer transition-colors border-t border-border"
                  onClick={() => { setShowAddMenu(false); uploadModeRef.current = 'general'; uploadInputRef.current?.click() }}
                >Upload file</button>
              </div>
            </>
          )}
        </div>
        <input
          ref={uploadInputRef}
          type="file"
          className="hidden"
          onChange={e => {
            const f = e.target.files?.[0]
            if (f) {
              if (uploadModeRef.current === 'icon') {
                onUploadIcon(f)
              } else {
                onUploadFile(f)
              }
            }
            e.target.value = ''
          }}
        />
      </div>

      {/* File list */}
      <div
        className="p-1 flex-1 min-h-0 overflow-y-auto"
        onContextMenu={e => {
          if (e.target === e.currentTarget) {
            e.preventDefault()
            setContextMenu({ x: e.clientX, y: e.clientY, file: '' })
          }
        }}
      >
        {allFiles.map(f => renameTarget === f ? (
          <form
            key={f}
            className="px-1 py-0.5"
            onSubmit={e => { e.preventDefault(); handleRenameSubmit(f, renameValue) }}
          >
            <input
              autoFocus
              value={renameValue}
              onChange={e => setRenameValue(e.target.value)}
              onBlur={() => handleRenameSubmit(f, renameValue)}
              className="w-full bg-bg-primary border border-primary rounded px-2 py-1 text-xs font-mono text-text-primary outline-none"
              onKeyDown={e => { if (e.key === 'Escape') { setRenameTarget(null) } }}
            />
          </form>
        ) : (
          <button
            key={f}
            onClick={() => onSelectFile(f)}
            onContextMenu={e => handleContextMenu(e, f)}
            className={`w-full text-left px-3 py-1.5 text-xs font-mono rounded cursor-pointer transition-colors ${activeFile === f ? 'bg-primary/10 text-primary' : 'text-text-secondary hover:text-text-primary hover:bg-white/5'}`}
          >
            {f}
          </button>
        ))}
        {showNewFile && (
          <form
            className="px-1 py-1"
            onSubmit={e => {
              e.preventDefault()
              const name = newFileName.trim()
              if (!name) return
              onNewFile(name)
              setShowNewFile(false)
              setNewFileName('')
            }}
          >
            <input
              autoFocus
              value={newFileName}
              onChange={e => setNewFileName(e.target.value)}
              onBlur={() => { if (!newFileName.trim()) setShowNewFile(false) }}
              placeholder="filename..."
              className="w-full bg-bg-primary border border-primary rounded px-2 py-1 text-xs font-mono text-text-primary outline-none"
              onKeyDown={e => { if (e.key === 'Escape') setShowNewFile(false) }}
            />
          </form>
        )}
      </div>
    </>
  )
}
