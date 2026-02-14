import type { AppInput } from '../types'

export function formatUptime(seconds: number): string {
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`
}

export function formatBytesShort(bytes: number): string {
  if (bytes === 0) return '0B'
  const units = ['B', 'K', 'M', 'G', 'T']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(1)}${units[i]}`
}

export function groupInputs(inputs: AppInput[]): Record<string, AppInput[]> {
  const groups: Record<string, AppInput[]> = {}
  for (const inp of inputs) {
    const g = inp.group || 'General'
    if (!groups[g]) groups[g] = []
    groups[g].push(inp)
  }
  return groups
}
