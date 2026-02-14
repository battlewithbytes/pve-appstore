import type { AppsResponse, AppDetail, CategoriesResponse, HealthResponse, JobsResponse, LogsResponse, InstallsResponse, InstallRequest, InstallDetail, Install, Job, ConfigDefaultsResponse, BrowseResponse, MountInfo, ExportResponse, ApplyResponse, ApplyPreviewResponse, AppStatusResponse, StacksResponse, StackDetail, StackCreateRequest, StackValidateResponse, EditRequest, ReconfigureRequest, Settings, SettingsUpdate, DiscoverResponse, DevAppsResponse, DevApp, DevTemplate, ValidationResult, DockerfileChainEvent, GitHubStatus, GitHubRepoInfo, PublishStatus, DevStacksResponse, DevStack, CatalogStacksResponse, CatalogStack, ZipImportResponse } from './types';

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

  reconfigure: (id: string, req: ReconfigureRequest) =>
    fetchJSON<Install>(`${BASE}/installs/${id}/reconfigure`, {
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

  discoverResources: () => fetchJSON<DiscoverResponse>(`${BASE}/settings/discover`),

  // --- Developer Mode ---

  devApps: () => fetchJSON<DevAppsResponse>(`${BASE}/dev/apps`),

  devCreateApp: (id: string, template?: string) =>
    fetchJSON<DevApp>(`${BASE}/dev/apps`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id, template }),
    }),

  devForkApp: (sourceId: string, newId: string) =>
    fetchJSON<DevApp>(`${BASE}/dev/fork`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ source_id: sourceId, new_id: newId }),
    }),

  devBranchApp: (sourceId: string) =>
    fetchJSON<DevApp>(`${BASE}/dev/branch`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ source_id: sourceId }),
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

  devGetFile: (id: string, path: string) =>
    fetchJSON<{ content: string }>(`${BASE}/dev/apps/${id}/file?path=${encodeURIComponent(path)}`),

  devSaveFile: (id: string, path: string, content: string) =>
    fetchJSON<{ status: string }>(`${BASE}/dev/apps/${id}/file`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path, content }),
    }),

  devDeleteFile: (id: string, path: string) =>
    fetchJSON<{ status: string; path: string }>(`${BASE}/dev/apps/${id}/file?path=${encodeURIComponent(path)}`, { method: 'DELETE' }),

  devDeleteApp: (id: string) =>
    fetchJSON<{ status: string }>(`${BASE}/dev/apps/${id}`, { method: 'DELETE' }),

  devValidate: (id: string) =>
    fetchJSON<ValidationResult>(`${BASE}/dev/apps/${id}/validate`, { method: 'POST' }),

  devDeploy: (id: string) =>
    fetchJSON<{ status: string; app_id: string; message: string }>(`${BASE}/dev/apps/${id}/deploy`, { method: 'POST' }),

  devUndeploy: (id: string) =>
    fetchJSON<{ status: string }>(`${BASE}/dev/apps/${id}/undeploy`, { method: 'POST' }),

  devIconUrl: (id: string) => `${BASE}/dev/apps/${id}/icon`,

  devSetIcon: (id: string, url: string) =>
    fetchJSON<{ status: string }>(`${BASE}/dev/apps/${id}/icon`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url }),
    }),

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

  // --- Developer GitHub Integration ---

  devGitHubStatus: () => fetchJSON<GitHubStatus>(`${BASE}/dev/github/status`),

  devGitHubConnect: (token: string) =>
    fetchJSON<{ status: string; user?: { login: string; name: string; avatar_url: string }; fork?: { full_name: string; clone_url: string; owner: string } }>(`${BASE}/dev/github/connect`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token }),
    }),

  devGitHubDisconnect: () =>
    fetchJSON<{ status: string }>(`${BASE}/dev/github/disconnect`, { method: 'POST' }),

  devGitHubRepoInfo: () => fetchJSON<GitHubRepoInfo>(`${BASE}/dev/github/repo-info`),

  devGitHubDeleteBranch: (branch: string) =>
    fetchJSON<{ status: string; branch: string }>(`${BASE}/dev/github/delete-branch`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ branch }),
    }),

  devPublishStatus: (id: string) =>
    fetchJSON<PublishStatus>(`${BASE}/dev/apps/${id}/publish-status`),

  devPublish: (id: string) =>
    fetchJSON<{ pr_url: string; pr_number: number; action?: string }>(`${BASE}/dev/apps/${id}/publish`, { method: 'POST' }),

  devImportDockerfileStream: async (
    payload: { name: string; url?: string; dockerfile?: string },
    onEvent: (e: DockerfileChainEvent) => void,
    signal?: AbortSignal,
  ): Promise<string> => {
    const resp = await fetch(`${BASE}/dev/import/dockerfile/stream`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
      signal,
    });
    if (!resp.ok) {
      const body = await resp.json().catch(() => ({}));
      throw new Error(body.error || `HTTP ${resp.status}`);
    }
    const reader = resp.body!.getReader();
    const decoder = new TextDecoder();
    let buffer = '';
    let appId = '';
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop()!;
      for (const line of lines) {
        if (line.startsWith('data: ')) {
          try {
            const event: DockerfileChainEvent = JSON.parse(line.slice(6));
            onEvent(event);
            if (event.type === 'complete' && event.app_id) appId = event.app_id;
          } catch { /* skip malformed */ }
        }
      }
    }
    return appId;
  },

  // --- Import ZIP ---

  devUploadFile: async (id: string, path: string, file: File): Promise<{ status: string; path: string; resized?: boolean }> => {
    const form = new FormData();
    form.append('file', file);
    form.append('path', path);
    const res = await fetch(`${BASE}/dev/apps/${id}/upload`, { method: 'POST', body: form });
    if (!res.ok) { const b = await res.json().catch(() => ({})); throw new Error(b.error || `HTTP ${res.status}`); }
    return res.json();
  },

  devImportZip: async (file: File): Promise<ZipImportResponse> => {
    const form = new FormData();
    form.append('file', file);
    const res = await fetch(`${BASE}/dev/import/zip`, { method: 'POST', body: form });
    if (!res.ok) { const b = await res.json().catch(() => ({})); throw new Error(b.error || `HTTP ${res.status}`); }
    return res.json();
  },

  // --- Developer Stacks ---

  devStacks: () => fetchJSON<DevStacksResponse>(`${BASE}/dev/stacks`),

  devCreateStack: (id: string, template?: string) =>
    fetchJSON<DevStack>(`${BASE}/dev/stacks`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id, template }),
    }),

  devGetStack: (id: string) => fetchJSON<DevStack>(`${BASE}/dev/stacks/${id}`),

  devSaveStackManifest: (id: string, content: string) =>
    fetchJSON<{ status: string }>(`${BASE}/dev/stacks/${id}/manifest`, {
      method: 'PUT',
      headers: { 'Content-Type': 'text/plain' },
      body: content,
    }),

  devDeleteStack: (id: string) =>
    fetchJSON<{ status: string }>(`${BASE}/dev/stacks/${id}`, { method: 'DELETE' }),

  devValidateStack: (id: string) =>
    fetchJSON<ValidationResult>(`${BASE}/dev/stacks/${id}/validate`, { method: 'POST' }),

  devDeployStack: (id: string) =>
    fetchJSON<{ status: string; stack_id: string; message: string }>(`${BASE}/dev/stacks/${id}/deploy`, { method: 'POST' }),

  devUndeployStack: (id: string) =>
    fetchJSON<{ status: string }>(`${BASE}/dev/stacks/${id}/undeploy`, { method: 'POST' }),

  devStackIconUrl: (id: string) => `${BASE}/dev/stacks/${id}/icon`,

  devExportStackUrl: (id: string) => `${BASE}/dev/stacks/${id}/export`,

  devStackPublishStatus: (id: string) =>
    fetchJSON<PublishStatus>(`${BASE}/dev/stacks/${id}/publish-status`),

  devPublishStack: (id: string) =>
    fetchJSON<{ pr_url: string; pr_number: number; action?: string }>(`${BASE}/dev/stacks/${id}/publish`, { method: 'POST' }),

  // --- Catalog Stacks ---

  catalogStacks: () => fetchJSON<CatalogStacksResponse>(`${BASE}/catalog-stacks`),

  catalogStack: (id: string) => fetchJSON<{ stack: CatalogStack; readme: string }>(`${BASE}/catalog-stacks/${id}`),

  installCatalogStack: (id: string, overrides?: { storage?: string; bridge?: string; cores?: number; memory_mb?: number; disk_gb?: number; hostname?: string }) =>
    fetchJSON<Job>(`${BASE}/catalog-stacks/${id}/install`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(overrides || {}),
    }),
};
