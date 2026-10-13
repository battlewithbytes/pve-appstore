import type { StructureItem } from './structure'

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
