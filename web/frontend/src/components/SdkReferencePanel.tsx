import { useState, useEffect } from 'react'

interface SdkMethod {
  name: string
  signature: string
  description: string
  group: string
}

// Preferred group display order
const GROUP_ORDER = [
  'Package Management', 'File Operations', 'Service Management',
  'Commands & System', 'User Management', 'Inputs', 'Logging & Outputs', 'Advanced',
]

export function SdkReferencePanel() {
  const [methods, setMethods] = useState<SdkMethod[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    fetch('/api/dev/sdk-docs')
      .then(r => r.json())
      .then((docs: SdkMethod[]) => setMethods(docs))
      .catch(() => setError('Failed to load SDK docs'))
  }, [])

  // Group methods
  const grouped = new Map<string, SdkMethod[]>()
  for (const m of methods) {
    const list = grouped.get(m.group) || []
    list.push(m)
    grouped.set(m.group, list)
  }

  // Sort groups by preferred order, unknown groups at the end
  const sortedGroups = Array.from(grouped.keys()).sort((a, b) => {
    const ai = GROUP_ORDER.indexOf(a)
    const bi = GROUP_ORDER.indexOf(b)
    return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
  })

  return (
    <div className="border border-border rounded-lg mt-4 overflow-hidden">
      <div className="bg-bg-card px-4 py-2 border-b border-border">
        <span className="text-xs text-text-muted font-mono uppercase">Python SDK Reference</span>
      </div>
      {error ? (
        <div className="p-4 text-xs text-red-400">{error}</div>
      ) : methods.length === 0 ? (
        <div className="p-4 text-xs text-text-muted">Loading...</div>
      ) : (
        <div className="p-4 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 max-h-64 overflow-y-auto">
          {sortedGroups.map(group => (
            <div key={group}>
              <h4 className="text-xs font-mono text-primary font-bold mb-1">{group}</h4>
              {grouped.get(group)!.map(m => (
                <div key={m.name} className="mb-1.5">
                  <code className="text-xs font-mono text-text-primary">{m.signature}</code>
                  <p className="text-xs text-text-muted mt-0.5">{m.description.split('\n')[0]}</p>
                </div>
              ))}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
