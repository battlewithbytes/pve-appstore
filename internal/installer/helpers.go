package installer

import (
	"fmt"
	"strconv"
	"strings"
)

// InstallerAnswers holds raw string values from the TUI form.
// Numeric fields are strings because huh.Input binds to *string.
type InstallerAnswers struct {
	// Pool
	PoolChoice string // existing pool name or "__new__"
	NewPool    string

	// Placement
	Storage string
	Bridge  string

	// Resources (string from input, parsed later)
	CoresStr    string
	MemoryMBStr string
	DiskGBStr   string

	// Security
	UnprivilegedOnly bool
	AllowedFeatures  []string

	// Service
	BindAddress string
	PortStr     string

	// Auth
	AuthMode        string
	Password        string
	PasswordConfirm string

	// Proxmox
	AutoCreateToken bool
	TokenID         string
	TokenSecret     string

	// Catalog
	CatalogURL string
	Branch     string
	Refresh    string

	// GPU
	GPUEnabled  bool
	GPUPolicy   string
	GPUDevices  []string

	// Confirmation
	Confirmed bool
}

// ParsedNumerics holds the int values after parsing.
type ParsedNumerics struct {
	Cores    int
	MemoryMB int
	DiskGB   int
	Port     int
}

// ParseNumerics converts string fields to ints.
func (a *InstallerAnswers) ParseNumerics() (*ParsedNumerics, error) {
	cores, err := strconv.Atoi(strings.TrimSpace(a.CoresStr))
	if err != nil || cores < 1 {
		return nil, fmt.Errorf("cores must be a positive integer, got %q", a.CoresStr)
	}

	mem, err := strconv.Atoi(strings.TrimSpace(a.MemoryMBStr))
	if err != nil || mem < 128 {
		return nil, fmt.Errorf("memory must be >= 128 MB, got %q", a.MemoryMBStr)
	}

	disk, err := strconv.Atoi(strings.TrimSpace(a.DiskGBStr))
	if err != nil || disk < 1 {
		return nil, fmt.Errorf("disk must be >= 1 GB, got %q", a.DiskGBStr)
	}

	port, err := strconv.Atoi(strings.TrimSpace(a.PortStr))
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("port must be 1-65535, got %q", a.PortStr)
	}

	return &ParsedNumerics{
		Cores:    cores,
		MemoryMB: mem,
		DiskGB:   disk,
		Port:     port,
	}, nil
}

// ValidatePositiveInt returns nil if s is a positive integer.
func ValidatePositiveInt(s string) error {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return fmt.Errorf("must be a number")
	}
	if n < 1 {
		return fmt.Errorf("must be positive")
	}
	return nil
}

// ValidatePort returns nil if s is a valid port number.
func ValidatePort(s string) error {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return fmt.Errorf("must be a number")
	}
	if n < 1 || n > 65535 {
		return fmt.Errorf("must be 1-65535")
	}
	return nil
}

// ValidateMemory returns nil if s is >= 128.
func ValidateMemory(s string) error {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return fmt.Errorf("must be a number")
	}
	if n < 128 {
		return fmt.Errorf("must be >= 128 MB")
	}
	return nil
}

// EffectivePool returns the pool name from the answers.
func (a *InstallerAnswers) EffectivePool() string {
	if a.PoolChoice == "__new__" {
		return a.NewPool
	}
	return a.PoolChoice
}
