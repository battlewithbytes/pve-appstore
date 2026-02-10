package proxmox

import "fmt"

// ProxmoxError represents an error response from the Proxmox API.
type ProxmoxError struct {
	StatusCode int
	Message    string
	Errors     map[string]string
}

func (e *ProxmoxError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("proxmox API %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("proxmox API %d", e.StatusCode)
}
