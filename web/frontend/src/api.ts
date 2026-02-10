import type { AppsResponse, AppDetail, CategoriesResponse, HealthResponse, JobsResponse, LogsResponse, InstallsResponse, InstallRequest, InstallDetail, Job, ConfigDefaultsResponse, BrowseResponse, MountInfo, ExportResponse, ApplyResponse, ApplyPreviewResponse, AppStatusResponse, StacksResponse, StackDetail, StackCreateRequest, StackValidateResponse, EditRequest, Settings, SettingsUpdate, DevAppsResponse, DevApp, DevTemplate, ValidationResult } from './types';

const BASE = '/api';

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `HTTP ${res.status}`);
  }
  return res.json();
}

export const api = {
  health: () => fetchJSON<HealthResponse>(`${BASE}/health`),

  apps: (params?: { q?: string; category?: string; sort?: string }) => {
    const sp = new URLSearchParams();
    if (params?.q) sp.set('q', params.q);
    if (params?.category) sp.set('category', params.category);
    if (params?.sort) sp.set('sort', params.sort);
    const qs = sp.toString();
    return fetchJSON<AppsResponse>(`${BASE}/apps${qs ? '?' + qs : ''}`);
  },

  app: (id: string) => fetchJSON<AppDetail>(`${BASE}/apps/${id}`),

  categories: () => fetchJSON<CategoriesResponse>(`${BASE}/categories`),

  appStatus: (id: string) => fetchJSON<AppStatusResponse>(`${BASE}/apps/${id}/status`),

  appReadme: async (id: string): Promise<string> => {
    const res = await fetch(`${BASE}/apps/${id}/readme`);
    if (!res.ok) return '';
    return res.text();
  },

  installApp: (id: string, req: InstallRequest) =>
    fetchJSON<Job>(`${BASE}/apps/${id}/install`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    }),

  jobs: () => fetchJSON<JobsResponse>(`${BASE}/jobs`),

  clearJobs: () => fetchJSON<{ deleted: number }>(`${BASE}/jobs`, { method: 'DELETE' }),

  job: (id: string) => fetchJSON<Job>(`${BASE}/jobs/${id}`),

  cancelJob: (id: string) =>
    fetchJSON<{ status: string; job_id: string }>(`${BASE}/jobs/${id}/cancel`, { method: 'POST' }),

  jobLogs: (id: string, after?: number) => {
    const qs = after ? `?after=${after}` : '';
    return fetchJSON<LogsResponse>(`${BASE}/jobs/${id}/logs${qs}`);
  },

  configDefaults: () => fetchJSON<ConfigDefaultsResponse>(`${BASE}/config/defaults`),

  browsePaths: (path?: string) => {
    const qs = path ? `?path=${encodeURIComponent(path)}` : '';
    return fetchJSON<BrowseResponse>(`${BASE}/browse/paths${qs}`);
  },

  browseStorages: () => fetchJSON<{ storages: string[] }>(`${BASE}/browse/storages`),

  browseMounts: () => fetchJSON<{ mounts: MountInfo[] }>(`${BASE}/browse/mounts`),

  browseMkdir: (path: string) =>
    fetchJSON<{ path: string; created: boolean }>(`${BASE}/browse/mkdir`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path }),
    }),

  installs: () => fetchJSON<InstallsResponse>(`${BASE}/installs`),

  installDetail: (id: string) => fetchJSON<InstallDetail>(`${BASE}/installs/${id}`),

  startContainer: (id: string) =>
    fetchJSON<{ status: string; install_id: string }>(`${BASE}/installs/${id}/start`, { method: 'POST' }),

  stopContainer: (id: string) =>
    fetchJSON<{ status: string; install_id: string }>(`${BASE}/installs/${id}/stop`, { method: 'POST' }),

  restartContainer: (id: string) =>
    fetchJSON<{ status: string; install_id: string }>(`${BASE}/installs/${id}/restart`, { method: 'POST' }),

  uninstall: (id: string, keepVolumes?: boolean) =>
    fetchJSON<Job>(`${BASE}/installs/${id}/uninstall`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ keep_volumes: keepVolumes }),
    }),

  purgeInstall: (id: string) =>
    fetchJSON<{ status: string }>(`${BASE}/installs/${id}`, { method: 'DELETE' }),

  reinstall: (id: string, overrides?: { cores?: number; memory_mb?: number; disk_gb?: number; storage?: string; bridge?: string; inputs?: Record<string, string> }) =>
    fetchJSON<Job>(`${BASE}/installs/${id}/reinstall`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(overrides || {}),
    }),

  update: (id: string, overrides?: { cores?: number; memory_mb?: number; disk_gb?: number; storage?: string; bridge?: string; inputs?: Record<string, string> }) =>
    fetchJSON<Job>(`${BASE}/installs/${id}/update`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(overrides || {}),
    }),

  editInstall: (id: string, req: EditRequest) =>
    fetchJSON<Job>(`${BASE}/installs/${id}/edit`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    }),

  configExport: () => fetchJSON<ExportResponse>(`${BASE}/config/export`),

  configExportDownload: () => {
    window.open(`${BASE}/config/export/download`, '_blank')
  },

  configApply: (recipes: InstallRequest[], stacks?: StackCreateRequest[]) =>
    fetchJSON<ApplyResponse>(`${BASE}/config/apply`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ recipes, stacks: stacks || [] }),
    }),

  configApplyPreview: async (text: string): Promise<ApplyPreviewResponse> => {
    const res = await fetch(`${BASE}/config/apply/preview`, {
      method: 'POST',
      headers: { 'Content-Type': 'text/plain' },
      body: text,
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.error || `HTTP ${res.status}`);
    }
    return res.json();
  },

  authCheck: () => fetchJSON<{ authenticated: boolean; auth_required: boolean }>(`${BASE}/auth/check`),

  login: (password: string) =>
    fetchJSON<{ status: string }>(`${BASE}/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password }),
    }),

  logout: () =>
    fetchJSON<{ status: string }>(`${BASE}/auth/logout`, { method: 'POST' }),

  terminalToken: () =>
    fetchJSON<{ token: string }>(`${BASE}/auth/terminal-token`, { method: 'POST' }),

  terminalUrl: (id: string, token: string) => {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${proto}//${window.location.host}${BASE}/installs/${id}/terminal?token=${encodeURIComponent(token)}`;
  },

  journalLogsUrl: (id: string, token: string) => {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${proto}//${window.location.host}${BASE}/installs/${id}/logs?token=${encodeURIComponent(token)}`;
  },

  // --- Stacks ---

  stacks: () => fetchJSON<StacksResponse>(`${BASE}/stacks`),

  stackDetail: (id: string) => fetchJSON<StackDetail>(`${BASE}/stacks/${id}`),

  createStack: (req: StackCreateRequest) =>
    fetchJSON<Job>(`${BASE}/stacks`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    }),

  validateStack: (req: StackCreateRequest) =>
    fetchJSON<StackValidateResponse>(`${BASE}/stacks/validate`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    }),

  startStack: (id: string) =>
    fetchJSON<{ status: string; stack_id: string }>(`${BASE}/stacks/${id}/start`, { method: 'POST' }),

  stopStack: (id: string) =>
    fetchJSON<{ status: string; stack_id: string }>(`${BASE}/stacks/${id}/stop`, { method: 'POST' }),

  restartStack: (id: string) =>
    fetchJSON<{ status: string; stack_id: string }>(`${BASE}/stacks/${id}/restart`, { method: 'POST' }),

  uninstallStack: (id: string) =>
    fetchJSON<Job>(`${BASE}/stacks/${id}/uninstall`, { method: 'POST' }),

  editStack: (id: string, req: EditRequest) =>
    fetchJSON<Job>(`${BASE}/stacks/${id}/edit`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    }),

  stackTerminalUrl: (id: string, token: string) => {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${proto}//${window.location.host}${BASE}/stacks/${id}/terminal?token=${encodeURIComponent(token)}`;
  },

  stackJournalLogsUrl: (id: string, token: string) => {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return `${proto}//${window.location.host}${BASE}/stacks/${id}/logs?token=${encodeURIComponent(token)}`;
  },

  // --- Settings ---

  settings: () => fetchJSON<Settings>(`${BASE}/settings`),

  updateSettings: (update: SettingsUpdate) =>
    fetchJSON<Settings>(`${BASE}/settings`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(update),
    }),

  // --- Developer Mode ---

  devApps: () => fetchJSON<DevAppsResponse>(`${BASE}/dev/apps`),

  devCreateApp: (id: string, template?: string) =>
    fetchJSON<DevApp>(`${BASE}/dev/apps`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id, template }),
    }),

  devGetApp: (id: string) => fetchJSON<DevApp>(`${BASE}/dev/apps/${id}`),

  devSaveManifest: (id: string, content: string) =>
    fetchJSON<{ status: string }>(`${BASE}/dev/apps/${id}/manifest`, {
      method: 'PUT',
      headers: { 'Content-Type': 'text/plain' },
      body: content,
    }),

  devSaveScript: (id: string, content: string) =>
    fetchJSON<{ status: string }>(`${BASE}/dev/apps/${id}/script`, {
      method: 'PUT',
      headers: { 'Content-Type': 'text/plain' },
      body: content,
    }),

  devSaveFile: (id: string, path: string, content: string) =>
    fetchJSON<{ status: string }>(`${BASE}/dev/apps/${id}/file`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path, content }),
    }),

  devDeleteApp: (id: string) =>
    fetchJSON<{ status: string }>(`${BASE}/dev/apps/${id}`, { method: 'DELETE' }),

  devValidate: (id: string) =>
    fetchJSON<ValidationResult>(`${BASE}/dev/apps/${id}/validate`, { method: 'POST' }),

  devDeploy: (id: string) =>
    fetchJSON<{ status: string; app_id: string; message: string }>(`${BASE}/dev/apps/${id}/deploy`, { method: 'POST' }),

  devUndeploy: (id: string) =>
    fetchJSON<{ status: string }>(`${BASE}/dev/apps/${id}/undeploy`, { method: 'POST' }),

  devExportUrl: (id: string) => `${BASE}/dev/apps/${id}/export`,

  devImportUnraid: (payload: { xml?: string; url?: string }) =>
    fetchJSON<DevApp>(`${BASE}/dev/import/unraid`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    }),

  devImportDockerfile: (payload: { name: string; dockerfile?: string; url?: string }) =>
    fetchJSON<DevApp>(`${BASE}/dev/import/dockerfile`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    }),

  devTemplates: () => fetchJSON<{ templates: DevTemplate[] }>(`${BASE}/dev/templates`),
};
