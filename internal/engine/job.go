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
	StatePushAssets          = "push_assets"
	StateProvision          = "provision"
	StateHealthcheck        = "healthcheck"
	StateCollectOutputs     = "collect_outputs"
	StateCompleted          = "completed"
	StateFailed             = "failed"
)

// Job types
const (
	JobTypeInstall   = "install"
	JobTypeUninstall = "uninstall"
)

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
	Inputs       map[string]string `json:"inputs,omitempty"`
	Outputs      map[string]string `json:"outputs,omitempty"`
	Error        string            `json:"error,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	CompletedAt  *time.Time        `json:"completed_at,omitempty"`
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
	ID        string    `json:"id"`
	AppID     string    `json:"app_id"`
	AppName   string    `json:"app_name"`
	CTID      int       `json:"ctid"`
	Node      string    `json:"node"`
	Pool      string    `json:"pool"`
	Status    string    `json:"status"` // running, stopped, failed
	CreatedAt time.Time `json:"created_at"`
}

// InstallRequest is the input for starting a new install job.
type InstallRequest struct {
	AppID    string            `json:"app_id"`
	Storage  string            `json:"storage,omitempty"`
	Bridge   string            `json:"bridge,omitempty"`
	Cores    int               `json:"cores,omitempty"`
	MemoryMB int               `json:"memory_mb,omitempty"`
	DiskGB   int               `json:"disk_gb,omitempty"`
	Inputs   map[string]string `json:"inputs,omitempty"`
}
