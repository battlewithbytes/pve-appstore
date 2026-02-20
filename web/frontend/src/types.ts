export interface AppSummary {
  id: string;
  name: string;
  description: string;
  version: string;
  categories: string[];
  tags: string[];
  has_icon: boolean;
  official?: boolean;
  featured?: boolean;
  gpu_required: boolean;
  gpu_support?: string[];
  source?: string;
  shadowed_by?: string;
}

export interface AppDetail {
  id: string;
  name: string;
  description: string;
  overview?: string;
  version: string;
  categories: string[];
  tags: string[];
  homepage?: string;
  license?: string;
  official?: boolean;
  featured?: boolean;
  maintainers?: string[];
  lxc: {
    ostemplate: string;
    defaults: {
      unprivileged: boolean;
      cores: number;
      memory_mb: number;
      disk_gb: number;
      features?: string[];
      onboot?: boolean;
      require_static_ip?: boolean;
    };
  };
  inputs?: AppInput[];
  provisioning: {
    script: string;
    timeout_sec?: number;
  };
  outputs?: AppOutput[];
  volumes?: VolumeSpec[];
  gpu: {
    supported?: string[];
    required: boolean;
    profiles?: string[];
    notes?: string;
  };
  icon_path?: string;
  readme_path?: string;
  source?: string;
}

export interface AppInput {
  key: string;
  label: string;
  type: 'string' | 'number' | 'boolean' | 'secret' | 'select';
  default?: unknown;
  required: boolean;
  reconfigurable?: boolean;
  validation?: {
    regex?: string;
    format?: string;
    min?: number;
    max?: number;
    min_length?: number;
    max_length?: number;
    enum?: string[];
  };
  help?: string;
  description?: string;
  group?: string;
  show_when?: {
    input: string;
    values: string[];
  };
}

export interface StorageDetail {
  id: string;
  type: string;
  browsable: boolean;
  path?: string;
  total_gb?: number;
  used_gb?: number;
  available_gb?: number;
}

export interface BridgeDetail {
  name: string;
  cidr?: string;
  gateway?: string;
  ports?: string;
  comment?: string;
  vlan_aware?: boolean;
  vlans?: string;
}

export interface ConfigDefaultsResponse {
  storages: string[];
  storage_details: StorageDetail[];
  bridges: string[];
  bridge_details: BridgeDetail[];
  defaults: {
    cores: number;
    memory_mb: number;
    disk_gb: number;
  };
}

export interface DevicePassthrough {
  path: string;
  gid?: number;
  mode?: string;
}

export interface AppOutput {
  key: string;
  label: string;
  value: string;
}

export interface VolumeSpec {
  name: string;
  type: 'volume' | 'bind';
  mount_path: string;
  size_gb?: number;
  label: string;
  default_host_path?: string;
  required: boolean;
  read_only?: boolean;
  description?: string;
}

export interface MountPoint {
  index: number;
  name: string;
  type: string;
  mount_path: string;
  size_gb?: number;
  volume_id?: string;
  host_path?: string;
  storage?: string;
  read_only?: boolean;
}

export interface MountInfo {
  path: string;
  fs_type: string;
  device: string;
}

export interface BrowseEntry {
  name: string;
  path: string;
  is_dir: boolean;
}

export interface BrowseResponse {
  path: string;
  entries: BrowseEntry[];
}

export interface HealthResponse {
  status: string;
  version: string;
  node: string;
  app_count: number;
}

export interface AppsResponse {
  apps: AppSummary[];
  total: number;
}

export interface CategoriesResponse {
  categories: string[];
}

export interface Job {
  id: string;
  type: string;
  state: string;
  app_id: string;
  app_name: string;
  ctid: number;
  node: string;
  pool: string;
  storage: string;
  bridge: string;
  cores: number;
  memory_mb: number;
  disk_gb: number;
  hostname?: string;
  ip_address?: string;
  mac_address?: string;
  onboot: boolean;
  unprivileged: boolean;
  inputs?: Record<string, string>;
  outputs?: Record<string, string>;
  mount_points?: MountPoint[];
  cpu_pin?: string;
  error?: string;
  created_at: string;
  updated_at: string;
  completed_at?: string;
}

export interface LogEntry {
  job_id: string;
  timestamp: string;
  level: string;
  message: string;
}

export interface Install {
  id: string;
  app_id: string;
  app_name: string;
  app_version: string;
  ctid: number;
  node: string;
  pool: string;
  storage: string;
  bridge: string;
  cores: number;
  memory_mb: number;
  disk_gb: number;
  hostname?: string;
  ip_address?: string;
  mac_address?: string;
  onboot?: boolean;
  unprivileged?: boolean;
  inputs?: Record<string, string>;
  outputs?: Record<string, string>;
  mount_points?: MountPoint[];
  devices?: DevicePassthrough[];
  env_vars?: Record<string, string>;
  cpu_pin?: string;
  status: string;
  created_at: string;
}

export interface ContainerLiveStatus {
  status: string;
  uptime: number;
  cpu: number;
  cpus: number;
  mem: number;
  maxmem: number;
  disk: number;
  maxdisk: number;
  netin: number;
  netout: number;
}

export interface InstallDetail extends Install {
  ip?: string;
  live?: ContainerLiveStatus;
  catalog_version?: string;
  update_available: boolean;
}

export interface JobsResponse {
  jobs: Job[];
  total: number;
}

export interface LogsResponse {
  logs: LogEntry[];
  last_id: number;
}

export interface InstallListItem extends Install {
  ip?: string;
  uptime?: number;
  live?: ContainerLiveStatus;
  catalog_version?: string;
  update_available?: boolean;
}

export interface InstallsResponse {
  installs: InstallListItem[];
  total: number;
}

export interface InstallRequest {
  app_id?: string;
  storage?: string;
  bridge?: string;
  cores?: number;
  memory_mb?: number;
  disk_gb?: number;
  hostname?: string;
  ip_address?: string;
  mac_address?: string;
  onboot?: boolean;
  unprivileged?: boolean;
  inputs?: Record<string, string>;
  bind_mounts?: Record<string, string>;
  extra_mounts?: { host_path: string; mount_path: string; read_only?: boolean }[];
  volume_storages?: Record<string, string>;
  devices?: DevicePassthrough[];
  env_vars?: Record<string, string>;
  cpu_pin?: string;
  replace_existing?: boolean;
  keep_volumes?: string[];
}

export interface EditRequest {
  cores?: number;
  memory_mb?: number;
  disk_gb?: number;
  bridge?: string;
  inputs?: Record<string, string>;
  devices?: DevicePassthrough[] | null;
  cpu_pin?: string | null;
}

export interface ReconfigureRequest {
  cores?: number;
  memory_mb?: number;
  inputs?: Record<string, string>;
}

export interface ExportRecipe {
  app_id: string;
  storage: string;
  bridge: string;
  cores: number;
  memory_mb: number;
  disk_gb: number;
  hostname?: string;
  onboot?: boolean;
  unprivileged?: boolean;
  inputs?: Record<string, string>;
  devices?: DevicePassthrough[];
  env_vars?: Record<string, string>;
  ports?: { key: string; label: string; value: number; protocol: string }[];
  bind_mounts?: Record<string, string>;
  volume_storages?: Record<string, string>;
  extra_mounts?: { host_path: string; mount_path: string; read_only?: boolean }[];
}

export interface ExportStackRecipe {
  name: string;
  apps: { app_id: string; inputs?: Record<string, string> }[];
  storage: string;
  bridge: string;
  cores: number;
  memory_mb: number;
  disk_gb: number;
  hostname?: string;
  onboot?: boolean;
  unprivileged?: boolean;
  devices?: DevicePassthrough[];
  env_vars?: Record<string, string>;
  bind_mounts?: Record<string, string>;
  volume_storages?: Record<string, string>;
  extra_mounts?: { host_path: string; mount_path: string; read_only?: boolean }[];
}

export interface ExportResponse {
  exported_at: string;
  node: string;
  version?: string;
  recipes: ExportRecipe[];
  stacks?: ExportStackRecipe[];
  installs: Install[];
}

export interface ApplyResponse {
  jobs: { app_id: string; job_id: string }[];
  stack_jobs?: { name: string; job_id: string }[];
}

export interface ApplyPreviewResponse {
  recipes: ExportRecipe[];
  stacks: ExportStackRecipe[];
  errors: string[];
}

export interface AppStatusResponse {
  installed: boolean;
  install_id?: string;
  install_status?: string;
  app_source?: string;
  ctid?: number;
  job_active: boolean;
  job_id?: string;
  job_state?: string;
}

// --- Stacks ---

export interface StackApp {
  app_id: string;
  app_name: string;
  app_version: string;
  order: number;
  inputs?: Record<string, string>;
  outputs?: Record<string, string>;
  status: string;
  error?: string;
}

export interface Stack {
  id: string;
  name: string;
  ctid: number;
  node: string;
  pool: string;
  storage: string;
  bridge: string;
  cores: number;
  memory_mb: number;
  disk_gb: number;
  hostname?: string;
  ip_address?: string;
  mac_address?: string;
  onboot: boolean;
  unprivileged: boolean;
  ostemplate: string;
  apps: StackApp[];
  mount_points?: MountPoint[];
  devices?: DevicePassthrough[];
  env_vars?: Record<string, string>;
  status: string;
  created_at: string;
}

export interface StackDetail extends Stack {
  ip?: string;
  live?: ContainerLiveStatus;
}

export interface StackListItem extends Stack {
  ip?: string;
  uptime?: number;
  live?: ContainerLiveStatus;
}

export interface StacksResponse {
  stacks: StackListItem[];
  total: number;
}

export interface StackCreateRequest {
  name: string;
  apps: { app_id: string; inputs?: Record<string, string> }[];
  storage?: string;
  bridge?: string;
  cores?: number;
  memory_mb?: number;
  disk_gb?: number;
  hostname?: string;
  ip_address?: string;
  mac_address?: string;
  onboot?: boolean;
  unprivileged?: boolean;
  bind_mounts?: Record<string, string>;
  extra_mounts?: { host_path: string; mount_path: string; read_only?: boolean }[];
  volume_storages?: Record<string, string>;
  devices?: DevicePassthrough[];
  env_vars?: Record<string, string>;
}

export interface StackValidateResponse {
  valid: boolean;
  errors: string[];
  warnings: string[];
  recommended?: { cores: number; memory_mb: number; disk_gb: number };
  ostemplate?: string;
}

// --- Settings ---

export interface Settings {
  defaults: { cores: number; memory_mb: number; disk_gb: number };
  storages: string[];
  bridges: string[];
  developer: { enabled: boolean };
  service: { port: number };
  auth: { mode: string };
  catalog: {
    refresh: string;
    url: string;
    branch: string;
    app_count: number;
    stack_count: number;
    last_refresh?: string;
  };
  gpu: { enabled: boolean; policy: string; allowed_devices?: string[] };
}

export interface SettingsUpdate {
  defaults?: { cores?: number; memory_mb?: number; disk_gb?: number };
  storages?: string[];
  bridges?: string[];
  developer?: { enabled: boolean };
  catalog?: { refresh?: string };
  gpu?: { enabled?: boolean; policy?: string; allowed_devices?: string[] };
  auth?: { mode: string; password?: string };
}

export interface DiscoverResponse {
  storages: { id: string; type: string }[];
  bridges: BridgeDetail[];
}

// --- Developer Mode ---

export interface GitHubStatus {
  connected: boolean;
  user?: { login: string; name: string; avatar_url: string };
  fork?: { full_name: string; clone_url: string; owner: string };
}

export interface GitHubRepoInfo {
  upstream: { url: string; branch: string };
  fork: { full_name: string; url: string; clone_url: string };
  branches: { name: string; app_id: string; pr_url: string; pr_state: string }[] | null;
  local: { catalog_path: string; dev_apps_path: string };
}

export interface PublishStatus {
  ready: boolean;
  checks: Record<string, boolean>;
  published: boolean;
  pr_url?: string;
  pr_state?: string;
}

export interface DevAppMeta {
  id: string;
  name: string;
  version: string;
  description: string;
  status: string;
  has_icon: boolean;
  has_readme: boolean;
  github_branch?: string;
  github_pr_url?: string;
  test_install_id?: string;
  created_at: string;
  updated_at: string;
}

export interface DevFile {
  path: string;
  size: number;
  is_dir: boolean;
}

export interface DevApp extends DevAppMeta {
  manifest: string;
  script: string;
  readme: string;
  files: DevFile[];
  deployed: boolean;
}

export interface DevAppsResponse {
  apps: DevAppMeta[];
  total: number;
}

export interface DevTemplate {
  id: string;
  name: string;
  description: string;
  category: string;
}

export interface ValidationMsg {
  file: string;
  line?: number;
  message: string;
  code: string;
}

export interface ChecklistItem {
  label: string;
  passed: boolean;
}

export interface ValidationResult {
  valid: boolean;
  errors: ValidationMsg[];
  warnings: ValidationMsg[];
  checklist: ChecklistItem[];
}

// --- Developer Stacks ---

export interface DevStackMeta {
  id: string;
  name: string;
  version: string;
  description: string;
  status: string;
  app_count: number;
  has_icon: boolean;
  github_branch?: string;
  github_pr_url?: string;
  created_at: string;
  updated_at: string;
}

export interface DevStack extends DevStackMeta {
  manifest: string;
  deployed: boolean;
  files: DevFile[];
}

export interface DevStacksResponse {
  stacks: DevStackMeta[];
  total: number;
}

// --- Catalog Stacks ---

export interface CatalogStack {
  id: string;
  name: string;
  description: string;
  version: string;
  categories: string[];
  tags: string[];
  icon?: string;
  apps: { app_id: string; inputs?: Record<string, string> }[];
  lxc: {
    ostemplate: string;
    defaults: {
      cores: number;
      memory_mb: number;
      disk_gb: number;
    };
  };
  icon_path?: string;
  source?: string;
}

export interface CatalogStacksResponse {
  stacks: CatalogStack[];
  total: number;
}

export interface ZipImportResponse {
  type: 'app' | 'stack';
  id: string;
  app?: DevApp;
  stack?: DevStack;
}

// --- GPU Discovery ---

export interface GPUInfo {
  path: string;
  type: string;
  name: string;
}

export interface GPUInstallInfo {
  id: string;
  app_name?: string;
  name?: string;
  ctid: number;
  devices: { path: string }[];
}

export interface GPUDriverStatus {
  nvidia_driver_loaded: boolean;
  nvidia_version?: string;
  nvidia_libs_found: boolean;
  intel_driver_loaded: boolean;
  amd_driver_loaded: boolean;
}

export interface GPUsResponse {
  gpus: GPUInfo[];
  gpu_installs: GPUInstallInfo[];
  gpu_stacks: GPUInstallInfo[];
  driver_status: GPUDriverStatus;
}

// --- System Updates ---

export interface UpdateStatus {
  current: string;
  latest: string;
  available: boolean;
  release?: {
    version: string;
    published_at: string;
    url: string;
    download_url: string;
  };
  checked_at: string;
}

// --- Dockerfile Chain Resolution ---

export interface DockerfileChainEvent {
  type: 'fetching' | 'parsed' | 'terminal' | 'error' | 'merged' | 'complete';
  layer: number;
  image?: string;
  url?: string;
  message: string;
  packages?: number;
  ports?: number;
  volumes?: number;
  app_id?: string;
}
