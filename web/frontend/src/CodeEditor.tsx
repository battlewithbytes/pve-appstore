import { useEffect, useRef, useCallback } from 'react'
import { EditorView, keymap, lineNumbers, highlightActiveLine, highlightActiveLineGutter, drawSelection } from '@codemirror/view'
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

// Python SDK completions
const sdkMethods = [
  { label: 'self.apt_install', type: 'method', detail: '(packages: list[str])', info: 'Install apt packages' },
  { label: 'self.pip_install', type: 'method', detail: '(packages: list[str])', info: 'Install pip packages' },
  { label: 'self.write_config', type: 'method', detail: '(path: str, content: str)', info: 'Write a configuration file' },
  { label: 'self.enable_service', type: 'method', detail: '(name: str)', info: 'Enable a systemd service' },
  { label: 'self.restart_service', type: 'method', detail: '(name: str)', info: 'Restart a systemd service' },
  { label: 'self.run_command', type: 'method', detail: '(cmd: str)', info: 'Run a shell command' },
  { label: 'self.create_user', type: 'method', detail: '(username: str, ...)', info: 'Create a system user' },
  { label: 'self.create_dir', type: 'method', detail: '(path: str, ...)', info: 'Create a directory' },
  { label: 'self.chown', type: 'method', detail: '(path: str, user: str, group: str)', info: 'Change file ownership' },
  { label: 'self.download', type: 'method', detail: '(url: str, dest: str)', info: 'Download a file' },
  { label: 'self.run_installer_script', type: 'method', detail: '(url: str)', info: 'Download and run an installer script' },
  { label: 'self.add_apt_key', type: 'method', detail: '(url: str, keyring: str)', info: 'Add an APT GPG key' },
  { label: 'self.add_apt_repo', type: 'method', detail: '(line: str, file: str)', info: 'Add an APT repository' },
  { label: 'self.pkg_install', type: 'method', detail: '(*packages: str)', info: 'OS-aware package install (apt/apk)' },
  { label: 'self.create_service', type: 'method', detail: '(name, exec_start, ...)', info: 'Create systemd/OpenRC service' },
  { label: 'self.wait_for_http', type: 'method', detail: '(url, timeout=60, interval=3)', info: 'Poll HTTP endpoint until 200' },
  { label: 'self.write_env_file', type: 'method', detail: '(path, env_dict, mode="0644")', info: 'Write KEY=VALUE env file' },
  { label: 'self.status_page', type: 'method', detail: '(port, title, api_url, fields)', info: 'Deploy CCO-themed status page' },
  { label: 'self.pull_oci_binary', type: 'method', detail: '(image, dest, tag="latest")', info: 'Pull binary from OCI/Docker image' },
  { label: 'self.sysctl', type: 'method', detail: '(settings: dict)', info: 'Apply sysctl settings persistently' },
  { label: 'self.disable_ipv6', type: 'method', detail: '()', info: 'Disable IPv6 via sysctl' },
  { label: 'self.provision_file', type: 'method', detail: '(name: str)', info: 'Read file from provision directory' },
  { label: 'self.deploy_provision_file', type: 'method', detail: '(name, dest, mode?)', info: 'Copy provision file to destination' },
  { label: 'self.log', type: 'method', detail: '(message: str)', info: 'Log a message' },
  { label: 'self.inputs.string', type: 'method', detail: '(key: str, default?: str)', info: 'Get a string input value' },
  { label: 'self.inputs.integer', type: 'method', detail: '(key: str, default?: int)', info: 'Get an integer input value' },
  { label: 'self.inputs.boolean', type: 'method', detail: '(key: str, default?: bool)', info: 'Get a boolean input value' },
  { label: 'self.inputs.secret', type: 'method', detail: '(key: str)', info: 'Get a secret input value' },
]

function sdkCompletions(context: CompletionContext): CompletionResult | null {
  const word = context.matchBefore(/self\.\w*/)
  if (!word || (word.from === word.to && !context.explicit)) return null
  return {
    from: word.from,
    options: sdkMethods,
    validFor: /^self\.\w*$/,
  }
}

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
        ...(isPython ? [autocompletion({ override: [sdkCompletions] })] : []),
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

  // Only sync when file changes (not on every keystroke)
  const prevFilename = useRef(filename)
  useEffect(() => {
    if (prevFilename.current !== filename) {
      syncValue(value)
      prevFilename.current = filename
      // Update language
      const view = viewRef.current
      if (view) {
        view.dispatch({
          effects: langCompartment.reconfigure(getLangExtension(filename)),
        })
      }
    }
  }, [filename, value, syncValue])

  // Scroll to a specific line when gotoLine changes
  useEffect(() => {
    const view = viewRef.current
    if (!view || !gotoLine || gotoLine < 1) return
    const line = Math.min(Math.floor(gotoLine), view.state.doc.lines)
    const lineInfo = view.state.doc.line(line)
    view.dispatch({
      selection: { anchor: lineInfo.from },
      scrollIntoView: true,
    })
    view.focus()
  }, [gotoLine])

  return <div ref={containerRef} className="h-full overflow-hidden" />
}
