export interface StructureItem {
  label: string
  line: number
  depth: number
}

function parsePythonStructure(content: string): StructureItem[] {
  const items: StructureItem[] = []
  const lines = content.split('\n')
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    const classMatch = line.match(/^class\s+(\w+)/)
    if (classMatch) {
      items.push({ label: classMatch[1], line: i + 1, depth: 0 })
      continue
    }
    const defMatch = line.match(/^(\s*)def\s+(\w+)/)
    if (defMatch) {
      const indent = defMatch[1].length
      items.push({ label: defMatch[2], line: i + 1, depth: indent > 0 ? 1 : 0 })
    }
  }
  return items
}

function parseYamlStructure(content: string): StructureItem[] {
  const items: StructureItem[] = []
  const lines = content.split('\n')
  let inInputs = false
  let inPermissions = false
  let inLxcDefaults = false
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    // Top-level key (no leading whitespace, not a comment)
    const topMatch = line.match(/^([a-zA-Z_][\w_]*):\s*/)
    if (topMatch) {
      items.push({ label: topMatch[1], line: i + 1, depth: 0 })
      inInputs = topMatch[1] === 'inputs'
      inPermissions = topMatch[1] === 'permissions'
      inLxcDefaults = false
      continue
    }
    // lxc.defaults sub-keys
    if (line.match(/^\s+defaults:\s*$/)) {
      items.push({ label: 'defaults', line: i + 1, depth: 1 })
      inLxcDefaults = true
      inInputs = false
      inPermissions = false
      continue
    }
    if (inLxcDefaults) {
      const subMatch = line.match(/^\s{4,}(\w+):/)
      if (subMatch) {
        items.push({ label: subMatch[1], line: i + 1, depth: 2 })
        continue
      }
      // End of defaults if we hit a non-indented or less-indented line
      if (line.match(/^\S/) || (line.trim() && !line.match(/^\s{4}/))) {
        inLxcDefaults = false
      }
    }
    // Input items: "  - key: xxx"
    if (inInputs) {
      const inputMatch = line.match(/^\s+-\s+key:\s*(.+)/)
      if (inputMatch) {
        items.push({ label: inputMatch[1].trim(), line: i + 1, depth: 1 })
        continue
      }
      // End of inputs section on new top-level key
      if (line.match(/^[a-zA-Z]/)) inInputs = false
    }
    // Permission sub-keys
    if (inPermissions) {
      const permMatch = line.match(/^\s{2}(\w+):/)
      if (permMatch) {
        items.push({ label: permMatch[1], line: i + 1, depth: 1 })
        continue
      }
      if (line.match(/^[a-zA-Z]/)) inPermissions = false
    }
  }
  return items
}

function parseMarkdownStructure(content: string): StructureItem[] {
  const items: StructureItem[] = []
  const lines = content.split('\n')
  for (let i = 0; i < lines.length; i++) {
    const match = lines[i].match(/^(#{1,6})\s+(.+)/)
    if (match) {
      items.push({ label: match[2].trim(), line: i + 1, depth: match[1].length - 1 })
    }
  }
  return items
}

export function parseStructure(content: string, filename: string): StructureItem[] {
  if (filename.endsWith('.py')) return parsePythonStructure(content)
  if (filename.endsWith('.yml') || filename.endsWith('.yaml')) return parseYamlStructure(content)
  if (filename.endsWith('.md')) return parseMarkdownStructure(content)
  return []
}

export function StructurePanel({ items, activeFile, onGotoLine }: { items: StructureItem[]; activeFile: string; onGotoLine: (file: string, line: number) => void }) {
  return (
    <div className="border-t border-border flex flex-col flex-1 min-h-0">
      <div className="bg-bg-card px-3 py-2 border-b border-border">
        <span className="text-xs text-text-muted font-mono uppercase">Structure</span>
      </div>
      <div className="p-1 flex-1 min-h-0 overflow-y-auto">
        {items.length > 0 ? items.map((item, i) => {
          const depthColors = ['text-primary', 'text-blue-400', 'text-purple-400']
          const color = depthColors[Math.min(item.depth, depthColors.length - 1)]
          return (
            <button
              key={i}
              onClick={() => onGotoLine(activeFile, item.line)}
              className={`w-full text-left px-2 py-0.5 text-[11px] font-mono rounded cursor-pointer transition-colors hover:bg-white/5 flex items-center justify-between gap-1 group ${color}`}
              style={{ paddingLeft: `${8 + item.depth * 12}px` }}
            >
              <span className="truncate">{item.label}</span>
              <span className="text-text-muted text-[10px] opacity-0 group-hover:opacity-100 shrink-0">{item.line}</span>
            </button>
          )
        }) : (
          <span className="text-[11px] text-text-muted font-mono px-2 py-2 block">No structure available</span>
        )}
      </div>
    </div>
  )
}
