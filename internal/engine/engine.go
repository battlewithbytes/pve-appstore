// Package engine orchestrates install/uninstall jobs for the PVE App Store.
package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
	"github.com/battlewithbytes/pve-appstore/internal/config"
	"github.com/battlewithbytes/pve-appstore/internal/pct"
)

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
	ResolveTemplate(ctx context.Context, name, storage string) string
	Exec(ctid int, command []string) (*pct.ExecResult, error)
	ExecScript(ctid int, scriptPath string, env map[string]string) (*pct.ExecResult, error)
	Push(ctid int, src, dst, perms string) error
	GetIP(ctid int) (string, error)
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
	Unprivileged bool
	Pool         string
	Features     []string
	OnBoot       bool
	Tags         string
}

// Engine manages the job lifecycle.
type Engine struct {
	cfg     *config.Config
	catalog *catalog.Catalog
	store   *Store
	cm      ContainerManager
}

// New creates a new Engine, opening the SQLite database.
func New(cfg *config.Config, cat *catalog.Catalog, dataDir string, cm ContainerManager) (*Engine, error) {
	dbPath := filepath.Join(dataDir, "jobs.db")
	store, err := NewStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening job store: %w", err)
	}

	return &Engine{
		cfg:     cfg,
		catalog: cat,
		store:   store,
		cm:      cm,
	}, nil
}

// Close closes the engine's resources.
func (e *Engine) Close() error {
	return e.store.Close()
}

// StartInstall creates a new install job and runs it asynchronously.
func (e *Engine) StartInstall(req InstallRequest) (*Job, error) {
	// Look up the app
	app, ok := e.catalog.Get(req.AppID)
	if !ok {
		return nil, fmt.Errorf("app %q not found in catalog", req.AppID)
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

	now := time.Now()
	job := &Job{
		ID:        generateID(),
		Type:      JobTypeInstall,
		State:     StateQueued,
		AppID:     req.AppID,
		AppName:   app.Name,
		Node:      e.cfg.NodeName,
		Pool:      e.cfg.Pool,
		Storage:   storage,
		Bridge:    bridge,
		Cores:     cores,
		MemoryMB:  memoryMB,
		DiskGB:    diskGB,
		Inputs:    req.Inputs,
		Outputs:   make(map[string]string),
		CreatedAt: now,
		UpdatedAt: now,
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
		return nil, fmt.Errorf("creating job: %w", err)
	}

	// Run asynchronously
	go e.runInstall(job)

	return job, nil
}

// Uninstall stops and destroys a container for an installed app.
func (e *Engine) Uninstall(installID string) (*Job, error) {
	// Find the install
	installs, err := e.store.ListInstalls()
	if err != nil {
		return nil, fmt.Errorf("listing installs: %w", err)
	}

	var target *Install
	for _, inst := range installs {
		if inst.ID == installID {
			target = inst
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("install %q not found", installID)
	}

	now := time.Now()
	job := &Job{
		ID:        generateID(),
		Type:      JobTypeUninstall,
		State:     StateQueued,
		AppID:     target.AppID,
		AppName:   target.AppName,
		CTID:      target.CTID,
		Node:      target.Node,
		Pool:      target.Pool,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := e.store.CreateJob(job); err != nil {
		return nil, fmt.Errorf("creating uninstall job: %w", err)
	}

	go e.runUninstall(job, target)

	return job, nil
}

func (e *Engine) runUninstall(job *Job, inst *Install) {
	ctx := &installContext{engine: e, job: job}

	ctx.info("Starting uninstall of %s (CTID %d)", inst.AppName, inst.CTID)

	// Stop container
	ctx.transition("stopping")
	ctx.info("Stopping container %d...", inst.CTID)
	bgCtx := context.Background()
	status, _ := e.cm.Status(bgCtx, inst.CTID)
	if status == "running" {
		if err := e.cm.Shutdown(bgCtx, inst.CTID, 30); err != nil {
			ctx.warn("Graceful shutdown failed, forcing stop: %v", err)
			e.cm.Stop(bgCtx, inst.CTID)
		}
	}

	// Destroy container
	ctx.transition("destroying")
	ctx.info("Destroying container %d...", inst.CTID)
	if err := e.cm.Destroy(bgCtx, inst.CTID); err != nil {
		ctx.log("error", "Failed to destroy container: %v", err)
		job.State = StateFailed
		job.Error = fmt.Sprintf("destroy: %v", err)
		now := time.Now()
		job.UpdatedAt = now
		job.CompletedAt = &now
		e.store.UpdateJob(job)
		return
	}

	// Remove install record
	e.store.db.Exec("DELETE FROM installs WHERE id=?", inst.ID)

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

// ListInstalls returns all installations.
func (e *Engine) ListInstalls() ([]*Install, error) {
	return e.store.ListInstalls()
}

func generateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
