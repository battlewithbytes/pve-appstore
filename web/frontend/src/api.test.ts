import { describe, it, expect, beforeEach, vi } from 'vitest'

// Mock fetch globally
const mockFetch = vi.fn()
global.fetch = mockFetch

// Import the api module
import { api } from './api'

describe('API client', () => {
  beforeEach(() => {
    mockFetch.mockReset()
  })

  // ---------- fetchJSON core behavior ----------

  describe('fetchJSON', () => {
    it('returns parsed JSON on success', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'ok' }),
      })

      const result = await api.health()
      expect(result).toEqual({ status: 'ok' })
      expect(mockFetch).toHaveBeenCalledTimes(1)
    })

    it('throws on HTTP error with error message from body', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ error: 'Not found' }),
      })

      await expect(api.app('nonexistent')).rejects.toThrow('Not found')
    })

    it('throws with HTTP status when body has no error field', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        json: () => Promise.resolve({}),
      })

      await expect(api.health()).rejects.toThrow('HTTP 500')
    })

    it('throws with HTTP status when body JSON parsing fails', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        json: () => Promise.reject(new Error('invalid json')),
      })

      await expect(api.apps()).rejects.toThrow('HTTP 500')
    })
  })

  // ---------- App endpoints ----------

  describe('app endpoints', () => {
    it('fetches app list with no params', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ apps: [] }),
      })

      await api.apps()
      expect(mockFetch).toHaveBeenCalledWith('/api/apps', undefined)
    })

    it('fetches app list with search query', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ apps: [] }),
      })

      await api.apps({ q: 'nginx' })
      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/api/apps?')
      expect(url).toContain('q=nginx')
    })

    it('fetches app list with category filter', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ apps: [] }),
      })

      await api.apps({ category: 'media' })
      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('category=media')
    })

    it('fetches app list with sort param', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ apps: [] }),
      })

      await api.apps({ sort: 'name' })
      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('sort=name')
    })

    it('fetches single app by id', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'nginx', name: 'Nginx' }),
      })

      await api.app('nginx')
      expect(mockFetch).toHaveBeenCalledWith('/api/apps/nginx', undefined)
    })

    it('fetches app status', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ installed: false }),
      })

      await api.appStatus('nginx')
      expect(mockFetch).toHaveBeenCalledWith('/api/apps/nginx/status', undefined)
    })

    it('fetches app readme as text', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        text: () => Promise.resolve('# Hello'),
      })

      const result = await api.appReadme('nginx')
      expect(result).toBe('# Hello')
      expect(mockFetch).toHaveBeenCalledWith('/api/apps/nginx/readme')
    })

    it('returns empty string when readme fetch fails', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        text: () => Promise.resolve(''),
      })

      const result = await api.appReadme('nginx')
      expect(result).toBe('')
    })

    it('installs an app with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'job-1', status: 'running' }),
      })

      const req = { storage: 'local', bridge: 'vmbr0' }
      await api.installApp('nginx', req)
      expect(mockFetch).toHaveBeenCalledWith('/api/apps/nginx/install', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(req),
      })
    })
  })

  // ---------- Categories ----------

  describe('categories endpoint', () => {
    it('fetches categories', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ categories: ['media', 'networking'] }),
      })

      await api.categories()
      expect(mockFetch).toHaveBeenCalledWith('/api/categories', undefined)
    })
  })

  // ---------- Job endpoints ----------

  describe('job endpoints', () => {
    it('fetches jobs list', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ jobs: [] }),
      })

      await api.jobs()
      expect(mockFetch).toHaveBeenCalledWith('/api/jobs', undefined)
    })

    it('clears jobs with DELETE', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ deleted: 3 }),
      })

      await api.clearJobs()
      expect(mockFetch).toHaveBeenCalledWith('/api/jobs', { method: 'DELETE' })
    })

    it('fetches single job', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'job-1' }),
      })

      await api.job('job-1')
      expect(mockFetch).toHaveBeenCalledWith('/api/jobs/job-1', undefined)
    })

    it('cancels a job with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'cancelled', job_id: 'job-1' }),
      })

      await api.cancelJob('job-1')
      expect(mockFetch).toHaveBeenCalledWith('/api/jobs/job-1/cancel', { method: 'POST' })
    })

    it('fetches job logs', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ lines: [] }),
      })

      await api.jobLogs('job-1')
      expect(mockFetch).toHaveBeenCalledWith('/api/jobs/job-1/logs', undefined)
    })

    it('fetches job logs with after parameter', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ lines: [] }),
      })

      await api.jobLogs('job-1', 42)
      expect(mockFetch).toHaveBeenCalledWith('/api/jobs/job-1/logs?after=42', undefined)
    })
  })

  // ---------- Config endpoints ----------

  describe('config endpoints', () => {
    it('fetches config defaults', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ storage: 'local' }),
      })

      await api.configDefaults()
      expect(mockFetch).toHaveBeenCalledWith('/api/config/defaults', undefined)
    })

    it('exports config', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ recipes: [] }),
      })

      await api.configExport()
      expect(mockFetch).toHaveBeenCalledWith('/api/config/export', undefined)
    })

    it('applies config with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ jobs: [] }),
      })

      await api.configApply([], [])
      expect(mockFetch).toHaveBeenCalledWith('/api/config/apply', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ recipes: [], stacks: [] }),
      })
    })

    it('previews config apply with text body', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ recipes: [], valid: true }),
      })

      await api.configApplyPreview('yaml content here')
      expect(mockFetch).toHaveBeenCalledWith('/api/config/apply/preview', {
        method: 'POST',
        headers: { 'Content-Type': 'text/plain' },
        body: 'yaml content here',
      })
    })
  })

  // ---------- Browse endpoints ----------

  describe('browse endpoints', () => {
    it('browses paths without path param', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ entries: [] }),
      })

      await api.browsePaths()
      expect(mockFetch).toHaveBeenCalledWith('/api/browse/paths', undefined)
    })

    it('browses paths with path param', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ entries: [] }),
      })

      await api.browsePaths('/mnt/data')
      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('path=%2Fmnt%2Fdata')
    })

    it('browses storages', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ storages: [] }),
      })

      await api.browseStorages()
      expect(mockFetch).toHaveBeenCalledWith('/api/browse/storages', undefined)
    })

    it('browses mounts', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ mounts: [] }),
      })

      await api.browseMounts()
      expect(mockFetch).toHaveBeenCalledWith('/api/browse/mounts', undefined)
    })

    it('creates directory with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ path: '/mnt/new', created: true }),
      })

      await api.browseMkdir('/mnt/new')
      expect(mockFetch).toHaveBeenCalledWith('/api/browse/mkdir', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: '/mnt/new' }),
      })
    })
  })

  // ---------- Install endpoints ----------

  describe('install endpoints', () => {
    it('fetches installs list', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ installs: [] }),
      })

      await api.installs()
      expect(mockFetch).toHaveBeenCalledWith('/api/installs', undefined)
    })

    it('fetches install detail', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'inst-1' }),
      })

      await api.installDetail('inst-1')
      expect(mockFetch).toHaveBeenCalledWith('/api/installs/inst-1', undefined)
    })

    it('starts container with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'started', install_id: 'inst-1' }),
      })

      await api.startContainer('inst-1')
      expect(mockFetch).toHaveBeenCalledWith('/api/installs/inst-1/start', { method: 'POST' })
    })

    it('stops container with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'stopped', install_id: 'inst-1' }),
      })

      await api.stopContainer('inst-1')
      expect(mockFetch).toHaveBeenCalledWith('/api/installs/inst-1/stop', { method: 'POST' })
    })

    it('restarts container with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'restarted', install_id: 'inst-1' }),
      })

      await api.restartContainer('inst-1')
      expect(mockFetch).toHaveBeenCalledWith('/api/installs/inst-1/restart', { method: 'POST' })
    })

    it('uninstalls with volumes config', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'job-1' }),
      })

      await api.uninstall('inst-1', ['vol1'], ['/data'])
      expect(mockFetch).toHaveBeenCalledWith('/api/installs/inst-1/uninstall', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ keep_volumes: ['vol1'], delete_binds: ['/data'] }),
      })
    })

    it('purges install with DELETE', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'purged' }),
      })

      await api.purgeInstall('inst-1')
      expect(mockFetch).toHaveBeenCalledWith('/api/installs/inst-1', { method: 'DELETE' })
    })

    it('reinstalls with overrides', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'job-1' }),
      })

      const overrides = { cores: 4, memory_mb: 2048 }
      await api.reinstall('inst-1', overrides)
      expect(mockFetch).toHaveBeenCalledWith('/api/installs/inst-1/reinstall', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(overrides),
      })
    })

    it('updates install with overrides', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'job-1' }),
      })

      const overrides = { cores: 2 }
      await api.update('inst-1', overrides)
      expect(mockFetch).toHaveBeenCalledWith('/api/installs/inst-1/update', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(overrides),
      })
    })

    it('edits install with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'job-1' }),
      })

      const req = { cores: 4, memory_mb: 4096 }
      await api.editInstall('inst-1', req)
      expect(mockFetch).toHaveBeenCalledWith('/api/installs/inst-1/edit', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(req),
      })
    })

    it('reconfigures install with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'inst-1' }),
      })

      const req = { inputs: { key: 'value' } }
      await api.reconfigure('inst-1', req)
      expect(mockFetch).toHaveBeenCalledWith('/api/installs/inst-1/reconfigure', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(req),
      })
    })
  })

  // ---------- Auth endpoints ----------

  describe('auth endpoints', () => {
    it('checks auth status', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ authenticated: true, auth_required: true }),
      })

      const result = await api.authCheck()
      expect(result).toEqual({ authenticated: true, auth_required: true })
      expect(mockFetch).toHaveBeenCalledWith('/api/auth/check', undefined)
    })

    it('logs in with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'ok' }),
      })

      await api.login('mypassword')
      expect(mockFetch).toHaveBeenCalledWith('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password: 'mypassword' }),
      })
    })

    it('logs out with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'ok' }),
      })

      await api.logout()
      expect(mockFetch).toHaveBeenCalledWith('/api/auth/logout', { method: 'POST' })
    })

    it('requests terminal token with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ token: 'abc123' }),
      })

      const result = await api.terminalToken()
      expect(result.token).toBe('abc123')
      expect(mockFetch).toHaveBeenCalledWith('/api/auth/terminal-token', { method: 'POST' })
    })
  })

  // ---------- URL builder functions (no fetch) ----------

  describe('URL builders', () => {
    it('builds terminal URL', () => {
      const url = api.terminalUrl('inst-1', 'tok')
      expect(url).toContain('/api/installs/inst-1/terminal')
      expect(url).toContain('token=tok')
    })

    it('builds journal logs URL', () => {
      const url = api.journalLogsUrl('inst-1', 'tok')
      expect(url).toContain('/api/installs/inst-1/logs')
      expect(url).toContain('token=tok')
    })

    it('builds dev icon URL', () => {
      expect(api.devIconUrl('myapp')).toBe('/api/dev/apps/myapp/icon')
    })

    it('builds dev export URL', () => {
      expect(api.devExportUrl('myapp')).toBe('/api/dev/apps/myapp/export')
    })

    it('builds stack terminal URL', () => {
      const url = api.stackTerminalUrl('stk-1', 'tok')
      expect(url).toContain('/api/stacks/stk-1/terminal')
      expect(url).toContain('token=tok')
    })

    it('builds stack journal logs URL', () => {
      const url = api.stackJournalLogsUrl('stk-1', 'tok')
      expect(url).toContain('/api/stacks/stk-1/logs')
      expect(url).toContain('token=tok')
    })

    it('builds dev stack icon URL', () => {
      expect(api.devStackIconUrl('mystack')).toBe('/api/dev/stacks/mystack/icon')
    })

    it('builds dev stack export URL', () => {
      expect(api.devExportStackUrl('mystack')).toBe('/api/dev/stacks/mystack/export')
    })
  })

  // ---------- Stack endpoints ----------

  describe('stack endpoints', () => {
    it('fetches stacks list', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ stacks: [] }),
      })

      await api.stacks()
      expect(mockFetch).toHaveBeenCalledWith('/api/stacks', undefined)
    })

    it('fetches stack detail', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'stk-1' }),
      })

      await api.stackDetail('stk-1')
      expect(mockFetch).toHaveBeenCalledWith('/api/stacks/stk-1', undefined)
    })

    it('creates stack with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'job-1' }),
      })

      const req = { name: 'test-stack', apps: [] }
      await api.createStack(req)
      expect(mockFetch).toHaveBeenCalledWith('/api/stacks', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(req),
      })
    })

    it('validates stack with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ valid: true }),
      })

      const req = { name: 'test-stack', apps: [] }
      await api.validateStack(req)
      expect(mockFetch).toHaveBeenCalledWith('/api/stacks/validate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(req),
      })
    })

    it('starts stack with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'started', stack_id: 'stk-1' }),
      })

      await api.startStack('stk-1')
      expect(mockFetch).toHaveBeenCalledWith('/api/stacks/stk-1/start', { method: 'POST' })
    })

    it('stops stack with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'stopped', stack_id: 'stk-1' }),
      })

      await api.stopStack('stk-1')
      expect(mockFetch).toHaveBeenCalledWith('/api/stacks/stk-1/stop', { method: 'POST' })
    })

    it('restarts stack with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'restarted', stack_id: 'stk-1' }),
      })

      await api.restartStack('stk-1')
      expect(mockFetch).toHaveBeenCalledWith('/api/stacks/stk-1/restart', { method: 'POST' })
    })

    it('uninstalls stack with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'job-1' }),
      })

      await api.uninstallStack('stk-1')
      expect(mockFetch).toHaveBeenCalledWith('/api/stacks/stk-1/uninstall', { method: 'POST' })
    })

    it('edits stack with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'job-1' }),
      })

      const req = { cores: 4 }
      await api.editStack('stk-1', req)
      expect(mockFetch).toHaveBeenCalledWith('/api/stacks/stk-1/edit', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(req),
      })
    })
  })

  // ---------- Settings endpoints ----------

  describe('settings endpoints', () => {
    it('fetches settings', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ theme: 'dark' }),
      })

      await api.settings()
      expect(mockFetch).toHaveBeenCalledWith('/api/settings', undefined)
    })

    it('updates settings with PUT', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ theme: 'light' }),
      })

      const update = { theme: 'light' }
      await api.updateSettings(update)
      expect(mockFetch).toHaveBeenCalledWith('/api/settings', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(update),
      })
    })

    it('refreshes catalog with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'ok', app_count: 10 }),
      })

      await api.catalogRefresh()
      expect(mockFetch).toHaveBeenCalledWith('/api/catalog/refresh', { method: 'POST' })
    })

    it('discovers resources', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ storages: [], bridges: [] }),
      })

      await api.discoverResources()
      expect(mockFetch).toHaveBeenCalledWith('/api/settings/discover', undefined)
    })
  })

  // ---------- Developer endpoints ----------

  describe('developer endpoints', () => {
    it('fetches dev apps', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ apps: [] }),
      })

      await api.devApps()
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/apps', undefined)
    })

    it('creates dev app with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'myapp' }),
      })

      await api.devCreateApp('myapp', 'basic')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/apps', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: 'myapp', template: 'basic' }),
      })
    })

    it('forks dev app with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'forked' }),
      })

      await api.devForkApp('source', 'forked')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/fork', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ source_id: 'source', new_id: 'forked' }),
      })
    })

    it('branches dev app with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'branched' }),
      })

      await api.devBranchApp('source')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/branch', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ source_id: 'source' }),
      })
    })

    it('gets dev app', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'myapp' }),
      })

      await api.devGetApp('myapp')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/apps/myapp', undefined)
    })

    it('saves dev manifest with PUT text/plain', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'ok' }),
      })

      await api.devSaveManifest('myapp', 'name: test')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/apps/myapp/manifest', {
        method: 'PUT',
        headers: { 'Content-Type': 'text/plain' },
        body: 'name: test',
      })
    })

    it('saves dev script with PUT text/plain', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'ok' }),
      })

      await api.devSaveScript('myapp', '#!/bin/bash')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/apps/myapp/script', {
        method: 'PUT',
        headers: { 'Content-Type': 'text/plain' },
        body: '#!/bin/bash',
      })
    })

    it('gets dev file', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ content: 'hello' }),
      })

      await api.devGetFile('myapp', 'extra/readme.md')
      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/api/dev/apps/myapp/file?path=')
      expect(url).toContain(encodeURIComponent('extra/readme.md'))
    })

    it('saves dev file with PUT', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'ok' }),
      })

      await api.devSaveFile('myapp', 'extra/test.txt', 'content')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/apps/myapp/file', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: 'extra/test.txt', content: 'content' }),
      })
    })

    it('deletes dev file with DELETE', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'ok', path: 'extra/test.txt' }),
      })

      await api.devDeleteFile('myapp', 'extra/test.txt')
      const url = mockFetch.mock.calls[0][0] as string
      expect(url).toContain('/api/dev/apps/myapp/file?path=')
      expect(mockFetch.mock.calls[0][1]).toEqual({ method: 'DELETE' })
    })

    it('renames dev file with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'ok' }),
      })

      await api.devRenameFile('myapp', 'old.txt', 'new.txt')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/apps/myapp/file/rename', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ from: 'old.txt', to: 'new.txt' }),
      })
    })

    it('renames dev app with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'ok', new_id: 'renamed' }),
      })

      await api.devRenameApp('myapp', 'renamed')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/apps/myapp/rename', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ new_id: 'renamed' }),
      })
    })

    it('deletes dev app with DELETE', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'ok' }),
      })

      await api.devDeleteApp('myapp')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/apps/myapp', { method: 'DELETE' })
    })

    it('validates dev app with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ valid: true, errors: [] }),
      })

      await api.devValidate('myapp')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/apps/myapp/validate', { method: 'POST' })
    })

    it('deploys dev app with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'deployed', app_id: 'myapp', message: 'ok' }),
      })

      await api.devDeploy('myapp')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/apps/myapp/deploy', { method: 'POST' })
    })

    it('undeploys dev app with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'ok' }),
      })

      await api.devUndeploy('myapp')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/apps/myapp/undeploy', { method: 'POST' })
    })

    it('sets dev icon with PUT', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'ok' }),
      })

      await api.devSetIcon('myapp', 'https://example.com/icon.png')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/apps/myapp/icon', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url: 'https://example.com/icon.png' }),
      })
    })

    it('fetches dev templates', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ templates: [] }),
      })

      await api.devTemplates()
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/templates', undefined)
    })

    it('imports unraid template with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'imported' }),
      })

      await api.devImportUnraid({ xml: '<template/>' })
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/import/unraid', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ xml: '<template/>' }),
      })
    })

    it('imports dockerfile with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'imported' }),
      })

      await api.devImportDockerfile({ name: 'myapp', dockerfile: 'FROM node' })
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/import/dockerfile', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: 'myapp', dockerfile: 'FROM node' }),
      })
    })
  })

  // ---------- Developer GitHub endpoints ----------

  describe('developer GitHub endpoints', () => {
    it('fetches GitHub status', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ connected: false }),
      })

      await api.devGitHubStatus()
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/github/status', undefined)
    })

    it('connects GitHub with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'connected' }),
      })

      await api.devGitHubConnect('ghp_abc123')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/github/connect', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token: 'ghp_abc123' }),
      })
    })

    it('disconnects GitHub with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'ok' }),
      })

      await api.devGitHubDisconnect()
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/github/disconnect', { method: 'POST' })
    })

    it('fetches GitHub repo info', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ repo: 'test/repo' }),
      })

      await api.devGitHubRepoInfo()
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/github/repo-info', undefined)
    })

    it('deletes GitHub branch with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'ok', branch: 'feature' }),
      })

      await api.devGitHubDeleteBranch('feature')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/github/delete-branch', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ branch: 'feature' }),
      })
    })

    it('fetches publish status', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ published: false }),
      })

      await api.devPublishStatus('myapp')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/apps/myapp/publish-status', undefined)
    })

    it('publishes app with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ pr_url: 'https://github.com/pr/1', pr_number: 1 }),
      })

      await api.devPublish('myapp')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/apps/myapp/publish', { method: 'POST' })
    })
  })

  // ---------- Developer Stack endpoints ----------

  describe('developer stack endpoints', () => {
    it('fetches dev stacks', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ stacks: [] }),
      })

      await api.devStacks()
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/stacks', undefined)
    })

    it('creates dev stack with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'mystack' }),
      })

      await api.devCreateStack('mystack', 'basic')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/stacks', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: 'mystack', template: 'basic' }),
      })
    })

    it('gets dev stack', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'mystack' }),
      })

      await api.devGetStack('mystack')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/stacks/mystack', undefined)
    })

    it('saves stack manifest with PUT text/plain', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'ok' }),
      })

      await api.devSaveStackManifest('mystack', 'name: test')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/stacks/mystack/manifest', {
        method: 'PUT',
        headers: { 'Content-Type': 'text/plain' },
        body: 'name: test',
      })
    })

    it('deletes dev stack with DELETE', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'ok' }),
      })

      await api.devDeleteStack('mystack')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/stacks/mystack', { method: 'DELETE' })
    })

    it('validates dev stack with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ valid: true }),
      })

      await api.devValidateStack('mystack')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/stacks/mystack/validate', { method: 'POST' })
    })

    it('deploys dev stack with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'deployed', stack_id: 'mystack', message: 'ok' }),
      })

      await api.devDeployStack('mystack')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/stacks/mystack/deploy', { method: 'POST' })
    })

    it('undeploys dev stack with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'ok' }),
      })

      await api.devUndeployStack('mystack')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/stacks/mystack/undeploy', { method: 'POST' })
    })

    it('fetches stack publish status', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ published: false }),
      })

      await api.devStackPublishStatus('mystack')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/stacks/mystack/publish-status', undefined)
    })

    it('publishes stack with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ pr_url: 'https://github.com/pr/2', pr_number: 2 }),
      })

      await api.devPublishStack('mystack')
      expect(mockFetch).toHaveBeenCalledWith('/api/dev/stacks/mystack/publish', { method: 'POST' })
    })
  })

  // ---------- Catalog Stack endpoints ----------

  describe('catalog stack endpoints', () => {
    it('fetches catalog stacks', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ stacks: [] }),
      })

      await api.catalogStacks()
      expect(mockFetch).toHaveBeenCalledWith('/api/catalog-stacks', undefined)
    })

    it('fetches catalog stack detail', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ stack: {}, readme: '' }),
      })

      await api.catalogStack('media')
      expect(mockFetch).toHaveBeenCalledWith('/api/catalog-stacks/media', undefined)
    })

    it('installs catalog stack with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ id: 'job-1' }),
      })

      const overrides = { storage: 'local', cores: 4 }
      await api.installCatalogStack('media', overrides)
      expect(mockFetch).toHaveBeenCalledWith('/api/catalog-stacks/media/install', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(overrides),
      })
    })
  })

  // ---------- System endpoints ----------

  describe('system endpoints', () => {
    it('lists GPUs', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ gpus: [] }),
      })

      await api.listGPUs()
      expect(mockFetch).toHaveBeenCalledWith('/api/system/gpus', undefined)
    })

    it('checks for updates', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ available: false }),
      })

      await api.checkUpdate()
      expect(mockFetch).toHaveBeenCalledWith('/api/system/update-check', undefined)
    })

    it('applies update with POST', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve({ status: 'updated', version: '2.0.0' }),
      })

      await api.applyUpdate()
      expect(mockFetch).toHaveBeenCalledWith('/api/system/update', { method: 'POST' })
    })
  })
})
