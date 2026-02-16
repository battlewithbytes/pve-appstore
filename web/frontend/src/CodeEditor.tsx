import { useEffect, useRef, useCallback } from 'react'
import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter, drawSelection, hoverTooltip } from '@codemirror/view'
import { EditorState, Compartment } from '@codemirror/state'
import { yaml } from '@codemirror/lang-yaml'
import { python } from '@codemirror/lang-python'
import { markdown } from '@codemirror/lang-markdown'
import { html } from '@codemirror/lang-html'
import { oneDark } from '@codemirror/theme-one-dark'
import { defaultKeymap, indentWithTab, history, historyKeymap } from '@codemirror/commands'
import { autocompletion } from '@codemirror/autocomplete'
import type { CompletionContext, CompletionResult } from '@codemirror/autocomplete'
import { syntaxHighlighting, defaultHighlightStyle, indentOnInput, bracketMatching, foldGutter, foldKeymap, foldService } from '@codemirror/language'
import { searchKeymap, highlightSelectionMatches } from '@codemirror/search'

// Fold multi-line bracket pairs: ( ... ), [ ... ], { ... }
const bracketFold = foldService.of((state, lineStart) => {
  const line = state.doc.lineAt(lineStart)
  const text = line.text
  // Find last unmatched opening bracket on this line
  const openers = '([{'
  const closers = ')]}'
  let stack: { char: string; pos: number }[] = []
  for (let i = 0; i < text.length; i++) {
    const ch = text[i]
    const oi = openers.indexOf(ch)
    if (oi >= 0) stack.push({ char: closers[oi], pos: line.from + i })
    const ci = closers.indexOf(ch)
    if (ci >= 0) {
      // Pop matching opener
      for (let j = stack.length - 1; j >= 0; j--) {
        if (stack[j].char === ch) { stack.splice(j, 1); break }
      }
    }
  }
  if (stack.length === 0) return null
  // Use the first unmatched opener
  const open = stack[0]
  const closer = open.char
  // Scan forward to find the matching close bracket
  let depth = 1
  for (let pos = open.pos + 1; pos < state.doc.length && depth > 0; pos++) {
    const ch = state.doc.sliceString(pos, pos + 1)
    if (ch === text[open.pos - line.from]) depth++ // same opener
    if (ch === closer) depth--
    if (depth === 0) {
      const closeLine = state.doc.lineAt(pos)
      // Only fold if the close bracket is on a different line
      if (closeLine.number > line.number) {
        return { from: open.pos + 1, to: pos }
      }
    }
  }
  return null
})

// --- SDK docs fetched from /api/dev/sdk-docs (single source of truth: Python SDK) ---

interface SDKDocEntry {
  name: string        // lookup key: "apt_install", "inputs.string", "log.info"
  signature: string   // full signature: "self.apt_install(*packages: str) -> None"
  description: string // first paragraph of docstring
}

// Module-level state populated asynchronously from the API.
// Completions and hover handlers read from these — they return empty
// results until the fetch completes (typically <100ms on localhost).
let sdkCompletionOptions: { label: string; type: string; detail: string; info: string }[] = []
let sdkHoverDocs: Record<string, { signature: string; description: string }> = {}

function extractDetail(sig: string): string {
  const start = sig.indexOf('(')
  if (start < 0) return ''
  let depth = 0
  for (let i = start; i < sig.length; i++) {
    if (sig[i] === '(') depth++
    if (sig[i] === ')') depth--
    if (depth === 0) return sig.substring(start, i + 1)
  }
  return sig.substring(start)
}

// Fetch SDK docs once when module loads — populates completions + hover
fetch('/api/dev/sdk-docs')
  .then(r => r.ok ? r.json() as Promise<SDKDocEntry[]> : [])
  .then(docs => {
    sdkCompletionOptions = docs.map(d => ({
      label: 'self.' + d.name,
      type: 'method',
      detail: extractDetail(d.signature),
      info: d.description,
    }))
    sdkHoverDocs = {}
    for (const d of docs) {
      sdkHoverDocs[d.name] = { signature: d.signature, description: d.description }
    }
  })
  .catch(() => {})

function sdkCompletions(context: CompletionContext): CompletionResult | null {
  const word = context.matchBefore(/self\.[\w.]*/)
  if (!word || (word.from === word.to && !context.explicit)) return null
  return {
    from: word.from,
    options: sdkCompletionOptions,
    validFor: /^self\.[\w.]*$/,
  }
}

// Hover tooltip for SDK methods — matches self.method, self.inputs.method, self.log.method
const sdkHoverTooltip = hoverTooltip((view, pos) => {
  const line = view.state.doc.lineAt(pos)
  const lineText = line.text
  const col = pos - line.from

  const pattern = /self\.((?:inputs|log)\.\w+|\w+)/g
  let match
  while ((match = pattern.exec(lineText)) !== null) {
    const start = match.index
    const end = start + match[0].length
    if (col >= start && col < end) {
      const lookupKey = match[1]
      const doc = sdkHoverDocs[lookupKey]
      if (!doc) return null
      return {
        pos: line.from + start,
        end: line.from + end,
        above: true,
        create() {
          const dom = document.createElement('div')
          dom.className = 'cm-sdk-tooltip'

          const sig = document.createElement('div')
          sig.className = 'cm-sdk-tooltip-sig'
          sig.textContent = doc.signature
          dom.appendChild(sig)

          const hr = document.createElement('div')
          hr.className = 'cm-sdk-tooltip-hr'
          dom.appendChild(hr)

          const desc = document.createElement('div')
          desc.className = 'cm-sdk-tooltip-desc'
          desc.textContent = doc.description
          dom.appendChild(desc)

          return { dom }
        },
      }
    }
  }
  return null
})

const langCompartment = new Compartment()

function getLangExtension(filename: string) {
  if (filename.endsWith('.yml') || filename.endsWith('.yaml')) return yaml()
  if (filename.endsWith('.py')) return python()
  if (filename.endsWith('.md')) return markdown()
  if (filename.endsWith('.html') || filename.endsWith('.tmpl')) return html()
  return []
}

// Custom dark theme that matches the app
const appTheme = EditorView.theme({
  '&': {
    backgroundColor: '#0A0A0A',
    color: '#e0e0e0',
    fontSize: '13px',
    height: '100%',
  },
  '.cm-content': {
    fontFamily: '"JetBrains Mono", "Fira Code", monospace',
    padding: '8px 0',
  },
  '.cm-gutters': {
    backgroundColor: '#0A0A0A',
    color: '#555',
    border: 'none',
    borderRight: '1px solid #1a1a1a',
  },
  '.cm-foldGutter .cm-gutterElement': {
    color: '#888',
    cursor: 'pointer',
    padding: '0 4px',
    fontSize: '14px',
  },
  '.cm-foldGutter .cm-gutterElement:hover': {
    color: '#00FF9D',
  },
  '.cm-activeLineGutter': {
    backgroundColor: '#111',
  },
  '.cm-activeLine': {
    backgroundColor: '#111',
  },
  '&.cm-focused .cm-cursor': {
    borderLeftColor: '#00FF9D',
  },
  '&.cm-focused .cm-selectionBackground, ::selection': {
    backgroundColor: '#00FF9D22',
  },
  '.cm-tooltip.cm-tooltip-autocomplete': {
    backgroundColor: '#1a1a1a',
    border: '1px solid #333',
  },
  '.cm-tooltip.cm-tooltip-autocomplete > ul > li[aria-selected]': {
    backgroundColor: '#00FF9D22',
    color: '#e0e0e0',
  },
  '.cm-tooltip-hover': {
    backgroundColor: '#1a1a1a',
    border: '1px solid #333',
    borderRadius: '6px',
    maxWidth: '520px',
    boxShadow: '0 4px 12px rgba(0,0,0,0.5)',
  },
  '.cm-sdk-tooltip': {
    padding: '8px 12px',
  },
  '.cm-sdk-tooltip-sig': {
    fontFamily: '"JetBrains Mono", "Fira Code", monospace',
    fontSize: '12px',
    color: '#00FF9D',
    whiteSpace: 'pre-wrap',
    lineHeight: '1.4',
  },
  '.cm-sdk-tooltip-hr': {
    height: '1px',
    backgroundColor: '#333',
    margin: '6px 0',
  },
  '.cm-sdk-tooltip-desc': {
    fontSize: '12px',
    color: '#aaa',
    lineHeight: '1.5',
    whiteSpace: 'pre-wrap',
  },
}, { dark: true })

interface CodeEditorProps {
  value: string
  onChange: (value: string) => void
  filename: string
  onSave?: () => void
  gotoLine?: number
}

export function CodeEditor({ value, onChange, filename, onSave, gotoLine }: CodeEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const onChangeRef = useRef(onChange)
  const onSaveRef = useRef(onSave)

  onChangeRef.current = onChange
  onSaveRef.current = onSave

  // Create editor on mount
  useEffect(() => {
    if (!containerRef.current) return

    const isPython = filename.endsWith('.py')

    const state = EditorState.create({
      doc: value,
      extensions: [
        lineNumbers(),
        highlightActiveLineGutter(),
        highlightActiveLine(),
        drawSelection(),
        indentOnInput(),
        bracketMatching(),
        foldGutter(),
        bracketFold,
        history(),
        highlightSelectionMatches(),
        syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
        langCompartment.of(getLangExtension(filename)),
        oneDark,
        appTheme,
        keymap.of([
          ...defaultKeymap,
          ...historyKeymap,
          ...searchKeymap,
          ...foldKeymap,
          indentWithTab,
          {
            key: 'Mod-s',
            run: () => {
              onSaveRef.current?.()
              return true
            },
          },
        ]),
        ...(isPython ? [autocompletion({ override: [sdkCompletions] }), sdkHoverTooltip] : []),
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            onChangeRef.current(update.state.doc.toString())
          }
        }),
      ],
    })

    const view = new EditorView({
      state,
      parent: containerRef.current,
    })

    viewRef.current = view
    return () => { view.destroy() }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filename])

  // Sync external value changes
  const syncValue = useCallback((newValue: string) => {
    const view = viewRef.current
    if (!view) return
    const currentValue = view.state.doc.toString()
    if (currentValue !== newValue) {
      view.dispatch({
        changes: { from: 0, to: currentValue.length, insert: newValue },
      })
    }
  }, [])

  // Sync external value/filename changes into the editor
  const prevFilename = useRef(filename)
  useEffect(() => {
    if (prevFilename.current !== filename) {
      prevFilename.current = filename
      // Update language
      const view = viewRef.current
      if (view) {
        view.dispatch({
          effects: langCompartment.reconfigure(getLangExtension(filename)),
        })
      }
    }
    // Always attempt sync — syncValue internally skips if editor already matches
    syncValue(value)
  }, [filename, value, syncValue])

  // Scroll to a specific line when gotoLine changes
  useEffect(() => {
    const view = viewRef.current
    if (!view || !gotoLine || gotoLine < 1) return
    const line = Math.min(Math.floor(gotoLine), view.state.doc.lines)
    const lineInfo = view.state.doc.line(line)
    view.dispatch({
      selection: { anchor: lineInfo.from },
      effects: EditorView.scrollIntoView(lineInfo.from, { y: 'center' }),
    })
    view.focus()
  }, [gotoLine])

  return <div ref={containerRef} className="h-full overflow-hidden" />
}
