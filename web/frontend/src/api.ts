import type { AppsResponse, AppDetail, CategoriesResponse, HealthResponse, JobsResponse, LogsResponse, InstallsResponse, InstallRequest, InstallDetail, Job, ConfigDefaultsResponse, BrowseResponse, MountInfo, ExportResponse, ApplyResponse, AppStatusResponse } from './types';

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

  reinstall: (id: string, overrides?: { cores?: number; memory_mb?: number; disk_gb?: number; storage?: string; bridge?: string; inputs?: Record<string, string> }) =>
    fetchJSON<Job>(`${BASE}/installs/${id}/reinstall`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(overrides || {}),
    }),

  configExport: () => fetchJSON<ExportResponse>(`${BASE}/config/export`),

  configExportDownload: () => {
    window.open(`${BASE}/config/export/download`, '_blank')
  },

  configApply: (recipes: InstallRequest[]) =>
    fetchJSON<ApplyResponse>(`${BASE}/config/apply`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ recipes }),
    }),

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
};
