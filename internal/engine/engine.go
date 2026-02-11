// Package engine orchestrates install/uninstall jobs for the PVE App Store.
package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
	"github.com/battlewithbytes/pve-appstore/internal/config"
	"github.com/battlewithbytes/pve-appstore/internal/pct"
)

// ContainerStatusDetail holds detailed runtime information about a container.
type ContainerStatusDetail struct {
	Status  string  `json:"status"`
	Uptime  int64   `json:"uptime"`
	CPU     float64 `json:"cpu"`
	CPUs    int     `json:"cpus"`
	Mem     int64   `json:"mem"`
	MaxMem  int64   `json:"maxmem"`
	Disk    int64   `json:"disk"`
	MaxDisk int64   `json:"maxdisk"`
	NetIn   int64   `json:"netin"`
	NetOut  int64   `json:"netout"`
}

// StorageInfo holds resolved information about a Proxmox storage.
type StorageInfo struct {
	ID        string // Proxmox storage ID (e.g. "media-storage")
	Type      string // Storage type (e.g. "zfspool", "dir", "lvmthin")
	Path      string // Resolved filesystem path (empty if not browsable)
	Browsable bool   // true if the storage has a real filesystem path
}

// ContainerManager abstracts container lifecycle operations.
// API-based operations use context; shell-based operations (Exec, Push) do not.
type ContainerManager interface {
	AllocateCTID(ctx context.Context) (int, error)
	Create(ctx context.Context, opts CreateOptions) error
	Start(ctx context.Context, ctid int) error
	Stop(ctx context.Context, ctid int) error
	Shutdown(ctx context.Context, ctid int, timeout int) error
	Destroy(ctx context.Context, ctid int, keepVolumes ...bool) error
	Status(ctx context.Context, ctid int) (string, error)
	StatusDetail(ctx context.Context, ctid int) (*ContainerStatusDetail, error)
	ResolveTemplate(ctx context.Context, name, storage string) string
	Exec(ctid int, command []string) (*pct.ExecResult, error)
	ExecStream(ctid int, command []string, onLine func(line string)) (*pct.ExecResult, error)
	ExecScript(ctid int, scriptPath string, env map[string]string) (*pct.ExecResult, error)
	Push(ctid int, src, dst, perms string) error
	GetIP(ctid int) (string, error)
	GetConfig(ctx context.Context, ctid int) (map[string]interface{}, error)
	UpdateConfig(ctx context.Context, ctid int, params url.Values) error
	DetachMountPoints(ctx context.Context, ctid int, indexes []int) error
	GetStorageInfo(ctx context.Context, storageID string) (*StorageInfo, error)
	ConfigureDevices(ctid int, devices []DevicePassthrough) error
	MountHostPath(ctid int, mpIndex int, hostPath, containerPath string, readOnly bool) error
	AppendLXCConfig(ctid int, lines []string) error
}

// MountPointOption defines a mount point for container creation.
type MountPointOption struct {
	Index     int
	Type      string // "volume" or "bind"
	Storage   string // Proxmox storage (e.g. "local-lvm") — volume only
	SizeGB    int    // size for new volume (0 if reattaching existing)
	MountPath string
	VolumeID  string // non-empty when reattaching existing volume
	HostPath  string // host path — bind only
	ReadOnly  bool
}

// CreateOptions defines the parameters for creating a new container.
type CreateOptions struct {
	CTID         int
	OSTemplate   string
	Storage      string
	RootFSSize   int // GB
	Cores        int
	MemoryMB     int
	Bridge       string
	HWAddr       string // MAC address to preserve across recreates
	Hostname     string
	IPAddress    string
	Unprivileged bool
	Pool         string
	Features     []string
	OnBoot       bool
	Tags         string
	MountPoints  []MountPointOption
}

// gpuProfiles maps GPU profile names to device passthrough configurations.
var gpuProfiles = map[string][]DevicePassthrough{
	"dri-render": {{Path: "/dev/dri/renderD128", GID: 44, Mode: "0666"}},
	"nvidia-basic": {
		{Path: "/dev/nvidia0"},
		{Path: "/dev/nvidiactl"},
		{Path: "/dev/nvidia-uvm"},
	},
}

// Engine manages the job lifecycle.
type Engine struct {
	cfg     *config.Config
	catalog *catalog.Catalog
	store   *Store
	cm      ContainerManager
	mu      sync.Mutex
	ctidMu  sync.Mutex // serializes CTID allocation → container creation
	cancels map[string]context.CancelFunc
}

// New creates a new Engine, opening the SQLite database.
func New(cfg *config.Config, cat *catalog.Catalog, dataDir string, cm ContainerManager) (*Engine, error) {
	dbPath := filepath.Join(dataDir, "jobs.db")
	store, err := NewStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening job store: %w", err)
	}

	e := &Engine{
		cfg:     cfg,
		catalog: cat,
		store:   store,
		cm:      cm,
		cancels: make(map[string]context.CancelFunc),
	}

	// Recover orphaned jobs from previous run and clean up their containers
	if orphans, err := store.RecoverOrphanedJobs(); err != nil {
		fmt.Printf("  engine:  warning: orphan recovery failed: %v\n", err)
	} else if len(orphans) > 0 {
		fmt.Printf("  engine:  recovered %d orphaned job(s) from previous run\n", len(orphans))
		// Destroy containers that were left behind by interrupted jobs
		if cm != nil {
			for _, o := range orphans {
				if o.CTID > 0 {
					fmt.Printf("  engine:  destroying orphaned container %d (job %s)\n", o.CTID, o.ID)
					ctx := context.Background()
					_ = cm.Stop(ctx, o.CTID)
					if err := cm.Destroy(ctx, o.CTID); err != nil {
						fmt.Printf("  engine:  warning: failed to destroy container %d: %v\n", o.CTID, err)
					}
				}
			}
		}
	}

	return e, nil
}

// CancelJob cancels a running job by ID.
// It cancels the context and also stops the container to kill any running pct exec.
func (e *Engine) CancelJob(jobID string) error {
	e.mu.Lock()
	cancel, ok := e.cancels[jobID]
	e.mu.Unlock()
	if !ok {
		return fmt.Errorf("job %q is not running", jobID)
	}
	cancel()

	// Also stop the container to interrupt any running pct exec (e.g. pip install)
	job, err := e.store.GetJob(jobID)
	if err == nil && job.CTID > 0 {
		go func() {
			ctx := context.Background()
			_ = e.cm.Stop(ctx, job.CTID)
		}()
	}

	return nil
}

// StartContainer starts a stopped container for an existing install.
func (e *Engine) StartContainer(installID string) error {
	inst, err := e.store.GetInstall(installID)
	if err != nil {
		return fmt.Errorf("install %q not found", installID)
	}
	if inst.Status == "uninstalled" || inst.CTID == 0 {
		return fmt.Errorf("install %q has no active container", installID)
	}
	return e.cm.Start(context.Background(), inst.CTID)
}

// StopContainer gracefully stops a container for an existing install.
func (e *Engine) StopContainer(installID string) error {
	inst, err := e.store.GetInstall(installID)
	if err != nil {
		return fmt.Errorf("install %q not found", installID)
	}
	if inst.Status == "uninstalled" || inst.CTID == 0 {
		return fmt.Errorf("install %q has no active container", installID)
	}
	return e.cm.Shutdown(context.Background(), inst.CTID, 30)
}

// RestartContainer stops then starts a container for an existing install.
func (e *Engine) RestartContainer(installID string) error {
	inst, err := e.store.GetInstall(installID)
	if err != nil {
		return fmt.Errorf("install %q not found", installID)
	}
	if inst.Status == "uninstalled" || inst.CTID == 0 {
		return fmt.Errorf("install %q has no active container", installID)
	}
	ctx := context.Background()
	if err := e.cm.Shutdown(ctx, inst.CTID, 30); err != nil {
		// Fallback to force stop
		_ = e.cm.Stop(ctx, inst.CTID)
	}
	time.Sleep(2 * time.Second)
	return e.cm.Start(ctx, inst.CTID)
}

// Close closes the engine's resources.
func (e *Engine) Close() error {
	return e.store.Close()
}

// ErrDuplicate is returned when an install is blocked by an existing install or active job.
type ErrDuplicate struct {
	Message   string
	InstallID string // non-empty if blocked by existing install
	JobID     string // non-empty if blocked by active job
}

func (e *ErrDuplicate) Error() string { return e.Message }

// HasActiveInstallForApp checks if the given app has a non-uninstalled install.
func (e *Engine) HasActiveInstallForApp(appID string) (*Install, bool) {
	return e.store.HasActiveInstallForApp(appID)
}

// HasActiveJobForApp checks if the given app has a non-terminal install job.
func (e *Engine) HasActiveJobForApp(appID string) (*Job, bool) {
	return e.store.HasActiveJobForApp(appID)
}

// StartInstall creates a new install job and runs it asynchronously.
func (e *Engine) StartInstall(req InstallRequest) (*Job, error) {
	// Look up the app
	app, ok := e.catalog.Get(req.AppID)
	if !ok {
		return nil, fmt.Errorf("app %q not found in catalog", req.AppID)
	}

	// Prevent duplicate installs
	if inst, exists := e.store.HasActiveInstallForApp(req.AppID); exists {
		return nil, &ErrDuplicate{
			Message:   fmt.Sprintf("app %q is already installed (CTID %d, status: %s)", req.AppID, inst.CTID, inst.Status),
			InstallID: inst.ID,
		}
	}
	if job, exists := e.store.HasActiveJobForApp(req.AppID); exists {
		return nil, &ErrDuplicate{
			Message: fmt.Sprintf("app %q already has an active install job (state: %s)", req.AppID, job.State),
			JobID:   job.ID,
		}
	}

	// Validate request inputs
	if err := ValidateHostname(req.Hostname); err != nil {
		return nil, err
	}
	if err := ValidateBridge(req.Bridge); err != nil {
		return nil, err
	}
	if err := ValidateIPAddress(req.IPAddress); err != nil {
		return nil, err
	}
	if err := ValidateTags(req.ExtraTags); err != nil {
		return nil, err
	}
	if err := ValidateEnvVars(req.EnvVars); err != nil {
		return nil, err
	}
	// Validate bind mount paths
	for _, hp := range req.BindMounts {
		if err := ValidateBindMountPath(hp); err != nil {
			return nil, err
		}
	}
	for _, em := range req.ExtraMounts {
		if err := ValidateBindMountPath(em.HostPath); err != nil {
			return nil, err
		}
	}

	// Apply defaults from config, then from manifest, then from request
	storage := e.cfg.Storages[0]
	if req.Storage != "" {
		storage = req.Storage
	}
	bridge := e.cfg.Bridges[0]
	if req.Bridge != "" {
		bridge = req.Bridge
	}
	cores := e.cfg.Defaults.Cores
	if app.LXC.Defaults.Cores > 0 {
		cores = app.LXC.Defaults.Cores
	}
	if req.Cores > 0 {
		cores = req.Cores
	}
	memoryMB := e.cfg.Defaults.MemoryMB
	if app.LXC.Defaults.MemoryMB > 0 {
		memoryMB = app.LXC.Defaults.MemoryMB
	}
	if req.MemoryMB > 0 {
		memoryMB = req.MemoryMB
	}
	diskGB := e.cfg.Defaults.DiskGB
	if app.LXC.Defaults.DiskGB > 0 {
		diskGB = app.LXC.Defaults.DiskGB
	}
	if req.DiskGB > 0 {
		diskGB = req.DiskGB
	}

	// Hostname defaults to app ID
	hostname := req.Hostname
	if hostname == "" {
		hostname = req.AppID
	}
	// OnBoot/Unprivileged: request overrides manifest defaults
	onboot := app.LXC.Defaults.OnBoot
	if req.OnBoot != nil {
		onboot = *req.OnBoot
	}
	unprivileged := app.LXC.Defaults.Unprivileged
	if req.Unprivileged != nil {
		unprivileged = *req.Unprivileged
	}

	now := time.Now()
	job := &Job{
		ID:           generateID(),
		Type:         JobTypeInstall,
		State:        StateQueued,
		AppID:        req.AppID,
		AppName:      app.Name,
		Node:         e.cfg.NodeName,
		Pool:         e.cfg.Pool,
		Storage:      storage,
		Bridge:       bridge,
		Cores:        cores,
		MemoryMB:     memoryMB,
		DiskGB:       diskGB,
		Hostname:     hostname,
		IPAddress:    req.IPAddress,
		OnBoot:       onboot,
		Unprivileged: unprivileged,
		ExtraTags:    req.ExtraTags,
		Inputs:       req.Inputs,
		Outputs:      make(map[string]string),
		Devices:      nil, // Devices are now determined from GPUProfile, not directly from request
		EnvVars:      req.EnvVars,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Determine GPU devices from request profile or manifest profiles.
	// Validate that device nodes actually exist on the host before adding.
	if req.GPUProfile != "" && e.cfg.GPU.Policy != config.GPUPolicyNone {
		if devs, ok := gpuProfiles[req.GPUProfile]; ok {
			available, missing := validateGPUDevices(devs)
			if len(missing) > 0 && app.GPU.Required {
				return nil, fmt.Errorf("GPU profile %q requires device(s) not found on host: %s", req.GPUProfile, strings.Join(missing, ", "))
			}
			job.Devices = available
		} else {
			return nil, fmt.Errorf("invalid GPU profile requested: %s", req.GPUProfile)
		}
	} else if len(app.GPU.Profiles) > 0 && e.cfg.GPU.Policy != config.GPUPolicyNone {
		// Auto-select from manifest if no profile requested
		for _, profile := range app.GPU.Profiles {
			if devs, ok := gpuProfiles[profile]; ok {
				available, missing := validateGPUDevices(devs)
				if len(missing) > 0 && app.GPU.Required {
					return nil, fmt.Errorf("GPU required but device(s) not found on host: %s", strings.Join(missing, ", "))
				}
				job.Devices = available
				break // use first matching profile
			}
			// If app.GPU.Required is true, and the profile is not found, it should error here,
			// but we can't do that as the loop `break`s after the first matching profile.
			// The manifest validation should catch invalid profiles anyway.
		}
	}

	if job.Inputs == nil {
		job.Inputs = make(map[string]string)
	}

	// Apply input defaults from manifest
	for _, input := range app.Inputs {
		if _, exists := job.Inputs[input.Key]; !exists && input.Default != nil {
			job.Inputs[input.Key] = fmt.Sprintf("%v", input.Default)
		}
	}

	// Build mount points from manifest volumes + request bind mounts + extra mounts
	mpIndex := 0
	for _, vol := range app.Volumes {
		volType := vol.Type
		if volType == "" {
			volType = "volume"
		}
		mp := MountPoint{
			Index:     mpIndex,
			Name:      vol.Name,
			Type:      volType,
			MountPath: vol.MountPath,
			SizeGB:    vol.SizeGB,
			ReadOnly:  vol.ReadOnly,
		}
		// For volume types: allow user override to bind mount, or per-volume storage
		if volType == "volume" {
			if hp, ok := req.BindMounts[vol.Name]; ok && hp != "" {
				// User wants to use a host path instead of Proxmox volume
				mp.Type = "bind"
				mp.HostPath = hp
				mp.SizeGB = 0
				mp.Storage = ""
			} else if vs, ok := req.VolumeStorages[vol.Name]; ok && vs != "" {
				mp.Storage = vs
			}
		}
		if volType == "bind" {
			// Get host path from request, fall back to default.
			// An explicit empty string in BindMounts ("media": "") clears the
			// default, allowing optional bind mounts to be skipped at request time.
			if hp, ok := req.BindMounts[vol.Name]; ok {
				mp.HostPath = hp // may be empty to clear default
			} else if vol.DefaultHostPath != "" {
				mp.HostPath = vol.DefaultHostPath
			}
			// Skip optional bind mounts with no host path
			if !vol.Required && mp.HostPath == "" {
				continue
			}
		}
		job.MountPoints = append(job.MountPoints, mp)
		mpIndex++
	}
	// Add extra user-defined mounts
	for _, em := range req.ExtraMounts {
		if em.HostPath == "" || em.MountPath == "" {
			continue
		}
		job.MountPoints = append(job.MountPoints, MountPoint{
			Index:     mpIndex,
			Name:      fmt.Sprintf("extra-%d", mpIndex),
			Type:      "bind",
			MountPath: em.MountPath,
			HostPath:  em.HostPath,
			ReadOnly:  em.ReadOnly,
		})
		mpIndex++
	}

	if err := e.store.CreateJob(job); err != nil {
		return nil, fmt.Errorf("creating job: %w", err)
	}

	// Run asynchronously with cancel support
	ctx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	e.cancels[job.ID] = cancel
	e.mu.Unlock()

	go func() {
		defer func() {
			e.mu.Lock()
			delete(e.cancels, job.ID)
			e.mu.Unlock()
			cancel()
		}()
		e.runInstall(ctx, job)
	}()

	return job, nil
}

// Uninstall stops and destroys a container for an installed app.
// PurgeInstall deletes an "uninstalled" install record from the database.
// Only works on installs with status "uninstalled" (container already destroyed).
func (e *Engine) PurgeInstall(installID string) error {
	inst, err := e.store.GetInstall(installID)
	if err != nil {
		return fmt.Errorf("install %q not found", installID)
	}
	if inst.Status != "uninstalled" {
		return fmt.Errorf("can only purge uninstalled records (status is %q)", inst.Status)
	}
	_, err = e.store.db.Exec("DELETE FROM installs WHERE id=?", inst.ID)
	return err
}

// If keepVolumes is true, mount point volumes are detached before destroy and preserved.
func (e *Engine) Uninstall(installID string, keepVolumes bool) (*Job, error) {
	// Find the install
	inst, err := e.store.GetInstall(installID)
	if err != nil {
		return nil, fmt.Errorf("install %q not found", installID)
	}

	now := time.Now()
	job := &Job{
		ID:        generateID(),
		Type:      JobTypeUninstall,
		State:     StateQueued,
		AppID:     inst.AppID,
		AppName:   inst.AppName,
		CTID:      inst.CTID,
		Node:      inst.Node,
		Pool:      inst.Pool,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := e.store.CreateJob(job); err != nil {
		return nil, fmt.Errorf("creating uninstall job: %w", err)
	}

	go e.runUninstall(job, inst, keepVolumes)

	return job, nil
}

func (e *Engine) runUninstall(job *Job, inst *Install, keepVolumes bool) {
	ctx := &installContext{engine: e, job: job}

	ctx.info("Starting uninstall of %s (CTID %d, keepVolumes=%v)", inst.AppName, inst.CTID, keepVolumes)

	bgCtx := context.Background()
	ctGone := false // set when we detect the container no longer exists

	// Stop container
	ctx.transition("stopping")
	ctx.info("Stopping container %d...", inst.CTID)

	// Force stop — no need for graceful shutdown during uninstall
	if err := e.cm.Stop(bgCtx, inst.CTID); err != nil {
		if isContainerGone(err) {
			ctx.info("Container %d already removed — skipping destroy", inst.CTID)
			ctGone = true
		} else {
			ctx.warn("Force stop error: %v", err)
		}
	}
	if !ctGone {
		ctx.info("Container %d stopped", inst.CTID)
	}

	// If keeping volumes, detach managed volumes before destroy (bind mounts don't need detaching)
	if !ctGone && keepVolumes && len(inst.MountPoints) > 0 {
		var managedIndexes []int
		for _, mp := range inst.MountPoints {
			if mp.Type == "volume" {
				managedIndexes = append(managedIndexes, mp.Index)
			}
		}

		if len(managedIndexes) > 0 {
			ctx.transition("detaching_volumes")

			// Read fresh config to get current volume IDs
			if config, err := e.cm.GetConfig(bgCtx, inst.CTID); err == nil {
				for i := range inst.MountPoints {
					if inst.MountPoints[i].Type != "volume" {
						continue
					}
					key := fmt.Sprintf("mp%d", inst.MountPoints[i].Index)
					if val, ok := config[key]; ok {
						if valStr, ok := val.(string); ok {
							parts := strings.SplitN(valStr, ",", 2)
							if len(parts) > 0 {
								inst.MountPoints[i].VolumeID = parts[0]
							}
						}
					}
				}
			}

			// Detach only managed volume mount points from config
			for _, mp := range inst.MountPoints {
				if mp.Type == "volume" {
					ctx.info("Detaching volume %s (%s): %s", mp.Name, fmt.Sprintf("mp%d", mp.Index), mp.VolumeID)
				}
			}
			if err := e.cm.DetachMountPoints(bgCtx, inst.CTID, managedIndexes); err != nil {
				if isContainerGone(err) {
					ctx.info("Container %d config already removed — skipping detach", inst.CTID)
					ctGone = true
				} else {
					ctx.warn("Failed to detach mount points: %v — volumes may be destroyed", err)
				}
			}
		}
	}

	// Destroy container with retries (container may need a moment to fully stop)
	if !ctGone {
		ctx.transition("destroying")
		ctx.info("Destroying container %d...", inst.CTID)
		var destroyErr error
		for attempt := 0; attempt < 5; attempt++ {
			if attempt > 0 {
				time.Sleep(5 * time.Second)
			}
			destroyErr = e.cm.Destroy(bgCtx, inst.CTID, keepVolumes)
			if destroyErr == nil {
				break
			}
			if isContainerGone(destroyErr) {
				ctx.info("Container %d already removed", inst.CTID)
				destroyErr = nil
				break
			}
			ctx.warn("Destroy attempt %d failed: %v", attempt+1, destroyErr)
		}
		if destroyErr != nil {
			ctx.log("error", "Failed to destroy container after retries: %v", destroyErr)
			job.State = StateFailed
			job.Error = fmt.Sprintf("destroy: %v", destroyErr)
			now := time.Now()
			job.UpdatedAt = now
			job.CompletedAt = &now
			e.store.UpdateJob(job)
			return
		}
	}

	// Only preserve install record if there are managed volumes worth keeping
	hasManagedVolumes := false
	for _, mp := range inst.MountPoints {
		if mp.Type == "volume" {
			hasManagedVolumes = true
			break
		}
	}
	if keepVolumes && hasManagedVolumes {
		// Preserve install record with "uninstalled" status and volume IDs
		inst.Status = "uninstalled"
		inst.CTID = 0
		e.store.UpdateInstall(inst)
		ctx.info("Install record preserved with managed volume(s)")
	} else {
		// Remove install record entirely
		e.store.db.Exec("DELETE FROM installs WHERE id=?", inst.ID)
	}

	ctx.transition(StateCompleted)
	now := time.Now()
	job.CompletedAt = &now
	e.store.UpdateJob(job)
	ctx.info("Uninstall complete. Container %d destroyed.", inst.CTID)
}

// ClearJobs deletes all terminal (completed/failed/cancelled) jobs and their logs.
func (e *Engine) ClearJobs() (int64, error) {
	return e.store.ClearTerminalJobs()
}

// GetJob returns a job by ID.
func (e *Engine) GetJob(id string) (*Job, error) {
	return e.store.GetJob(id)
}

// ListJobs returns all jobs.
func (e *Engine) ListJobs() ([]*Job, error) {
	return e.store.ListJobs()
}

// GetLogs returns all logs for a job.
func (e *Engine) GetLogs(jobID string) ([]*LogEntry, error) {
	return e.store.GetLogs(jobID)
}

// GetLogsSince returns logs after a given cursor.
func (e *Engine) GetLogsSince(jobID string, afterID int) ([]*LogEntry, int, error) {
	return e.store.GetLogsSince(jobID, afterID)
}

// InstallDetail combines stored install data with live container status.
type InstallDetail struct {
	Install
	IP              string                `json:"ip,omitempty"`
	Live            *ContainerStatusDetail `json:"live,omitempty"`
	CatalogVersion  string                `json:"catalog_version,omitempty"`
	UpdateAvailable bool                  `json:"update_available"`
}

// GetInstall returns a single install by ID.
func (e *Engine) GetInstall(id string) (*Install, error) {
	return e.store.GetInstall(id)
}

// GetInstallDetail returns an install enriched with live data.
func (e *Engine) GetInstallDetail(id string) (*InstallDetail, error) {
	inst, err := e.store.GetInstall(id)
	if err != nil {
		return nil, err
	}

	detail := &InstallDetail{Install: *inst}

	// Only fetch live data for active installs (not uninstalled)
	if inst.Status != "uninstalled" && inst.CTID > 0 {
		ctx := context.Background()

		// Fetch live status
		if sd, err := e.cm.StatusDetail(ctx, inst.CTID); err == nil {
			detail.Live = sd
			detail.Status = sd.Status
		}

		// Fetch IP
		if ip, err := e.cm.GetIP(inst.CTID); err == nil && ip != "" {
			detail.IP = ip
		}
	}

	// Check catalog version
	if app, ok := e.catalog.Get(inst.AppID); ok {
		detail.CatalogVersion = app.Version
		if inst.AppVersion != "" && isNewerVersion(app.Version, inst.AppVersion) {
			detail.UpdateAvailable = true
		}
	}

	return detail, nil
}

// ListInstalls returns all installations.
func (e *Engine) ListInstalls() ([]*Install, error) {
	return e.store.ListInstalls()
}

// InstallListItem extends Install with lightweight live data for the list view.
type InstallListItem struct {
	Install
	IP              string                 `json:"ip,omitempty"`
	Uptime          int64                  `json:"uptime"`
	Live            *ContainerStatusDetail `json:"live,omitempty"`
	CatalogVersion  string                 `json:"catalog_version,omitempty"`
	UpdateAvailable bool                   `json:"update_available,omitempty"`
}

// ListInstallsLive returns all installations with live status refreshed from Proxmox.
func (e *Engine) ListInstallsLive() ([]*Install, error) {
	installs, err := e.store.ListInstalls()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	for _, inst := range installs {
		if inst.Status == "uninstalled" || inst.CTID == 0 {
			continue
		}
		if status, err := e.cm.Status(ctx, inst.CTID); err == nil {
			inst.Status = status
		}
	}

	return installs, nil
}

// ListInstallsEnriched returns all installations with IP and uptime from Proxmox.
func (e *Engine) ListInstallsEnriched() ([]*InstallListItem, error) {
	installs, err := e.store.ListInstalls()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	items := make([]*InstallListItem, 0, len(installs))
	for _, inst := range installs {
		item := &InstallListItem{Install: *inst}
		if inst.Status == "uninstalled" || inst.CTID == 0 {
			items = append(items, item)
			continue
		}
		if sd, err := e.cm.StatusDetail(ctx, inst.CTID); err == nil {
			item.Status = sd.Status
			item.Uptime = sd.Uptime
			item.Live = sd
		}
		if ip, err := e.cm.GetIP(inst.CTID); err == nil && ip != "" {
			item.IP = ip
		}
		if app, ok := e.catalog.Get(inst.AppID); ok {
			item.CatalogVersion = app.Version
			if inst.AppVersion != "" && isNewerVersion(app.Version, inst.AppVersion) {
				item.UpdateAvailable = true
			}
		}
		items = append(items, item)
	}

	return items, nil
}

// Reinstall creates a new container for a previously uninstalled app, reattaching preserved volumes.
func (e *Engine) Reinstall(installID string, req ReinstallRequest) (*Job, error) {
	inst, err := e.store.GetInstall(installID)
	if err != nil {
		return nil, fmt.Errorf("install %q not found", installID)
	}
	if inst.Status != "uninstalled" {
		return nil, fmt.Errorf("install %q is not uninstalled (status: %s)", installID, inst.Status)
	}
	if len(inst.MountPoints) == 0 {
		return nil, fmt.Errorf("install %q has no preserved volumes to reattach", installID)
	}

	// Look up the app
	app, ok := e.catalog.Get(inst.AppID)
	if !ok {
		return nil, fmt.Errorf("app %q not found in catalog", inst.AppID)
	}

	// Use existing values with optional overrides
	storage := inst.Storage
	if req.Storage != "" {
		storage = req.Storage
	}
	bridge := inst.Bridge
	if req.Bridge != "" {
		bridge = req.Bridge
	}
	cores := inst.Cores
	if req.Cores > 0 {
		cores = req.Cores
	}
	memoryMB := inst.MemoryMB
	if req.MemoryMB > 0 {
		memoryMB = req.MemoryMB
	}
	diskGB := inst.DiskGB
	if req.DiskGB > 0 {
		diskGB = req.DiskGB
	}

	now := time.Now()
	job := &Job{
		ID:          generateID(),
		Type:        JobTypeReinstall,
		State:       StateQueued,
		AppID:       inst.AppID,
		AppName:     inst.AppName,
		Node:        e.cfg.NodeName,
		Pool:        inst.Pool,
		Storage:     storage,
		Bridge:      bridge,
		Cores:       cores,
		MemoryMB:    memoryMB,
		DiskGB:      diskGB,
		Inputs:      req.Inputs,
		Outputs:     make(map[string]string),
		MountPoints: inst.MountPoints, // Carry preserved volumes with VolumeIDs
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if job.Inputs == nil {
		job.Inputs = make(map[string]string)
	}
	// Apply input defaults from manifest
	for _, input := range app.Inputs {
		if _, exists := job.Inputs[input.Key]; !exists && input.Default != nil {
			job.Inputs[input.Key] = fmt.Sprintf("%v", input.Default)
		}
	}

	if err := e.store.CreateJob(job); err != nil {
		return nil, fmt.Errorf("creating reinstall job: %w", err)
	}

	// Run install pipeline — on success, update the existing install record
	go e.runReinstall(job, inst)

	return job, nil
}

func (e *Engine) runReinstall(job *Job, inst *Install) {
	app, ok := e.catalog.Get(job.AppID)
	if !ok {
		job.State = StateFailed
		job.Error = fmt.Sprintf("app %q not found in catalog", job.AppID)
		now := time.Now()
		job.UpdatedAt = now
		job.CompletedAt = &now
		e.store.UpdateJob(job)
		return
	}

	ctx := &installContext{
		engine:   e,
		job:      job,
		manifest: app,
	}

	ctx.info("Starting reinstall of %s with %d preserved volume(s)", app.Name, len(job.MountPoints))

	for _, step := range installSteps {
		ctx.transition(step.state)

		if err := step.fn(ctx); err != nil {
			ctx.log("error", "Failed at %s: %v", step.state, err)
			ctx.job.State = StateFailed
			ctx.job.Error = fmt.Sprintf("%s: %v", step.state, err)
			now := time.Now()
			ctx.job.UpdatedAt = now
			ctx.job.CompletedAt = &now
			e.store.UpdateJob(ctx.job)
			return
		}
	}

	// Success — update the existing install record
	ctx.transition(StateCompleted)
	now := time.Now()
	ctx.job.CompletedAt = &now
	e.store.UpdateJob(ctx.job)

	inst.CTID = ctx.job.CTID
	inst.Status = "running"
	inst.Storage = ctx.job.Storage
	inst.Bridge = ctx.job.Bridge
	inst.Cores = ctx.job.Cores
	inst.MemoryMB = ctx.job.MemoryMB
	inst.DiskGB = ctx.job.DiskGB
	inst.Outputs = ctx.job.Outputs
	inst.MountPoints = ctx.job.MountPoints
	inst.AppVersion = app.Version
	e.store.UpdateInstall(inst)

	ctx.info("Reinstall complete! Container %d is running with reattached volumes.", ctx.job.CTID)
}

// Update creates a new container for an active install using the latest catalog version.
// Unlike Reinstall (which requires uninstalled status and preserved volumes), Update works
// on running/stopped installs and destroys the old container before re-provisioning.
func (e *Engine) Update(installID string, req ReinstallRequest) (*Job, error) {
	inst, err := e.store.GetInstall(installID)
	if err != nil {
		return nil, fmt.Errorf("install %q not found", installID)
	}
	if inst.Status != "uninstalled" {
		return nil, fmt.Errorf("install %q is uninstalled — use reinstall instead", installID)
	}

	// Look up the app and verify there's actually a newer version
	app, ok := e.catalog.Get(inst.AppID)
	if !ok {
		return nil, fmt.Errorf("app %q not found in catalog", inst.AppID)
	}
	if !isNewerVersion(app.Version, inst.AppVersion) {
		return nil, fmt.Errorf("no update available: catalog version %s is not newer than installed %s", app.Version, inst.AppVersion)
	}

	// Use existing values with optional overrides
	storage := inst.Storage
	if req.Storage != "" {
		storage = req.Storage
	}
	bridge := inst.Bridge
	if req.Bridge != "" {
		bridge = req.Bridge
	}
	cores := inst.Cores
	if req.Cores > 0 {
		cores = req.Cores
	}
	memoryMB := inst.MemoryMB
	if req.MemoryMB > 0 {
		memoryMB = req.MemoryMB
	}
	diskGB := inst.DiskGB
	if req.DiskGB > 0 {
		diskGB = req.DiskGB
	}

	now := time.Now()
	job := &Job{
		ID:          generateID(),
		Type:        JobTypeUpdate,
		State:       StateQueued,
		AppID:       inst.AppID,
		AppName:     inst.AppName,
		Node:        e.cfg.NodeName,
		Pool:        inst.Pool,
		Storage:     storage,
		Bridge:      bridge,
		Cores:       cores,
		MemoryMB:    memoryMB,
		DiskGB:      diskGB,
		Inputs:      req.Inputs,
		Outputs:     make(map[string]string),
		MountPoints: inst.MountPoints, // Carry mount points forward
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if job.Inputs == nil {
		job.Inputs = make(map[string]string)
	}
	for _, input := range app.Inputs {
		if _, exists := job.Inputs[input.Key]; !exists && input.Default != nil {
			job.Inputs[input.Key] = fmt.Sprintf("%v", input.Default)
		}
	}

	if err := e.store.CreateJob(job); err != nil {
		return nil, fmt.Errorf("creating update job: %w", err)
	}

	go e.runUpdate(job, inst)

	return job, nil
}

func (e *Engine) runUpdate(job *Job, inst *Install) {
	app, ok := e.catalog.Get(job.AppID)
	if !ok {
		job.State = StateFailed
		job.Error = fmt.Sprintf("app %q not found in catalog", job.AppID)
		now := time.Now()
		job.UpdatedAt = now
		job.CompletedAt = &now
		e.store.UpdateJob(job)
		return
	}

	ctx := &installContext{
		engine:   e,
		job:      job,
		manifest: app,
	}

	ctx.info("Starting update of %s from v%s to v%s", app.Name, inst.AppVersion, app.Version)

	// Destroy old container first
	if inst.CTID > 0 {
		bgCtx := context.Background()

		// Detach managed volumes before destroy if present
		hasDetachedVolumes := false
		if len(inst.MountPoints) > 0 {
			var managedIndexes []int
			for _, mp := range inst.MountPoints {
				if mp.Type == "volume" {
					managedIndexes = append(managedIndexes, mp.Index)
				}
			}
			if len(managedIndexes) > 0 {
				ctx.info("Detaching %d volume(s) before destroy...", len(managedIndexes))
				if config, err := e.cm.GetConfig(bgCtx, inst.CTID); err == nil {
					for i := range inst.MountPoints {
						if inst.MountPoints[i].Type != "volume" {
							continue
						}
						key := fmt.Sprintf("mp%d", inst.MountPoints[i].Index)
						if val, ok := config[key]; ok {
							if valStr, ok := val.(string); ok {
								parts := strings.SplitN(valStr, ",", 2)
								if len(parts) > 0 {
									inst.MountPoints[i].VolumeID = parts[0]
								}
							}
						}
					}
				}
				if err := e.cm.DetachMountPoints(bgCtx, inst.CTID, managedIndexes); err != nil {
					ctx.warn("Failed to detach mount points: %v — volumes may be destroyed", err)
				} else {
					hasDetachedVolumes = true
				}
				// Update job's mount points with fresh volume IDs
				job.MountPoints = inst.MountPoints
			}
		}

		ctx.info("Stopping and destroying old container CT %d...", inst.CTID)
		if err := e.cm.Shutdown(bgCtx, inst.CTID, 30); err != nil {
			_ = e.cm.Stop(bgCtx, inst.CTID)
		}
		var destroyErr error
		for attempt := 0; attempt < 5; attempt++ {
			if attempt > 0 {
				time.Sleep(5 * time.Second)
			}
			destroyErr = e.cm.Destroy(bgCtx, inst.CTID, hasDetachedVolumes)
			if destroyErr == nil {
				break
			}
			ctx.warn("Destroy attempt %d failed: %v", attempt+1, destroyErr)
		}
		if destroyErr != nil {
			ctx.log("error", "Failed to destroy old container: %v", destroyErr)
			job.State = StateFailed
			job.Error = fmt.Sprintf("destroy old container: %v", destroyErr)
			now := time.Now()
			job.UpdatedAt = now
			job.CompletedAt = &now
			e.store.UpdateJob(job)
			return
		}
		ctx.info("Old container destroyed successfully")
		// Brief pause after destroy for ZFS dataset cleanup to fully propagate
		time.Sleep(3 * time.Second)
	}

	// Run full install pipeline
	for _, step := range installSteps {
		ctx.transition(step.state)
		if err := step.fn(ctx); err != nil {
			ctx.log("error", "Failed at %s: %v", step.state, err)
			ctx.job.State = StateFailed
			ctx.job.Error = fmt.Sprintf("%s: %v", step.state, err)
			now := time.Now()
			ctx.job.UpdatedAt = now
			ctx.job.CompletedAt = &now
			e.store.UpdateJob(ctx.job)
			return
		}
	}

	// Success — update the existing install record
	ctx.transition(StateCompleted)
	now := time.Now()
	ctx.job.CompletedAt = &now
	e.store.UpdateJob(ctx.job)

	inst.CTID = ctx.job.CTID
	inst.Status = "running"
	inst.Storage = ctx.job.Storage
	inst.Bridge = ctx.job.Bridge
	inst.Cores = ctx.job.Cores
	inst.MemoryMB = ctx.job.MemoryMB
	inst.DiskGB = ctx.job.DiskGB
	inst.Outputs = ctx.job.Outputs
	inst.MountPoints = ctx.job.MountPoints
	inst.AppVersion = app.Version
	e.store.UpdateInstall(inst)

	ctx.info("Update complete! %s is now v%s (CT %d).", app.Name, app.Version, ctx.job.CTID)
}

// EditInstall recreates a container for an active install with modified settings.
// Unlike Update, it doesn't require a newer catalog version and preserves the MAC address
// so DHCP leases (and thus IP addresses) are maintained across the recreate.
func (e *Engine) EditInstall(installID string, req EditRequest) (*Job, error) {
	inst, err := e.store.GetInstall(installID)
	if err != nil {
		return nil, fmt.Errorf("install %q not found", installID)
	}
	if inst.Status == "uninstalled" {
		return nil, fmt.Errorf("install %q is uninstalled — cannot edit", installID)
	}

	// Validate edit request inputs
	if err := ValidateBridge(req.Bridge); err != nil {
		return nil, err
	}

	// Look up the app
	app, ok := e.catalog.Get(inst.AppID)
	if !ok {
		return nil, fmt.Errorf("app %q not found in catalog", inst.AppID)
	}

	// Apply overrides with existing values as defaults
	cores := inst.Cores
	if req.Cores > 0 {
		cores = req.Cores
	}
	memoryMB := inst.MemoryMB
	if req.MemoryMB > 0 {
		memoryMB = req.MemoryMB
	}
	diskGB := inst.DiskGB
	if req.DiskGB > 0 {
		if req.DiskGB < inst.DiskGB {
			return nil, fmt.Errorf("cannot shrink disk from %d GB to %d GB (Proxmox limitation)", inst.DiskGB, req.DiskGB)
		}
		diskGB = req.DiskGB
	}
	bridge := inst.Bridge
	if req.Bridge != "" {
		bridge = req.Bridge
	}

	// Merge inputs: existing values + overlay from request
	inputs := make(map[string]string)
	for k, v := range inst.Inputs {
		inputs[k] = v
	}
	for k, v := range req.Inputs {
		inputs[k] = v
	}
	// Apply input defaults from manifest for any keys not yet set
	for _, input := range app.Inputs {
		if _, exists := inputs[input.Key]; !exists && input.Default != nil {
			inputs[input.Key] = fmt.Sprintf("%v", input.Default)
		}
	}

	now := time.Now()
	job := &Job{
		ID:          generateID(),
		Type:        JobTypeEdit,
		State:       StateQueued,
		AppID:       inst.AppID,
		AppName:     inst.AppName,
		Node:        e.cfg.NodeName,
		Pool:        inst.Pool,
		Storage:     inst.Storage, // storage cannot change (volumes tied to it)
		Bridge:      bridge,
		Cores:       cores,
		MemoryMB:    memoryMB,
		DiskGB:      diskGB,
		Inputs:      inputs,
		Outputs:     make(map[string]string),
		MountPoints: inst.MountPoints, // carry mount points forward
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := e.store.CreateJob(job); err != nil {
		return nil, fmt.Errorf("creating edit job: %w", err)
	}

	go e.runEdit(job, inst)

	return job, nil
}

func (e *Engine) runEdit(job *Job, inst *Install) {
	app, ok := e.catalog.Get(job.AppID)
	if !ok {
		job.State = StateFailed
		job.Error = fmt.Sprintf("app %q not found in catalog", job.AppID)
		now := time.Now()
		job.UpdatedAt = now
		job.CompletedAt = &now
		e.store.UpdateJob(job)
		return
	}

	ctx := &installContext{
		engine:   e,
		job:      job,
		manifest: app,
	}

	ctx.info("Starting edit of %s (CTID %d)", app.Name, inst.CTID)

	bgCtx := context.Background()

	// Read MAC address from current container before destroying it
	if inst.CTID > 0 {
		if config, err := e.cm.GetConfig(bgCtx, inst.CTID); err == nil {
			if net0, ok := config["net0"]; ok {
				if net0Str, ok := net0.(string); ok {
					ctx.hwAddr = extractHWAddr(net0Str)
					if ctx.hwAddr != "" {
						ctx.info("Preserved MAC address: %s", ctx.hwAddr)
					}
				}
			}
		} else {
			ctx.warn("Could not read container config for MAC address: %v", err)
		}

		// Detach managed volumes before destroy if present
		hasDetachedVolumes := false
		if len(inst.MountPoints) > 0 {
			var managedIndexes []int
			for _, mp := range inst.MountPoints {
				if mp.Type == "volume" {
					managedIndexes = append(managedIndexes, mp.Index)
				}
			}
			if len(managedIndexes) > 0 {
				ctx.info("Detaching %d volume(s) before destroy...", len(managedIndexes))
				if config, err := e.cm.GetConfig(bgCtx, inst.CTID); err == nil {
					for i := range inst.MountPoints {
						if inst.MountPoints[i].Type != "volume" {
							continue
						}
						key := fmt.Sprintf("mp%d", inst.MountPoints[i].Index)
						if val, ok := config[key]; ok {
							if valStr, ok := val.(string); ok {
								parts := strings.SplitN(valStr, ",", 2)
								if len(parts) > 0 {
									inst.MountPoints[i].VolumeID = parts[0]
								}
							}
						}
					}
				}
				if err := e.cm.DetachMountPoints(bgCtx, inst.CTID, managedIndexes); err != nil {
					ctx.warn("Failed to detach mount points: %v — volumes may be destroyed", err)
				} else {
					hasDetachedVolumes = true
				}
				// Update job's mount points with fresh volume IDs
				job.MountPoints = inst.MountPoints
			}
		}

		// Stop and destroy old container
		ctx.info("Stopping and destroying old container CT %d...", inst.CTID)
		if err := e.cm.Shutdown(bgCtx, inst.CTID, 30); err != nil {
			_ = e.cm.Stop(bgCtx, inst.CTID)
		}
		var destroyErr error
		for attempt := 0; attempt < 5; attempt++ {
			if attempt > 0 {
				time.Sleep(5 * time.Second)
			}
			destroyErr = e.cm.Destroy(bgCtx, inst.CTID, hasDetachedVolumes)
			if destroyErr == nil {
				break
			}
			ctx.warn("Destroy attempt %d failed: %v", attempt+1, destroyErr)
		}
		if destroyErr != nil {
			ctx.log("error", "Failed to destroy old container: %v", destroyErr)
			job.State = StateFailed
			job.Error = fmt.Sprintf("destroy old container: %v", destroyErr)
			now := time.Now()
			job.UpdatedAt = now
			job.CompletedAt = &now
			e.store.UpdateJob(job)
			return
		}
		ctx.info("Old container destroyed successfully")
		time.Sleep(3 * time.Second)
	}

	// Run full install pipeline (uses ctx.hwAddr for MAC preservation)
	for _, step := range installSteps {
		ctx.transition(step.state)
		if err := step.fn(ctx); err != nil {
			ctx.log("error", "Failed at %s: %v", step.state, err)
			ctx.job.State = StateFailed
			ctx.job.Error = fmt.Sprintf("%s: %v", step.state, err)
			now := time.Now()
			ctx.job.UpdatedAt = now
			ctx.job.CompletedAt = &now
			e.store.UpdateJob(ctx.job)
			return
		}
	}

	// Success — update the existing install record (keep AppVersion unchanged)
	ctx.transition(StateCompleted)
	now := time.Now()
	ctx.job.CompletedAt = &now
	e.store.UpdateJob(ctx.job)

	inst.CTID = ctx.job.CTID
	inst.Status = "running"
	inst.Bridge = ctx.job.Bridge
	inst.Cores = ctx.job.Cores
	inst.MemoryMB = ctx.job.MemoryMB
	inst.DiskGB = ctx.job.DiskGB
	inst.Inputs = ctx.job.Inputs
	inst.Outputs = ctx.job.Outputs
	inst.MountPoints = ctx.job.MountPoints
	e.store.UpdateInstall(inst)

	ctx.info("Edit complete! %s CT %d recreated with preserved MAC address.", app.Name, ctx.job.CTID)
}

// validMAC matches a standard colon-separated MAC address (e.g. BC:24:11:AB:CD:EF).
var validMAC = regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}[0-9A-Fa-f]{2}$`)

// extractHWAddr parses a Proxmox net0 config string and returns the hwaddr value.
// Example input: "name=eth0,bridge=vmbr0,hwaddr=BC:24:11:AB:CD:EF,ip=dhcp"
// Returns empty string if the value is missing or doesn't match MAC format.
func extractHWAddr(net0 string) string {
	for _, part := range strings.Split(net0, ",") {
		if strings.HasPrefix(part, "hwaddr=") {
			mac := strings.TrimPrefix(part, "hwaddr=")
			if validMAC.MatchString(mac) {
				return mac
			}
			return ""
		}
	}
	return ""
}

// ReconfigureInstall applies in-place changes to an active install without
// destroying/recreating the container. Resource changes (cores, memory) are
// applied via the Proxmox API. Input changes trigger the app's configure()
// lifecycle method inside the container. This is a synchronous operation.
func (e *Engine) ReconfigureInstall(installID string, req ReconfigureRequest) (*Install, error) {
	inst, err := e.store.GetInstall(installID)
	if err != nil {
		return nil, fmt.Errorf("install %q not found", installID)
	}
	if inst.Status == "uninstalled" || inst.CTID == 0 {
		return nil, fmt.Errorf("install %q has no active container", installID)
	}

	ctx := context.Background()

	// Apply resource changes via Proxmox API (works on running or stopped containers)
	resourceChanged := false
	if req.Cores > 0 && req.Cores != inst.Cores {
		resourceChanged = true
		inst.Cores = req.Cores
	}
	if req.MemoryMB > 0 && req.MemoryMB != inst.MemoryMB {
		resourceChanged = true
		inst.MemoryMB = req.MemoryMB
	}

	if resourceChanged {
		params := url.Values{}
		if req.Cores > 0 {
			params.Set("cores", strconv.Itoa(inst.Cores))
		}
		if req.MemoryMB > 0 {
			params.Set("memory", strconv.Itoa(inst.MemoryMB))
		}
		if err := e.cm.UpdateConfig(ctx, inst.CTID, params); err != nil {
			return nil, fmt.Errorf("updating container config: %w", err)
		}
	}

	// Apply input changes and run configure() if needed
	inputsChanged := false
	if len(req.Inputs) > 0 {
		if inst.Inputs == nil {
			inst.Inputs = make(map[string]string)
		}
		for k, v := range req.Inputs {
			if inst.Inputs[k] != v {
				inst.Inputs[k] = v
				inputsChanged = true
			}
		}
	}

	if inputsChanged {
		// Push updated inputs.json into the container
		if err := pushInputsJSON(inst.CTID, e.cm, inst.Inputs); err != nil {
			return nil, fmt.Errorf("pushing updated inputs: %w", err)
		}

		// Look up the app to get the provision script path
		app, ok := e.catalog.Get(inst.AppID)
		if ok && app.Provisioning.Script != "" {
			envVars := mergeEnvVars(app.Provisioning.Env, inst.EnvVars)
			cmd := buildProvisionCommand(app.Provisioning.Script, "configure", envVars)
			result, err := e.cm.ExecStream(inst.CTID, cmd, func(line string) {
				// Log configure output but don't track as job logs (synchronous op)
			})
			if err != nil {
				return nil, fmt.Errorf("running configure: %w", err)
			}
			if result.ExitCode != 0 {
				return nil, fmt.Errorf("configure exited with code %d", result.ExitCode)
			}
		}
	}

	// Update the install record
	e.store.UpdateInstall(inst)

	return inst, nil
}

// --- Stack operations ---

// GetStack returns a stack by ID.
func (e *Engine) GetStack(id string) (*Stack, error) {
	return e.store.GetStack(id)
}

// GetStackDetail returns a stack enriched with live data.
func (e *Engine) GetStackDetail(id string) (*StackDetail, error) {
	stack, err := e.store.GetStack(id)
	if err != nil {
		return nil, err
	}

	detail := &StackDetail{Stack: *stack}

	if stack.Status != "uninstalled" && stack.CTID > 0 {
		ctx := context.Background()
		if sd, err := e.cm.StatusDetail(ctx, stack.CTID); err == nil {
			detail.Live = sd
			detail.Status = sd.Status
		}
		if ip, err := e.cm.GetIP(stack.CTID); err == nil && ip != "" {
			detail.IP = ip
		}
	}

	return detail, nil
}

// ListStacks returns all stacks.
func (e *Engine) ListStacks() ([]*Stack, error) {
	return e.store.ListStacks()
}

// ListStacksEnriched returns all stacks with live data from Proxmox.
func (e *Engine) ListStacksEnriched() ([]*StackListItem, error) {
	stacks, err := e.store.ListStacks()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	items := make([]*StackListItem, 0, len(stacks))
	for _, stack := range stacks {
		item := &StackListItem{Stack: *stack}
		if stack.Status == "uninstalled" || stack.CTID == 0 {
			items = append(items, item)
			continue
		}
		if sd, err := e.cm.StatusDetail(ctx, stack.CTID); err == nil {
			item.Status = sd.Status
			item.Uptime = sd.Uptime
			item.Live = sd
		}
		if ip, err := e.cm.GetIP(stack.CTID); err == nil && ip != "" {
			item.IP = ip
		}
		items = append(items, item)
	}

	return items, nil
}

// StartStackContainer starts a stopped stack container.
func (e *Engine) StartStackContainer(stackID string) error {
	stack, err := e.store.GetStack(stackID)
	if err != nil {
		return fmt.Errorf("stack %q not found", stackID)
	}
	if stack.Status == "uninstalled" || stack.CTID == 0 {
		return fmt.Errorf("stack %q has no active container", stackID)
	}
	return e.cm.Start(context.Background(), stack.CTID)
}

// StopStackContainer gracefully stops a stack container.
func (e *Engine) StopStackContainer(stackID string) error {
	stack, err := e.store.GetStack(stackID)
	if err != nil {
		return fmt.Errorf("stack %q not found", stackID)
	}
	if stack.Status == "uninstalled" || stack.CTID == 0 {
		return fmt.Errorf("stack %q has no active container", stackID)
	}
	return e.cm.Shutdown(context.Background(), stack.CTID, 30)
}

// RestartStackContainer stops then starts a stack container.
func (e *Engine) RestartStackContainer(stackID string) error {
	stack, err := e.store.GetStack(stackID)
	if err != nil {
		return fmt.Errorf("stack %q not found", stackID)
	}
	if stack.Status == "uninstalled" || stack.CTID == 0 {
		return fmt.Errorf("stack %q has no active container", stackID)
	}
	ctx := context.Background()
	if err := e.cm.Shutdown(ctx, stack.CTID, 30); err != nil {
		_ = e.cm.Stop(ctx, stack.CTID)
	}
	time.Sleep(2 * time.Second)
	return e.cm.Start(ctx, stack.CTID)
}

// UninstallStack destroys a stack's container.
func (e *Engine) UninstallStack(stackID string) (*Job, error) {
	stack, err := e.store.GetStack(stackID)
	if err != nil {
		return nil, fmt.Errorf("stack %q not found", stackID)
	}

	now := time.Now()
	job := &Job{
		ID:        generateID(),
		Type:      JobTypeUninstall,
		State:     StateQueued,
		AppID:     "stack:" + stack.Name,
		AppName:   stack.Name,
		CTID:      stack.CTID,
		Node:      stack.Node,
		Pool:      stack.Pool,
		StackID:   stack.ID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := e.store.CreateJob(job); err != nil {
		return nil, fmt.Errorf("creating uninstall job: %w", err)
	}

	go e.runStackUninstall(job, stack)

	return job, nil
}

func (e *Engine) runStackUninstall(job *Job, stack *Stack) {
	ctx := &installContext{engine: e, job: job}

	ctx.info("Starting uninstall of stack %s (CTID %d)", stack.Name, stack.CTID)

	bgCtx := context.Background()
	ctGone := false

	// Stop container
	ctx.transition("stopping")
	if err := e.cm.Shutdown(bgCtx, stack.CTID, 30); err != nil {
		if isContainerGone(err) {
			ctx.info("Container %d already removed — skipping stop/destroy", stack.CTID)
			ctGone = true
		} else {
			ctx.warn("Graceful shutdown failed: %v — forcing stop", err)
			if err := e.cm.Stop(bgCtx, stack.CTID); err != nil {
				if isContainerGone(err) {
					ctx.info("Container %d already removed — skipping destroy", stack.CTID)
					ctGone = true
				}
			}
		}
	}

	// Destroy container with retries
	if !ctGone {
		ctx.transition("destroying")
		var destroyErr error
		for attempt := 0; attempt < 5; attempt++ {
			if attempt > 0 {
				time.Sleep(5 * time.Second)
			}
			destroyErr = e.cm.Destroy(bgCtx, stack.CTID)
			if destroyErr == nil {
				break
			}
			if isContainerGone(destroyErr) {
				ctx.info("Container %d already removed", stack.CTID)
				destroyErr = nil
				break
			}
			ctx.warn("Destroy attempt %d failed: %v", attempt+1, destroyErr)
		}
		if destroyErr != nil {
			ctx.log("error", "Failed to destroy container: %v", destroyErr)
			job.State = StateFailed
			job.Error = fmt.Sprintf("destroy: %v", destroyErr)
			now := time.Now()
			job.UpdatedAt = now
			job.CompletedAt = &now
			e.store.UpdateJob(job)
			return
		}
	}

	// Remove stack record
	e.store.DeleteStack(stack.ID)

	ctx.transition(StateCompleted)
	now := time.Now()
	job.CompletedAt = &now
	e.store.UpdateJob(job)
	ctx.info("Stack %s uninstalled. Container %d destroyed.", stack.Name, stack.CTID)
}

// EditStack recreates a stack container with modified resource settings.
// Preserves MAC address for DHCP lease continuity.
func (e *Engine) EditStack(stackID string, req EditRequest) (*Job, error) {
	stack, err := e.store.GetStack(stackID)
	if err != nil {
		return nil, fmt.Errorf("stack %q not found", stackID)
	}
	if stack.Status == "uninstalled" {
		return nil, fmt.Errorf("stack %q is uninstalled — cannot edit", stackID)
	}

	// Validate edit request inputs
	if err := ValidateBridge(req.Bridge); err != nil {
		return nil, err
	}

	// Apply overrides
	cores := stack.Cores
	if req.Cores > 0 {
		cores = req.Cores
	}
	memoryMB := stack.MemoryMB
	if req.MemoryMB > 0 {
		memoryMB = req.MemoryMB
	}
	diskGB := stack.DiskGB
	if req.DiskGB > 0 {
		if req.DiskGB < stack.DiskGB {
			return nil, fmt.Errorf("cannot shrink disk from %d GB to %d GB (Proxmox limitation)", stack.DiskGB, req.DiskGB)
		}
		diskGB = req.DiskGB
	}
	bridge := stack.Bridge
	if req.Bridge != "" {
		bridge = req.Bridge
	}

	now := time.Now()
	job := &Job{
		ID:          generateID(),
		Type:        JobTypeEdit,
		State:       StateQueued,
		AppID:       "stack:" + stack.Name,
		AppName:     stack.Name,
		Node:        e.cfg.NodeName,
		Pool:        stack.Pool,
		Storage:     stack.Storage, // storage cannot change (volumes tied to it)
		Bridge:      bridge,
		Cores:       cores,
		MemoryMB:    memoryMB,
		DiskGB:      diskGB,
		Hostname:    stack.Hostname,
		IPAddress:   stack.IPAddress,
		OnBoot:      stack.OnBoot,
		Unprivileged: stack.Unprivileged,
		Inputs:      make(map[string]string),
		Outputs:     make(map[string]string),
		MountPoints: stack.MountPoints, // carry mount points forward
		Devices:     stack.Devices,
		EnvVars:     stack.EnvVars,
		StackID:     stack.ID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := e.store.CreateJob(job); err != nil {
		return nil, fmt.Errorf("creating stack edit job: %w", err)
	}

	go e.runStackEdit(job, stack)

	return job, nil
}

func (e *Engine) runStackEdit(job *Job, stack *Stack) {
	// Collect manifests for all apps in the stack
	manifests := make([]*catalog.AppManifest, 0, len(stack.Apps))
	for _, app := range stack.Apps {
		m, ok := e.catalog.Get(app.AppID)
		if !ok {
			job.State = StateFailed
			job.Error = fmt.Sprintf("app %q not found in catalog", app.AppID)
			now := time.Now()
			job.UpdatedAt = now
			job.CompletedAt = &now
			e.store.UpdateJob(job)
			return
		}
		manifests = append(manifests, m)
	}

	ctx := &installContext{engine: e, job: job}
	ctx.info("Starting edit of stack %s (CTID %d)", stack.Name, stack.CTID)

	bgCtx := context.Background()

	// Read MAC address before destroying
	if stack.CTID > 0 {
		if config, err := e.cm.GetConfig(bgCtx, stack.CTID); err == nil {
			if net0, ok := config["net0"]; ok {
				if net0Str, ok := net0.(string); ok {
					ctx.hwAddr = extractHWAddr(net0Str)
					if ctx.hwAddr != "" {
						ctx.info("Preserved MAC address: %s", ctx.hwAddr)
					}
				}
			}
		}

		// Detach managed volumes before destroy
		hasDetachedVolumes := false
		if len(stack.MountPoints) > 0 {
			var managedIndexes []int
			for _, mp := range stack.MountPoints {
				if mp.Type == "volume" {
					managedIndexes = append(managedIndexes, mp.Index)
				}
			}
			if len(managedIndexes) > 0 {
				ctx.info("Detaching %d volume(s) before destroy...", len(managedIndexes))
				if config, err := e.cm.GetConfig(bgCtx, stack.CTID); err == nil {
					for i := range stack.MountPoints {
						if stack.MountPoints[i].Type != "volume" {
							continue
						}
						key := fmt.Sprintf("mp%d", stack.MountPoints[i].Index)
						if val, ok := config[key]; ok {
							if valStr, ok := val.(string); ok {
								parts := strings.SplitN(valStr, ",", 2)
								if len(parts) > 0 {
									stack.MountPoints[i].VolumeID = parts[0]
								}
							}
						}
					}
				}
				if err := e.cm.DetachMountPoints(bgCtx, stack.CTID, managedIndexes); err != nil {
					ctx.warn("Failed to detach mount points: %v", err)
				} else {
					hasDetachedVolumes = true
				}
				job.MountPoints = stack.MountPoints
			}
		}

		// Stop and destroy old container
		ctx.info("Stopping and destroying old container CT %d...", stack.CTID)
		if err := e.cm.Shutdown(bgCtx, stack.CTID, 30); err != nil {
			_ = e.cm.Stop(bgCtx, stack.CTID)
		}
		var destroyErr error
		for attempt := 0; attempt < 5; attempt++ {
			if attempt > 0 {
				time.Sleep(5 * time.Second)
			}
			destroyErr = e.cm.Destroy(bgCtx, stack.CTID, hasDetachedVolumes)
			if destroyErr == nil {
				break
			}
			ctx.warn("Destroy attempt %d failed: %v", attempt+1, destroyErr)
		}
		if destroyErr != nil {
			ctx.log("error", "Failed to destroy old container: %v", destroyErr)
			job.State = StateFailed
			job.Error = fmt.Sprintf("destroy old container: %v", destroyErr)
			now := time.Now()
			job.UpdatedAt = now
			job.CompletedAt = &now
			e.store.UpdateJob(job)
			return
		}
		ctx.info("Old container destroyed successfully")
		time.Sleep(3 * time.Second)
	}

	// Re-run the full stack install pipeline (reuses hwAddr via the installContext)
	// We need to inject hwAddr into the stack create step — done via installContext
	e.runStackInstallWithHWAddr(bgCtx, job, stack, manifests, ctx.hwAddr)
}

// runStackInstallWithHWAddr re-runs the stack install pipeline preserving the MAC address.
// On success, updates the existing stack record instead of creating a new one.
func (e *Engine) runStackInstallWithHWAddr(bgCtx context.Context, job *Job, stack *Stack, manifests []*catalog.AppManifest, hwAddr string) {
	ctx := &installContext{
		ctx:    bgCtx,
		engine: e,
		job:    job,
		hwAddr: hwAddr,
	}

	ctx.info("Recreating stack %s with %d apps", stack.Name, len(stack.Apps))

	// Build fresh StackApp entries from existing stack (preserve inputs)
	apps := make([]StackApp, len(stack.Apps))
	copy(apps, stack.Apps)
	for i := range apps {
		apps[i].Status = "pending"
		apps[i].Error = ""
		apps[i].Outputs = make(map[string]string)
	}

	// Step 1: Validate manifests
	ctx.transition(StateValidateManifest)
	for _, m := range manifests {
		if err := m.Validate(); err != nil {
			ctx.failJob("validate_manifest: %v", err)
			return
		}
	}

	// Step 2: Allocate CTID (under lock to prevent races with concurrent installs)
	ctx.transition(StateAllocateCTID)
	e.ctidMu.Lock()
	ctid, err := e.cm.AllocateCTID(bgCtx)
	if err != nil {
		e.ctidMu.Unlock()
		ctx.failJob("allocate_ctid: %v", err)
		return
	}
	ctx.job.CTID = ctid
	ctx.job.UpdatedAt = time.Now()
	e.store.UpdateJob(ctx.job)
	ctx.info("Allocated CTID: %d", ctid)

	// Step 3: Create container with preserved MAC (ctidMu released after Create)
	ctx.transition(StateCreateContainer)
	template := stack.OSTemplate
	if !strings.Contains(template, ":") {
		template = e.cm.ResolveTemplate(bgCtx, template, job.Storage)
	}

	featureSet := make(map[string]bool)
	for _, m := range manifests {
		for _, f := range m.LXC.Defaults.Features {
			featureSet[f] = true
		}
	}
	var features []string
	for f := range featureSet {
		features = append(features, f)
	}

	opts := CreateOptions{
		CTID:         ctid,
		OSTemplate:   template,
		Storage:      job.Storage,
		RootFSSize:   job.DiskGB,
		Cores:        job.Cores,
		MemoryMB:     job.MemoryMB,
		Bridge:       job.Bridge,
		HWAddr:       hwAddr,
		Hostname:     job.Hostname,
		IPAddress:    job.IPAddress,
		Unprivileged: job.Unprivileged,
		Pool:         job.Pool,
		Features:     features,
		OnBoot:       job.OnBoot,
		Tags:         buildTags("appstore;stack;managed", job.ExtraTags),
	}

	for _, mp := range job.MountPoints {
		mpStorage := mp.Storage
		if mpStorage == "" {
			mpStorage = job.Storage
		}
		opts.MountPoints = append(opts.MountPoints, MountPointOption{
			Index: mp.Index, Type: mp.Type, MountPath: mp.MountPath,
			Storage: mpStorage, SizeGB: mp.SizeGB, VolumeID: mp.VolumeID,
			HostPath: mp.HostPath, ReadOnly: mp.ReadOnly,
		})
	}

	ctx.info("Creating container %d (template=%s, %d cores, %d MB, %d GB)", ctid, template, opts.Cores, opts.MemoryMB, opts.RootFSSize)
	if hwAddr != "" {
		ctx.info("Using preserved MAC address: %s", hwAddr)
	}
	if err := e.cm.Create(bgCtx, opts); err != nil {
		e.ctidMu.Unlock()
		ctx.failJob("create_container: %v", err)
		return
	}
	e.ctidMu.Unlock()

	// Aggregate GPU devices from all app manifests in the stack
	var allStackDevices []DevicePassthrough
	for _, m := range manifests {
		if len(m.GPU.Profiles) > 0 && e.cfg.GPU.Policy != config.GPUPolicyNone {
			// Take the first profile for each app that matches
			for _, profile := range m.GPU.Profiles {
				if devs, ok := gpuProfiles[profile]; ok {
					available, missing := validateGPUDevices(devs)
					if len(missing) > 0 && m.GPU.Required {
						ctx.failJob("GPU required for app %q but device(s) not found on host: %s", m.ID, strings.Join(missing, ", "))
						return
					}
					allStackDevices = append(allStackDevices, available...)
					break
				}
			}
		}
	}
	job.Devices = allStackDevices // Update job's devices

	// Configure devices
	if len(job.Devices) > 0 {
		ctx.info("Configuring %d device passthrough(s)...", len(job.Devices))
		if err := e.cm.ConfigureDevices(ctid, job.Devices); err != nil {
			ctx.failJob("configure_devices: %v", err)
			return
		}
		if hasNvidiaDevices(job.Devices) {
			libPath, _ := resolveNvidiaLibPath()
			if libPath != "" {
				nextMP := len(job.MountPoints)
				e.cm.MountHostPath(ctid, nextMP, libPath, nvidiaContainerLibPath, true)
			}
		}
	}

	// Apply extra LXC config from all manifests (with validation)
	for _, m := range manifests {
		if len(m.LXC.ExtraConfig) > 0 {
			if err := ValidateExtraConfig(m.LXC.ExtraConfig); err != nil {
				ctx.failJob("extra LXC config validation for %s: %v", m.ID, err)
				return
			}
			e.cm.AppendLXCConfig(ctid, m.LXC.ExtraConfig)
		}
	}

	// Start container
	ctx.transition(StateStartContainer)
	if err := e.cm.Start(bgCtx, ctid); err != nil {
		ctx.failJob("start_container: %v", err)
		return
	}

	// Wait for network
	ctx.transition(StateWaitForNetwork)
	for i := 0; i < 30; i++ {
		ip, err := e.cm.GetIP(ctid)
		if err == nil && ip != "" && ip != "127.0.0.1" {
			ctx.info("Container %d has IP: %s", ctid, ip)
			break
		}
		time.Sleep(2 * time.Second)
	}

	// GPU runtime setup
	if hasNvidiaDevices(job.Devices) {
		ldconfContent := nvidiaContainerLibPath + "\n"
		for _, cmd := range [][]string{
			{"mkdir", "-p", "/etc/ld.so.conf.d"},
			{"sh", "-c", fmt.Sprintf("echo '%s' > %s", ldconfContent, nvidiaLdconfPath)},
			{"ldconfig"},
		} {
			e.cm.Exec(ctid, cmd)
		}
	}

	// Install base packages
	ctx.transition(StateInstallBasePkgs)
	if err := stepInstallBasePackages(ctx); err != nil {
		ctx.failJob("install_base_packages: %v", err)
		return
	}

	// Push SDK
	ctx.transition(StatePushAssets)
	if err := ensurePython(ctid, e.cm); err != nil {
		ctx.failJob("push_assets: %v", err)
		return
	}
	if err := pushSDK(ctid, e.cm); err != nil {
		ctx.failJob("push_assets: %v", err)
		return
	}
	mergedPerms := mergeAllPermissions(manifests)
	if err := pushPermissionsJSON(ctid, e.cm, mergedPerms); err != nil {
		ctx.failJob("push_assets: %v", err)
		return
	}

	// Provision each app
	ctx.transition(StateProvision)
	allOutputs := make(map[string]string)

	for i, app := range apps {
		manifest := manifests[i]
		apps[i].Status = "provisioning"
		ctx.info("[%d/%d] Provisioning: %s", i+1, len(apps), app.AppName)

		appProvisionDir := filepath.Join(manifest.DirPath, "provision")
		if _, err := os.Stat(appProvisionDir); os.IsNotExist(err) {
			apps[i].Status = "failed"
			apps[i].Error = "provision directory not found"
			continue
		}

		targetProvisionDir := "/opt/appstore/provision/" + app.AppID
		e.cm.Exec(ctid, []string{"mkdir", "-p", targetProvisionDir})
		entries, _ := os.ReadDir(appProvisionDir)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			src := filepath.Join(appProvisionDir, entry.Name())
			e.cm.Push(ctid, src, targetProvisionDir+"/"+entry.Name(), "0755")
		}

		templatesDir := filepath.Join(manifest.DirPath, "templates")
		if _, err := os.Stat(templatesDir); err == nil {
			e.cm.Exec(ctid, []string{"mkdir", "-p", "/opt/appstore/templates"})
			tEntries, _ := os.ReadDir(templatesDir)
			for _, te := range tEntries {
				if !te.IsDir() {
					e.cm.Push(ctid, filepath.Join(templatesDir, te.Name()), "/opt/appstore/templates/"+te.Name(), "0644")
				}
			}
		}

		e.cm.Exec(ctid, []string{"mkdir", "-p", "/opt/appstore/" + app.AppID})
		pushInputsJSONToPath(ctid, e.cm, app.Inputs, "/opt/appstore/"+app.AppID+"/inputs.json")

		envVars := mergeEnvVars(manifest.Provisioning.Env, job.EnvVars)
		cmd := buildStackProvisionCommand(app.AppID, manifest.Provisioning.Script, "install", envVars)

		appOutputs := make(map[string]string)
		result, err := e.cm.ExecStream(ctid, cmd, func(line string) {
			line = strings.TrimSpace(line)
			if line == "" {
				return
			}
			if strings.HasPrefix(line, "@@APPLOG@@") {
				jsonStr := strings.TrimPrefix(line, "@@APPLOG@@")
				level := extractJSONField(jsonStr, "level")
				msg := extractJSONField(jsonStr, "msg")
				if level == "output" {
					key := extractJSONField(jsonStr, "key")
					value := extractJSONField(jsonStr, "value")
					if key != "" {
						appOutputs[key] = value
					}
				} else if msg != "" {
					ctx.log(level, "[%s] %s", app.AppID, msg)
				}
			} else {
				ctx.info("[%s] %s", app.AppID, line)
			}
		})

		if err != nil || (result != nil && result.ExitCode != 0) {
			apps[i].Status = "failed"
			if err != nil {
				apps[i].Error = err.Error()
			} else {
				apps[i].Error = fmt.Sprintf("exit code %d", result.ExitCode)
			}
			continue
		}

		apps[i].Status = "completed"
		apps[i].Outputs = appOutputs
		for k, v := range appOutputs {
			allOutputs[app.AppID+"."+k] = v
		}
	}

	// Collect manifest outputs
	ip, _ := e.cm.GetIP(ctid)
	for i, app := range apps {
		manifest := manifests[i]
		for _, out := range manifest.Outputs {
			value := out.Value
			value = strings.ReplaceAll(value, "{{ip}}", ip)
			for k, v := range app.Inputs {
				value = strings.ReplaceAll(value, "{{"+k+"}}", v)
			}
			allOutputs[app.AppID+"."+out.Key] = value
			if apps[i].Outputs == nil {
				apps[i].Outputs = make(map[string]string)
			}
			apps[i].Outputs[out.Key] = value
		}
	}

	job.Outputs = allOutputs
	ctx.transition(StateCompleted)
	now := time.Now()
	ctx.job.CompletedAt = &now
	e.store.UpdateJob(ctx.job)

	// Update existing stack record
	stack.CTID = ctid
	stack.Status = "running"
	stack.Bridge = job.Bridge
	stack.Cores = job.Cores
	stack.MemoryMB = job.MemoryMB
	stack.DiskGB = job.DiskGB
	stack.Apps = apps
	stack.MountPoints = job.MountPoints
	e.store.UpdateStack(stack)

	ctx.info("Stack edit complete! %s CT %d recreated with preserved MAC address.", stack.Name, ctid)
}

// GetStorageInfo returns resolved storage information for a Proxmox storage ID.
func (e *Engine) GetStorageInfo(ctx context.Context, storageID string) (*StorageInfo, error) {
	return e.cm.GetStorageInfo(ctx, storageID)
}

// isNewerVersion returns true if catalog version is strictly greater than installed version.
// Parses "major.minor.patch" semver (strips leading "v"). Falls back to string inequality on parse failure.
func isNewerVersion(catalog, installed string) bool {
	parse := func(s string) (int, int, int, bool) {
		s = strings.TrimPrefix(s, "v")
		parts := strings.SplitN(s, ".", 3)
		if len(parts) != 3 {
			return 0, 0, 0, false
		}
		major, e1 := strconv.Atoi(parts[0])
		minor, e2 := strconv.Atoi(parts[1])
		// Strip pre-release suffix (e.g. "1-beta")
		patchStr := strings.SplitN(parts[2], "-", 2)[0]
		patch, e3 := strconv.Atoi(patchStr)
		if e1 != nil || e2 != nil || e3 != nil {
			return 0, 0, 0, false
		}
		return major, minor, patch, true
	}

	cMaj, cMin, cPatch, cOK := parse(catalog)
	iMaj, iMin, iPatch, iOK := parse(installed)
	if !cOK || !iOK {
		return catalog != installed
	}

	if cMaj != iMaj {
		return cMaj > iMaj
	}
	if cMin != iMin {
		return cMin > iMin
	}
	return cPatch > iPatch
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// isContainerGone returns true if the error indicates the container or its
// config no longer exists on the Proxmox node (already destroyed externally).
func isContainerGone(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "no such container") ||
		strings.Contains(msg, "not found")
}