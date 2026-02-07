package proxmox

import (
	"context"

	"github.com/inertz/pve-appstore/internal/engine"
	"github.com/inertz/pve-appstore/internal/pct"
)

// Manager adapts the Proxmox API Client to the engine.ContainerManager interface.
// API-backed operations delegate to *Client; shell-only operations (Exec, Push, GetIP)
// delegate to the pct package since they have no REST API equivalent.
type Manager struct {
	client *Client
}

// compile-time check that Manager implements engine.ContainerManager
var _ engine.ContainerManager = (*Manager)(nil)

// NewManager creates a new Manager wrapping the given Client.
func NewManager(client *Client) *Manager {
	return &Manager{client: client}
}

func (m *Manager) AllocateCTID(ctx context.Context) (int, error) {
	return m.client.AllocateCTID(ctx)
}

func (m *Manager) Create(ctx context.Context, opts engine.CreateOptions) error {
	return m.client.Create(ctx, ContainerCreateOptions{
		CTID:         opts.CTID,
		OSTemplate:   opts.OSTemplate,
		Storage:      opts.Storage,
		RootFSSize:   opts.RootFSSize,
		Cores:        opts.Cores,
		MemoryMB:     opts.MemoryMB,
		Bridge:       opts.Bridge,
		Hostname:     opts.Hostname,
		Unprivileged: opts.Unprivileged,
		Pool:         opts.Pool,
		Features:     opts.Features,
		OnBoot:       opts.OnBoot,
		Tags:         opts.Tags,
	})
}

func (m *Manager) Start(ctx context.Context, ctid int) error {
	return m.client.Start(ctx, ctid)
}

func (m *Manager) Stop(ctx context.Context, ctid int) error {
	return m.client.Stop(ctx, ctid)
}

func (m *Manager) Shutdown(ctx context.Context, ctid int, timeout int) error {
	return m.client.Shutdown(ctx, ctid, timeout)
}

func (m *Manager) Destroy(ctx context.Context, ctid int) error {
	return m.client.Destroy(ctx, ctid)
}

func (m *Manager) Status(ctx context.Context, ctid int) (string, error) {
	return m.client.Status(ctx, ctid)
}

func (m *Manager) ResolveTemplate(ctx context.Context, name, storage string) string {
	return m.client.ResolveTemplate(ctx, name, storage)
}

// Shell-based operations — no API equivalent.

func (m *Manager) Exec(ctid int, command []string) (*pct.ExecResult, error) {
	return pct.Exec(ctid, command)
}

func (m *Manager) ExecScript(ctid int, scriptPath string, env map[string]string) (*pct.ExecResult, error) {
	return pct.ExecScript(ctid, scriptPath, env)
}

func (m *Manager) Push(ctid int, src, dst, perms string) error {
	return pct.Push(ctid, src, dst, perms)
}

func (m *Manager) GetIP(ctid int) (string, error) {
	return pct.GetIP(ctid)
}
