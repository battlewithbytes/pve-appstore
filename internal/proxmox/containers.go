package proxmox

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/engine"
)

const defaultTaskTimeout = 5 * time.Minute

// ContainerCreateOptions defines the parameters for creating a new LXC container.
type ContainerCreateOptions struct {
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
	MountPoints  []engine.MountPointOption
}

// Create creates a new LXC container via the Proxmox API.
func (c *Client) Create(ctx context.Context, opts ContainerCreateOptions) error {
	params := url.Values{}
	params.Set("vmid", strconv.Itoa(opts.CTID))
	params.Set("ostemplate", opts.OSTemplate)
	params.Set("storage", opts.Storage)
	params.Set("rootfs", fmt.Sprintf("%s:%d", opts.Storage, opts.RootFSSize))
	params.Set("cores", strconv.Itoa(opts.Cores))
	params.Set("memory", strconv.Itoa(opts.MemoryMB))
	params.Set("hostname", opts.Hostname)
	netCfg := fmt.Sprintf("name=eth0,bridge=%s", opts.Bridge)
	if opts.HWAddr != "" {
		netCfg += ",hwaddr=" + opts.HWAddr
	}
	if opts.IPAddress != "" {
		netCfg += ",ip=" + formatIPConfig(opts.IPAddress)
	} else {
		netCfg += ",ip=dhcp"
	}
	params.Set("net0", netCfg)
	params.Set("start", "0")

	if opts.Unprivileged {
		params.Set("unprivileged", "1")
	}
	if opts.Pool != "" {
		params.Set("pool", opts.Pool)
	}
	if len(opts.Features) > 0 {
		featureParts := make([]string, 0, len(opts.Features))
		for _, f := range opts.Features {
			featureParts = append(featureParts, f+"=1")
		}
		params.Set("features", strings.Join(featureParts, ","))
	}
	if opts.OnBoot {
		params.Set("onboot", "1")
	}
	if opts.Tags != "" {
		params.Set("tags", opts.Tags)
	}

	// Mount points
	for _, mp := range opts.MountPoints {
		key := fmt.Sprintf("mp%d", mp.Index)
		var val string
		switch {
		case mp.Type == "bind":
			// Bind mount: mp0=/host/path,mp=/container/path[,ro=1]
			val = fmt.Sprintf("%s,mp=%s", mp.HostPath, mp.MountPath)
			if mp.ReadOnly {
				val += ",ro=1"
			}
		case mp.VolumeID != "":
			// Reattach existing managed volume
			val = fmt.Sprintf("%s,mp=%s", mp.VolumeID, mp.MountPath)
		default:
			// New managed volume
			val = fmt.Sprintf("%s:%d,mp=%s", mp.Storage, mp.SizeGB, mp.MountPath)
		}
		params.Set(key, val)
	}

	// NOTE: Device passthrough (dev*) is NOT included here because the Proxmox
	// API restricts it to root@pam only. Devices are applied post-creation via
	// pct set in the configure_container step.

	path := fmt.Sprintf("/nodes/%s/lxc", c.node)
	var upid string
	if err := c.doRequest(ctx, "POST", path, params, &upid); err != nil {
		return fmt.Errorf("creating container %d: %w", opts.CTID, err)
	}

	return c.WaitForTask(ctx, upid, defaultTaskTimeout)
}

// Start starts an LXC container.
func (c *Client) Start(ctx context.Context, ctid int) error {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/status/start", c.node, ctid)
	var upid string
	if err := c.doRequest(ctx, "POST", path, nil, &upid); err != nil {
		return fmt.Errorf("starting container %d: %w", ctid, err)
	}
	return c.WaitForTask(ctx, upid, defaultTaskTimeout)
}

// Stop force-stops an LXC container.
func (c *Client) Stop(ctx context.Context, ctid int) error {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/status/stop", c.node, ctid)
	var upid string
	if err := c.doRequest(ctx, "POST", path, nil, &upid); err != nil {
		return fmt.Errorf("stopping container %d: %w", ctid, err)
	}
	return c.WaitForTask(ctx, upid, defaultTaskTimeout)
}

// Shutdown gracefully shuts down an LXC container.
func (c *Client) Shutdown(ctx context.Context, ctid int, timeout int) error {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/status/shutdown", c.node, ctid)
	params := url.Values{}
	if timeout > 0 {
		params.Set("timeout", strconv.Itoa(timeout))
	}
	var upid string
	if err := c.doRequest(ctx, "POST", path, params, &upid); err != nil {
		return fmt.Errorf("shutting down container %d: %w", ctid, err)
	}
	return c.WaitForTask(ctx, upid, defaultTaskTimeout)
}

// Destroy destroys an LXC container.
func (c *Client) Destroy(ctx context.Context, ctid int, keepVolumes ...bool) error {
	path := fmt.Sprintf("/nodes/%s/lxc/%d", c.node, ctid)
	if len(keepVolumes) > 0 && keepVolumes[0] {
		// Preserve detached volumes by telling Proxmox not to destroy
		// datasets that are no longer referenced in the container config.
		path += "?destroy-unreferenced-disks=0"
	}
	var upid string
	if err := c.doRequest(ctx, "DELETE", path, nil, &upid); err != nil {
		return fmt.Errorf("destroying container %d: %w", ctid, err)
	}
	return c.WaitForTask(ctx, upid, defaultTaskTimeout)
}

// ContainerInfo holds summary information about an LXC container from the list endpoint.
type ContainerInfo struct {
	VMID   int    `json:"vmid"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Tags   string `json:"tags"`
}

// ListContainers returns all LXC containers on the configured node.
func (c *Client) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	path := fmt.Sprintf("/nodes/%s/lxc", c.node)
	var containers []ContainerInfo
	if err := c.doRequest(ctx, "GET", path, nil, &containers); err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}
	return containers, nil
}

// containerStatus holds the current status of a container.
type containerStatus struct {
	Status string `json:"status"`
}

// Status returns the current status of a container (e.g., "running", "stopped").
func (c *Client) Status(ctx context.Context, ctid int) (string, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/status/current", c.node, ctid)
	var cs containerStatus
	if err := c.doRequest(ctx, "GET", path, nil, &cs); err != nil {
		return "", fmt.Errorf("getting status of container %d: %w", ctid, err)
	}
	return cs.Status, nil
}

// containerStatusDetail holds the full response from /status/current.
type containerStatusDetail struct {
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

// StatusDetail returns detailed runtime stats for a container.
func (c *Client) StatusDetail(ctx context.Context, ctid int) (*containerStatusDetail, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/status/current", c.node, ctid)
	var cs containerStatusDetail
	if err := c.doRequest(ctx, "GET", path, nil, &cs); err != nil {
		return nil, fmt.Errorf("getting status detail of container %d: %w", ctid, err)
	}
	return &cs, nil
}

// GetConfig returns the full config of a container as a key-value map.
func (c *Client) GetConfig(ctx context.Context, ctid int) (map[string]interface{}, error) {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/config", c.node, ctid)
	var config map[string]interface{}
	if err := c.doRequest(ctx, "GET", path, nil, &config); err != nil {
		return nil, fmt.Errorf("getting config of container %d: %w", ctid, err)
	}
	return config, nil
}

// DetachMountPoints removes mount point entries from container config without destroying volumes.
func (c *Client) DetachMountPoints(ctx context.Context, ctid int, indexes []int) error {
	path := fmt.Sprintf("/nodes/%s/lxc/%d/config", c.node, ctid)
	deleteKeys := make([]string, len(indexes))
	for i, idx := range indexes {
		deleteKeys[i] = fmt.Sprintf("mp%d", idx)
	}
	params := url.Values{"delete": {strings.Join(deleteKeys, ",")}}
	return c.doRequest(ctx, "PUT", path, params, nil)
}

// formatIPConfig formats an IP address for Proxmox net0 configuration.
// If the input already contains "/" (CIDR notation), it's passed through as-is.
// Otherwise, /24 is appended and a gateway is derived by replacing the last octet with .1.
func formatIPConfig(ip string) string {
	if strings.Contains(ip, "/") {
		// Already has CIDR; check if gateway is included
		if strings.Contains(ip, ",gw=") {
			return ip
		}
		// Has CIDR but no gateway — derive gateway from the IP portion
		ipPart := ip[:strings.Index(ip, "/")]
		return ip + ",gw=" + deriveGateway(ipPart)
	}
	return ip + "/24,gw=" + deriveGateway(ip)
}

// deriveGateway replaces the last octet of an IPv4 address with .1.
func deriveGateway(ip string) string {
	lastDot := strings.LastIndex(ip, ".")
	if lastDot < 0 {
		return ip
	}
	return ip[:lastDot] + ".1"
}
