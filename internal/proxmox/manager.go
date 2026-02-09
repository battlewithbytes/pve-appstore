package proxmox

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/battlewithbytes/pve-appstore/internal/engine"
	"github.com/battlewithbytes/pve-appstore/internal/pct"
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
		HWAddr:       opts.HWAddr,
		Hostname:     opts.Hostname,
		IPAddress:    opts.IPAddress,
		Unprivileged: opts.Unprivileged,
		Pool:         opts.Pool,
		Features:     opts.Features,
		OnBoot:       opts.OnBoot,
		Tags:         opts.Tags,
		MountPoints:  opts.MountPoints,
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

func (m *Manager) Destroy(ctx context.Context, ctid int, keepVolumes ...bool) error {
	return m.client.Destroy(ctx, ctid, keepVolumes...)
}

func (m *Manager) Status(ctx context.Context, ctid int) (string, error) {
	return m.client.Status(ctx, ctid)
}

func (m *Manager) StatusDetail(ctx context.Context, ctid int) (*engine.ContainerStatusDetail, error) {
	cs, err := m.client.StatusDetail(ctx, ctid)
	if err != nil {
		return nil, err
	}
	return &engine.ContainerStatusDetail{
		Status:  cs.Status,
		Uptime:  cs.Uptime,
		CPU:     cs.CPU,
		CPUs:    cs.CPUs,
		Mem:     cs.Mem,
		MaxMem:  cs.MaxMem,
		Disk:    cs.Disk,
		MaxDisk: cs.MaxDisk,
		NetIn:   cs.NetIn,
		NetOut:  cs.NetOut,
	}, nil
}

func (m *Manager) ResolveTemplate(ctx context.Context, name, storage string) string {
	return m.client.ResolveTemplate(ctx, name, storage)
}

// Shell-based operations — no API equivalent.

func (m *Manager) Exec(ctid int, command []string) (*pct.ExecResult, error) {
	return pct.Exec(ctid, command)
}

func (m *Manager) ExecStream(ctid int, command []string, onLine func(line string)) (*pct.ExecResult, error) {
	return pct.ExecStream(ctid, command, onLine)
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

func (m *Manager) GetConfig(ctx context.Context, ctid int) (map[string]interface{}, error) {
	return m.client.GetConfig(ctx, ctid)
}

func (m *Manager) DetachMountPoints(ctx context.Context, ctid int, indexes []int) error {
	return m.client.DetachMountPoints(ctx, ctid, indexes)
}

func (m *Manager) ConfigureDevices(ctid int, devices []engine.DevicePassthrough) error {
	for i, dev := range devices {
		val := dev.Path
		if dev.GID > 0 {
			val += fmt.Sprintf(",gid=%d", dev.GID)
		}
		if dev.Mode != "" {
			val += fmt.Sprintf(",mode=%s", dev.Mode)
		}
		if err := pct.Set(ctid, fmt.Sprintf("-dev%d", i), val); err != nil {
			return fmt.Errorf("configuring device %d (%s): %w", i, dev.Path, err)
		}
	}
	return nil
}

func (m *Manager) MountHostPath(ctid int, mpIndex int, hostPath, containerPath string, readOnly bool) error {
	val := fmt.Sprintf("%s,mp=%s", hostPath, containerPath)
	if readOnly {
		val += ",ro=1"
	}
	return pct.Set(ctid, fmt.Sprintf("-mp%d", mpIndex), val)
}

func (m *Manager) AppendLXCConfig(ctid int, lines []string) error {
	return pct.Set(ctid, lines...)
}

func (m *Manager) GetStorageInfo(ctx context.Context, storageID string) (*engine.StorageInfo, error) {
	si, err := m.client.GetStorageInfo(ctx, storageID)
	if err != nil {
		return nil, err
	}

	info := &engine.StorageInfo{
		ID:   si.ID,
		Type: si.Type,
	}

	// Resolve filesystem path based on storage type
	switch si.Type {
	case "zfspool":
		info.Path = si.Mountpoint
		info.Browsable = si.Mountpoint != ""
	case "dir", "nfs", "nfs4", "cifs":
		info.Path = si.Path
		info.Browsable = si.Path != ""
	default:
		// lvmthin, lvm, iscsi, etc. — block storage, not browsable
		info.Browsable = false
	}

	return info, nil
}
