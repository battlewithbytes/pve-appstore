package engine

import "time"

// Job states matching the PRD state machine (section 9.4).
const (
	StateQueued             = "queued"
	StateValidateRequest    = "validate_request"
	StateValidateManifest   = "validate_manifest"
	StateValidatePlacement  = "validate_placement"
	StateValidateGPU        = "validate_gpu"
	StateAllocateCTID       = "allocate_ctid"
	StateCreateContainer    = "create_container"
	StateConfigureContainer = "configure_container"
	StateAttachGPU          = "attach_gpu"
	StateStartContainer     = "start_container"
	StateWaitForNetwork     = "wait_for_network"
	StateInstallBasePkgs    = "install_base_packages"
	StatePushAssets          = "push_assets"
	StateProvision          = "provision"
	StateHealthcheck        = "healthcheck"
	StateCollectOutputs     = "collect_outputs"
	StateCompleted          = "completed"
	StateFailed             = "failed"
	StateCancelled          = "cancelled"
)

// Job types
const (
	JobTypeInstall   = "install"
	JobTypeUninstall = "uninstall"
	JobTypeReinstall = "reinstall"
	JobTypeUpdate    = "update"
	JobTypeEdit      = "edit"
	JobTypeStack     = "stack"
)

// DevicePassthrough represents a host device to pass through to the container.
type DevicePassthrough struct {
	Path string `json:"path"`           // e.g. "/dev/dri/renderD128"
	GID  int    `json:"gid,omitempty"`  // optional group ID
	Mode string `json:"mode,omitempty"` // e.g. "0666"
}

// MountPoint represents a volume mount attached to a container.
type MountPoint struct {
	Index     int    `json:"index"`                    // 0, 1, 2... (mp0, mp1, mp2)
	Name      string `json:"name"`                     // from VolumeSpec.Name
	Type      string `json:"type"`                     // "volume" or "bind"
	MountPath string `json:"mount_path"`               // container path
	SizeGB    int    `json:"size_gb,omitempty"`         // only for type=volume
	VolumeID  string `json:"volume_id,omitempty"`       // Proxmox volume ID (type=volume only)
	HostPath  string `json:"host_path,omitempty"`       // host path (type=bind only)
	Storage   string `json:"storage,omitempty"`          // Proxmox storage for this volume
	ReadOnly  bool   `json:"read_only,omitempty"`
}

// Job represents an install/uninstall job tracked by the engine.
type Job struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	State        string            `json:"state"`
	AppID        string            `json:"app_id"`
	AppName      string            `json:"app_name"`
	CTID         int               `json:"ctid,omitempty"`
	Node         string            `json:"node"`
	Pool         string            `json:"pool"`
	Storage      string            `json:"storage"`
	Bridge       string            `json:"bridge"`
	Cores        int               `json:"cores"`
	MemoryMB     int               `json:"memory_mb"`
	DiskGB       int               `json:"disk_gb"`
	Hostname     string            `json:"hostname,omitempty"`
	IPAddress    string            `json:"ip_address,omitempty"`
	MACAddress   string            `json:"mac_address,omitempty"`
	OnBoot       bool              `json:"onboot"`
	Unprivileged bool              `json:"unprivileged"`
	Inputs       map[string]string `json:"inputs,omitempty"`
	Outputs      map[string]string  `json:"outputs,omitempty"`
	MountPoints  []MountPoint       `json:"mount_points,omitempty"`
	Devices      []DevicePassthrough `json:"devices,omitempty"`
	EnvVars      map[string]string  `json:"env_vars,omitempty"`
	ExtraTags    string             `json:"extra_tags,omitempty"`
	StackID      string             `json:"stack_id,omitempty"`
	Error        string             `json:"error,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
	CompletedAt  *time.Time         `json:"completed_at,omitempty"`
}

// LogEntry represents a single log line for a job.
type LogEntry struct {
	JobID     string    `json:"job_id"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
}

// Install represents a completed/active app installation.
type Install struct {
	ID           string              `json:"id"`
	AppID        string              `json:"app_id"`
	AppName      string              `json:"app_name"`
	AppVersion   string              `json:"app_version"`
	CTID         int                 `json:"ctid"`
	Node         string              `json:"node"`
	Pool         string              `json:"pool"`
	Storage      string              `json:"storage"`
	Bridge       string              `json:"bridge"`
	Cores        int                 `json:"cores"`
	MemoryMB     int                 `json:"memory_mb"`
	DiskGB       int                 `json:"disk_gb"`
	Hostname     string              `json:"hostname,omitempty"`
	IPAddress    string              `json:"ip_address,omitempty"`
	MACAddress   string              `json:"mac_address,omitempty"`
	OnBoot       bool                `json:"onboot"`
	Unprivileged bool                `json:"unprivileged"`
	Inputs       map[string]string   `json:"inputs,omitempty"`
	Outputs      map[string]string   `json:"outputs,omitempty"`
	MountPoints  []MountPoint        `json:"mount_points,omitempty"`
	Devices      []DevicePassthrough `json:"devices,omitempty"`
	EnvVars      map[string]string   `json:"env_vars,omitempty"`
	Status       string              `json:"status"` // running, stopped, uninstalled
	CreatedAt    time.Time           `json:"created_at"`
}

// InstallRequest is the input for starting a new install job.
type InstallRequest struct {
	AppID          string               `json:"app_id"`
	Storage        string               `json:"storage,omitempty"`
	Bridge         string               `json:"bridge,omitempty"`
	Cores          int                  `json:"cores,omitempty"`
	MemoryMB       int                  `json:"memory_mb,omitempty"`
	DiskGB         int                  `json:"disk_gb,omitempty"`
	Hostname       string               `json:"hostname,omitempty"`
	IPAddress      string               `json:"ip_address,omitempty"`
	MACAddress     string               `json:"mac_address,omitempty"`
	OnBoot         *bool                `json:"onboot,omitempty"`
	Unprivileged   *bool                `json:"unprivileged,omitempty"`
	Inputs         map[string]string    `json:"inputs,omitempty"`
	BindMounts     map[string]string    `json:"bind_mounts,omitempty"`      // vol-name -> host path
	ExtraMounts    []ExtraMountRequest  `json:"extra_mounts,omitempty"`     // user-added
	VolumeStorages map[string]string    `json:"volume_storages,omitempty"`  // vol-name -> storage
	MountPoints    []MountPoint         `json:"mount_points,omitempty"`     // for reattaching volumes
	Devices        []DevicePassthrough  `json:"devices,omitempty"`
	EnvVars        map[string]string    `json:"env_vars,omitempty"`
	ExtraTags      string               `json:"extra_tags,omitempty"`
	GPUProfile     string               `json:"gpu_profile,omitempty"`      // GPU passthrough profile
}

// ExtraMountRequest is a user-defined bind mount added at install time.
type ExtraMountRequest struct {
	HostPath  string `json:"host_path"`
	MountPath string `json:"mount_path"`
	ReadOnly  bool   `json:"read_only,omitempty"`
}

// ReinstallRequest is the input for reinstalling an uninstalled app with preserved volumes.
type ReinstallRequest struct {
	Cores    int               `json:"cores,omitempty"`
	MemoryMB int               `json:"memory_mb,omitempty"`
	DiskGB   int               `json:"disk_gb,omitempty"`
	Storage  string            `json:"storage,omitempty"`
	Bridge   string            `json:"bridge,omitempty"`
	Inputs   map[string]string `json:"inputs,omitempty"`
}

// EditRequest is the input for editing (recreating) an active install.
// Storage is excluded (volumes are tied to it). Disk can only grow.
type EditRequest struct {
	Cores    int               `json:"cores,omitempty"`
	MemoryMB int               `json:"memory_mb,omitempty"`
	DiskGB   int               `json:"disk_gb,omitempty"`
	Bridge   string            `json:"bridge,omitempty"`
	Inputs   map[string]string `json:"inputs,omitempty"`
}

// ReconfigureRequest is the input for in-place reconfiguration of an active install.
// Unlike EditRequest, this does NOT destroy and recreate the container.
// Resource changes (cores, memory) are applied via Proxmox API.
// Input changes trigger the app's configure() lifecycle method.
type ReconfigureRequest struct {
	Cores    int               `json:"cores,omitempty"`
	MemoryMB int               `json:"memory_mb,omitempty"`
	Inputs   map[string]string `json:"inputs,omitempty"`
}

// StackApp represents a single app within a multi-app stack.
type StackApp struct {
	AppID      string            `json:"app_id"`
	AppName    string            `json:"app_name"`
	AppVersion string            `json:"app_version"`
	Order      int               `json:"order"`
	Inputs     map[string]string `json:"inputs,omitempty"`
	Outputs    map[string]string `json:"outputs,omitempty"`
	Status     string            `json:"status"` // pending, provisioning, completed, failed
	Error      string            `json:"error,omitempty"`
}

// Stack represents a multi-app container instance.
type Stack struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	CTID         int                 `json:"ctid"`
	Node         string              `json:"node"`
	Pool         string              `json:"pool"`
	Storage      string              `json:"storage"`
	Bridge       string              `json:"bridge"`
	Cores        int                 `json:"cores"`
	MemoryMB     int                 `json:"memory_mb"`
	DiskGB       int                 `json:"disk_gb"`
	Hostname     string              `json:"hostname,omitempty"`
	IPAddress    string              `json:"ip_address,omitempty"`
	MACAddress   string              `json:"mac_address,omitempty"`
	OnBoot       bool                `json:"onboot"`
	Unprivileged bool                `json:"unprivileged"`
	OSTemplate   string              `json:"ostemplate"`
	Apps         []StackApp          `json:"apps"`
	MountPoints  []MountPoint        `json:"mount_points,omitempty"`
	Devices      []DevicePassthrough `json:"devices,omitempty"`
	EnvVars      map[string]string   `json:"env_vars,omitempty"`
	Status       string              `json:"status"` // running, stopped, uninstalled
	CreatedAt    time.Time           `json:"created_at"`
}

// StackCreateRequest is the input for creating a new multi-app stack.
type StackCreateRequest struct {
	Name         string               `json:"name"`
	Apps         []StackAppRequest    `json:"apps"`
	Storage      string               `json:"storage,omitempty"`
	Bridge       string               `json:"bridge,omitempty"`
	Cores        int                  `json:"cores,omitempty"`
	MemoryMB     int                  `json:"memory_mb,omitempty"`
	DiskGB       int                  `json:"disk_gb,omitempty"`
	Hostname     string               `json:"hostname,omitempty"`
	IPAddress    string               `json:"ip_address,omitempty"`
	MACAddress   string               `json:"mac_address,omitempty"`
	OnBoot       *bool                `json:"onboot,omitempty"`
	Unprivileged *bool                `json:"unprivileged,omitempty"`
	BindMounts   map[string]string    `json:"bind_mounts,omitempty"`
	ExtraMounts  []ExtraMountRequest  `json:"extra_mounts,omitempty"`
	VolumeStorages map[string]string  `json:"volume_storages,omitempty"`
	Devices      []DevicePassthrough  `json:"devices,omitempty"`
	EnvVars      map[string]string    `json:"env_vars,omitempty"`
}

// StackAppRequest defines per-app configuration in a stack creation request.
type StackAppRequest struct {
	AppID  string            `json:"app_id"`
	Inputs map[string]string `json:"inputs,omitempty"`
}

// StackDetail combines stored stack data with live container status.
type StackDetail struct {
	Stack
	IP   string                `json:"ip,omitempty"`
	Live *ContainerStatusDetail `json:"live,omitempty"`
}

// StackListItem extends Stack with lightweight live data for the list view.
type StackListItem struct {
	Stack
	IP     string                 `json:"ip,omitempty"`
	Uptime int64                  `json:"uptime"`
	Live   *ContainerStatusDetail `json:"live,omitempty"`
}
