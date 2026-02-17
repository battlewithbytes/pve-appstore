import type { ValidationMsg } from '../types'

export function DevValidationMsg({ msg, type, onNavigate, onGotoLine }: { msg: ValidationMsg; type: 'error' | 'warning'; onNavigate?: (file: string, search?: string) => void; onGotoLine?: (file: string, line: number) => void }) {
  const color = type === 'error' ? 'border-red-400/30' : 'border-yellow-400/30'
  const permLink = msg.code?.startsWith('PERM_MISSING_')
  const scriptLink = msg.code?.startsWith('SCRIPT_') && !permLink
  const permSectionMap: Record<string, string> = {
    PERM_MISSING_PACKAGE: 'packages', PERM_MISSING_PIP: 'pip',
    PERM_MISSING_SERVICE: 'services', PERM_MISSING_COMMAND: 'commands',
    PERM_MISSING_USER: 'users', PERM_MISSING_PATH: 'paths',
    PERM_MISSING_URL: 'urls', PERM_MISSING_APT_REPO: 'apt_repos',
  }
  const permSection = (permLink && msg.code) ? (permSectionMap[msg.code] || '') : ''
  return (
    <div className={`border-l-2 ${color} px-2 py-1.5 my-1 text-xs font-mono`}>
      <div className="text-text-muted">{msg.file}{msg.line ? `:${msg.line}` : ''}</div>
      <div className={type === 'error' ? 'text-red-300' : 'text-yellow-300'}>{msg.message}</div>
      {permLink && onNavigate && (
        <button
          onClick={() => onNavigate('app.yml', permSection)}
          className="mt-1 text-primary hover:underline bg-transparent border-0 p-0 cursor-pointer text-xs font-mono"
        >→ Edit permissions.{permSection} in app.yml</button>
      )}
      {scriptLink && msg.line && onGotoLine && (
        <button
          onClick={() => onGotoLine('provision/install.py', msg.line!)}
          className="mt-1 text-primary hover:underline bg-transparent border-0 p-0 cursor-pointer text-xs font-mono"
        >→ Go to line {msg.line} in install.py</button>
      )}
    </div>
  )
}
