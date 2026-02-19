package proxmox

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const taskPollInterval = 2 * time.Second

// taskStatus represents the status of a Proxmox task.
type taskStatus struct {
	Status     string `json:"status"`
	ExitStatus string `json:"exitstatus"`
}

// taskLogEntry represents a single line from the Proxmox task log.
type taskLogEntry struct {
	N int    `json:"n"`
	T string `json:"t"`
}

// fetchTaskLog retrieves the task log from Proxmox for inclusion in error messages.
func (c *Client) fetchTaskLog(ctx context.Context, upid string) string {
	path := fmt.Sprintf("/nodes/%s/tasks/%s/log?limit=50", c.node, upid)
	var entries []taskLogEntry
	if err := c.doRequest(ctx, "GET", path, nil, &entries); err != nil {
		return ""
	}
	var lines []string
	for _, e := range entries {
		if e.T != "" {
			lines = append(lines, e.T)
		}
	}
	return strings.Join(lines, "\n")
}

// WaitForTask polls a Proxmox UPID until the task completes or the timeout is reached.
func (c *Client) WaitForTask(ctx context.Context, upid string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("task %s timed out after %v", upid, timeout)
		}

		var ts taskStatus
		path := fmt.Sprintf("/nodes/%s/tasks/%s/status", c.node, upid)
		if err := c.doRequest(ctx, "GET", path, nil, &ts); err != nil {
			return fmt.Errorf("polling task %s: %w", upid, err)
		}

		if ts.Status == "stopped" {
			if ts.ExitStatus != "OK" {
				detail := c.fetchTaskLog(ctx, upid)
				if detail != "" {
					return fmt.Errorf("task %s failed: %s\n%s", upid, ts.ExitStatus, detail)
				}
				return fmt.Errorf("task %s failed: %s", upid, ts.ExitStatus)
			}
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(taskPollInterval):
		}
	}
}
