package installer

import (
	"fmt"
	"net"

	"github.com/battlewithbytes/pve-appstore/internal/config"
	"github.com/battlewithbytes/pve-appstore/internal/ui"
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
	fmt.Println(ui.Green.Render("Installation complete!"))
	fmt.Println()
	fmt.Printf("  %s %s\n", ui.Dim.Render("Web UI: "), ui.Cyan.Render(fmt.Sprintf("http://%s:%d", displayAddr, nums.Port)))
	fmt.Printf("  %s %s\n", ui.Dim.Render("Health: "), ui.Cyan.Render(fmt.Sprintf("http://%s:%d/api/health", displayAddr, nums.Port)))
	fmt.Printf("  %s %s\n", ui.Dim.Render("Config: "), ui.Cyan.Render(config.DefaultConfigPath))
	fmt.Printf("  %s %s\n", ui.Dim.Render("Logs:   "), ui.Cyan.Render(config.DefaultLogDir+"/"))
	fmt.Printf("  %s %s\n", ui.Dim.Render("Service:"), ui.Cyan.Render("systemctl status pve-appstore"))
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
