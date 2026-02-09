// Package engine orchestrates install/uninstall jobs for the PVE App Store.
package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
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
	Destroy(ctx context.Context, ctid int) error
	Status(ctx context.Context, ctid int) (string, error)
	StatusDetail(ctx context.Context, ctid int) (*ContainerStatusDetail, error)
	ResolveTemplate(ctx context.Context, name, storage string) string
	Exec(ctid int, command []string) (*pct.ExecResult, error)
	ExecStream(ctid int, command []string, onLine func(line string)) (*pct.ExecResult, error)
	ExecScript(ctid int, scriptPath string, env map[string]string) (*pct.ExecResult, error)
	Push(ctid int, src, dst, perms string) error
	GetIP(ctid int) (string, error)
	GetConfig(ctx context.Context, ctid int) (map[string]interface{}, error)
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

	// Recover orphaned jobs from previous run
	if n, err := store.RecoverOrphanedJobs(); err != nil {
		fmt.Printf("  engine:  warning: orphan recovery failed: %v\n", err)
	} else if n > 0 {
		fmt.Printf("  engine:  recovered %d orphaned job(s) from previous run\n", n)
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
		Devices:      req.Devices,
		EnvVars:      req.EnvVars,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Auto-add GPU devices from manifest profiles if none specified.
	// Validate that device nodes actually exist on the host before adding.
	if len(job.Devices) == 0 && len(app.GPU.Profiles) > 0 && e.cfg.GPU.Policy != config.GPUPolicyNone {
		for _, profile := range app.GPU.Profiles {
			if devs, ok := gpuProfiles[profile]; ok {
				available, missing := validateGPUDevices(devs)
				if len(missing) > 0 && app.GPU.Required {
					return nil, fmt.Errorf("GPU required but device(s) not found on host: %s", strings.Join(missing, ", "))
				}
				job.Devices = available
				break // use first matching profile
			}
		}
	}
	// Validate explicitly-requested devices too
	if len(job.Devices) > 0 {
		available, missing := validateGPUDevices(job.Devices)
		if len(missing) > 0 {
			if app.GPU.Required {
				return nil, fmt.Errorf("GPU required but device(s) not found on host: %s", strings.Join(missing, ", "))
			}
			// Non-required: silently use only available devices
			job.Devices = available
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

	// Stop container
	ctx.transition("stopping")
	ctx.info("Stopping container %d...", inst.CTID)
	bgCtx := context.Background()

	// Try graceful shutdown first, then force stop
	ctx.info("Attempting graceful shutdown of container %d (timeout 30s)...", inst.CTID)
	if err := e.cm.Shutdown(bgCtx, inst.CTID, 30); err != nil {
		ctx.warn("Graceful shutdown failed: %v — forcing stop", err)
		if err := e.cm.Stop(bgCtx, inst.CTID); err != nil {
			ctx.warn("Force stop error: %v", err)
		}
	}
	ctx.info("Shutdown/stop command completed for container %d", inst.CTID)

	// If keeping volumes, detach managed volumes before destroy (bind mounts don't need detaching)
	if keepVolumes && len(inst.MountPoints) > 0 {
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
				ctx.warn("Failed to detach mount points: %v — volumes may be destroyed", err)
			}
		}
	}

	// Destroy container with retries (container may need a moment to fully stop)
	ctx.transition("destroying")
	ctx.info("Destroying container %d...", inst.CTID)
	var destroyErr error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			ctx.info("Retry %d: waiting before destroy attempt...", attempt)
			time.Sleep(5 * time.Second)
		}
		destroyErr = e.cm.Destroy(bgCtx, inst.CTID)
		if destroyErr == nil {
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
	IP              string `json:"ip,omitempty"`
	Uptime          int64  `json:"uptime"`
	CatalogVersion  string `json:"catalog_version,omitempty"`
	UpdateAvailable bool   `json:"update_available,omitempty"`
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
	if inst.Status == "uninstalled" {
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
			destroyErr = e.cm.Destroy(bgCtx, inst.CTID)
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

	// Stop container
	ctx.transition("stopping")
	if err := e.cm.Shutdown(bgCtx, stack.CTID, 30); err != nil {
		ctx.warn("Graceful shutdown failed: %v — forcing stop", err)
		_ = e.cm.Stop(bgCtx, stack.CTID)
	}

	// Destroy container with retries
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

	// Remove stack record
	e.store.DeleteStack(stack.ID)

	ctx.transition(StateCompleted)
	now := time.Now()
	job.CompletedAt = &now
	e.store.UpdateJob(job)
	ctx.info("Stack %s uninstalled. Container %d destroyed.", stack.Name, stack.CTID)
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
