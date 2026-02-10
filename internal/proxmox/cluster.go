package proxmox

import (
	"context"
	"fmt"
	"strconv"
)

// AllocateCTID requests the next available CTID from the Proxmox cluster.
func (c *Client) AllocateCTID(ctx context.Context) (int, error) {
	// GET /cluster/nextid returns a JSON string like "104"
	var idStr string
	if err := c.doRequest(ctx, "GET", "/cluster/nextid", nil, &idStr); err != nil {
		return 0, fmt.Errorf("allocating CTID: %w", err)
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		return 0, fmt.Errorf("parsing CTID %q: %w", idStr, err)
	}

	return id, nil
}
