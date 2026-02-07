export interface AppSummary {
  id: string;
  name: string;
  description: string;
  version: string;
  categories: string[];
  tags: string[];
  has_icon: boolean;
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

export interface ConfigDefaultsResponse {
  storage: string;
  bridge: string;
  defaults: {
    cores: number;
    memory_mb: number;
    disk_gb: number;
  };
}

export interface AppOutput {
  key: string;
  label: string;
  value: string;
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
  cores: number;
  memory_mb: number;
  disk_gb: number;
  inputs?: Record<string, string>;
  outputs?: Record<string, string>;
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
  ctid: number;
  node: string;
  pool: string;
  status: string;
  created_at: string;
}

export interface JobsResponse {
  jobs: Job[];
  total: number;
}

export interface LogsResponse {
  logs: LogEntry[];
  last_id: number;
}

export interface InstallsResponse {
  installs: Install[];
  total: number;
}

export interface InstallRequest {
  storage?: string;
  bridge?: string;
  cores?: number;
  memory_mb?: number;
  disk_gb?: number;
  inputs?: Record<string, string>;
}
