export interface AppSummary {
  id: string;
  name: string;
  description: string;
  version: string;
  categories: string[];
  tags: string[];
  has_icon: boolean;
  official?: boolean;
  gpu_required: boolean;
  gpu_support?: string[];
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
}

export interface AppInput {
  key: string;
  label: string;
  type: 'string' | 'number' | 'boolean' | 'secret' | 'select';
  default?: unknown;
  required: boolean;
  validation?: {
    regex?: string;
    min?: number;
    max?: number;
    enum?: string[];
  };
  help?: string;
  description?: string;
  group?: string;
}

export interface StorageDetail {
  id: string;
  type: string;
  browsable: boolean;
  path?: string;
}

export interface ConfigDefaultsResponse {
  storages: string[];
  storage_details: StorageDetail[];
  bridges: string[];
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
  onboot: boolean;
  unprivileged: boolean;
  inputs?: Record<string, string>;
  outputs?: Record<string, string>;
  mount_points?: MountPoint[];
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
  onboot?: boolean;
  unprivileged?: boolean;
  inputs?: Record<string, string>;
  outputs?: Record<string, string>;
  mount_points?: MountPoint[];
  devices?: DevicePassthrough[];
  env_vars?: Record<string, string>;
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
  onboot?: boolean;
  unprivileged?: boolean;
  inputs?: Record<string, string>;
  bind_mounts?: Record<string, string>;
  extra_mounts?: { host_path: string; mount_path: string; read_only?: boolean }[];
  volume_storages?: Record<string, string>;
  devices?: DevicePassthrough[];
  env_vars?: Record<string, string>;
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

export interface ExportResponse {
  exported_at: string;
  node: string;
  recipes: ExportRecipe[];
  installs: Install[];
}

export interface ApplyResponse {
  jobs: { app_id: string; job_id: string }[];
}

export interface AppStatusResponse {
  installed: boolean;
  install_id?: string;
  install_status?: string;
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
