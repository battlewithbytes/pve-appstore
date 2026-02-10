package proxmox

import (
	"context"
	"fmt"
	"time"
)

const taskPollInterval = 2 * time.Second

// taskStatus represents the status of a Proxmox task.
type taskStatus struct {
	Status     string `json:"status"`
	ExitStatus string `json:"exitstatus"`
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
