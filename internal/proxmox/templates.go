package proxmox

import (
	"context"
	"fmt"
	"strings"
)

// Template represents a container template in Proxmox storage.
type Template struct {
	Volid string `json:"volid"`
	Size  int64  `json:"size"`
}

// ListTemplates returns available container templates from a storage.
func (c *Client) ListTemplates(ctx context.Context, storage string) ([]Template, error) {
	path := fmt.Sprintf("/nodes/%s/storage/%s/content", c.node, storage)
	// Filter to vztmpl content type via query parameter
	params := make(map[string][]string)
	params["content"] = []string{"vztmpl"}

	var templates []Template
	if err := c.doRequest(ctx, "GET", path, params, &templates); err != nil {
		return nil, fmt.Errorf("listing templates on %s: %w", storage, err)
	}
	return templates, nil
}

// ResolveTemplate finds a template matching the short name (e.g., "debian-12")
// from the available templates on the given storage.
func (c *Client) ResolveTemplate(ctx context.Context, name, storage string) string {
	templates, err := c.ListTemplates(ctx, storage)
	if err == nil {
		for _, t := range templates {
			lower := strings.ToLower(t.Volid)
			if strings.Contains(lower, name+"-standard") {
				return t.Volid
			}
		}
	}

	// Fallback: standard pattern
	return fmt.Sprintf("local:vztmpl/%s-standard_amd64.tar.zst", name)
}
