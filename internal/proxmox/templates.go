package proxmox

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// Template represents a container template in Proxmox storage.
type Template struct {
	Volid string `json:"volid"`
	Size  int64  `json:"size"`
}

// APLInfo represents an entry from the Proxmox appliance list (pveam available).
type APLInfo struct {
	Template string `json:"template"`
	Section  string `json:"section"`
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

// ListAvailableTemplates returns downloadable templates from the Proxmox appliance list.
func (c *Client) ListAvailableTemplates(ctx context.Context) ([]APLInfo, error) {
	path := fmt.Sprintf("/nodes/%s/aplinfo", c.node)
	var entries []APLInfo
	if err := c.doRequest(ctx, "GET", path, nil, &entries); err != nil {
		return nil, fmt.Errorf("listing available templates: %w", err)
	}
	return entries, nil
}

// DownloadTemplate downloads a template to the given storage.
func (c *Client) DownloadTemplate(ctx context.Context, storage, template string) error {
	path := fmt.Sprintf("/nodes/%s/aplinfo", c.node)
	params := url.Values{}
	params.Set("storage", storage)
	params.Set("template", template)

	var upid string
	if err := c.doRequest(ctx, "POST", path, params, &upid); err != nil {
		return fmt.Errorf("downloading template %s: %w", template, err)
	}
	return c.WaitForTask(ctx, upid, defaultTaskTimeout)
}

// ResolveTemplate finds a template matching the short name (e.g., "debian-12")
// from the available templates on the given storage. If not found locally,
// it searches the Proxmox appliance list and downloads it automatically.
func (c *Client) ResolveTemplate(ctx context.Context, name, storage string) string {
	// 1. Check if template already exists on the target storage
	templates, err := c.ListTemplates(ctx, storage)
	if err == nil {
		for _, t := range templates {
			if strings.Contains(strings.ToLower(t.Volid), name+"-standard") {
				return t.Volid
			}
		}
	}

	// Also check local storage (templates are often stored there)
	if storage != "local" {
		templates, err = c.ListTemplates(ctx, "local")
		if err == nil {
			for _, t := range templates {
				if strings.Contains(strings.ToLower(t.Volid), name+"-standard") {
					return t.Volid
				}
			}
		}
	}

	// 2. Not found locally — search appliance list and download
	available, err := c.ListAvailableTemplates(ctx)
	if err == nil {
		for _, a := range available {
			if strings.Contains(strings.ToLower(a.Template), name+"-standard") {
				if dlErr := c.DownloadTemplate(ctx, "local", a.Template); dlErr == nil {
					return "local:vztmpl/" + a.Template
				}
				break
			}
		}
	}

	// 3. Fallback: standard pattern (will likely fail, but gives a clear error)
	return fmt.Sprintf("local:vztmpl/%s-standard_amd64.tar.zst", name)
}
