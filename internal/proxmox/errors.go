package proxmox

import (
	"errors"
	"fmt"
	"strings"
)

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

// IsNotFound returns true if the error indicates a container does not exist.
// Proxmox returns HTTP 500 with "does not exist" for missing config files.
func IsNotFound(err error) bool {
	var pErr *ProxmoxError
	if errors.As(err, &pErr) {
		return pErr.StatusCode == 500 && strings.Contains(pErr.Message, "does not exist")
	}
	return false
}
