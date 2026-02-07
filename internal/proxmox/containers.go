package proxmox

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
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
	Hostname     string
	Unprivileged bool
	Pool         string
	Features     []string
	OnBoot       bool
	Tags         string
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
	params.Set("net0", fmt.Sprintf("name=eth0,bridge=%s,ip=dhcp", opts.Bridge))
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
func (c *Client) Destroy(ctx context.Context, ctid int) error {
	path := fmt.Sprintf("/nodes/%s/lxc/%d", c.node, ctid)
	var upid string
	if err := c.doRequest(ctx, "DELETE", path, nil, &upid); err != nil {
		return fmt.Errorf("destroying container %d: %w", ctid, err)
	}
	return c.WaitForTask(ctx, upid, defaultTaskTimeout)
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
