package installer

import (
	"fmt"
	"net"

	"github.com/battlewithbytes/pve-appstore/internal/config"
)

// Run is the main entrypoint for the TUI installer.
func Run() error {
	fmt.Println("Discovering Proxmox resources...")

	res, err := Discover()
	if err != nil {
		return fmt.Errorf("discovery failed: %w", err)
	}

	answers := &InstallerAnswers{}
	form := BuildForm(res, answers)

	if err := form.Run(); err != nil {
		return fmt.Errorf("installer cancelled: %w", err)
	}

	if !answers.Confirmed {
		fmt.Println("Installation cancelled.")
		return nil
	}

	// Validate port is available before proceeding
	nums, err := answers.ParseNumerics()
	if err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	if err := checkPortAvailable(answers.BindAddress, nums.Port); err != nil {
		return fmt.Errorf("port %d is already in use: %w", nums.Port, err)
	}

	fmt.Println()
	fmt.Println("Installing PVE App Store...")
	fmt.Println()

	if err := ApplySystem(answers, res); err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	// Resolve display address
	displayAddr := answers.BindAddress
	if displayAddr == "0.0.0.0" || displayAddr == "" {
		if ip := getPrimaryIP(); ip != "" {
			displayAddr = ip
		}
	}

	fmt.Println()
	fmt.Println("Installation complete!")
	fmt.Println()
	fmt.Printf("  Web UI:    http://%s:%d\n", displayAddr, nums.Port)
	fmt.Printf("  Health:    http://%s:%d/api/health\n", displayAddr, nums.Port)
	fmt.Printf("  Config:    %s\n", config.DefaultConfigPath)
	fmt.Printf("  Logs:      %s/\n", config.DefaultLogDir)
	fmt.Printf("  Service:   systemctl status pve-appstore\n")
	fmt.Println()

	return nil
}

// checkPortAvailable tries to listen on the port to verify it's free.
func checkPortAvailable(addr string, port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", addr, port))
	if err != nil {
		return err
	}
	ln.Close()
	return nil
}

// getPrimaryIP returns the first non-loopback IPv4 address of the host.
func getPrimaryIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			return ipNet.IP.String()
		}
	}
	return ""
}
