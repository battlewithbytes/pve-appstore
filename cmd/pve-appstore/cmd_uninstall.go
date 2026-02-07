package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/battlewithbytes/pve-appstore/internal/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(uninstallCmd)
}

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the PVE App Store service",
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Geteuid() != 0 {
			return fmt.Errorf("uninstall must be run as root")
		}

		reader := bufio.NewReader(os.Stdin)

		fmt.Println("PVE App Store Uninstaller")
		fmt.Println()
		fmt.Println("This will stop and remove the App Store service.")
		fmt.Println("Your installed containers and Proxmox pool will NOT be touched.")
		fmt.Println()

		// Always removed (service infrastructure)
		fmt.Println("The following will be removed:")
		fmt.Println("  - systemd service (pve-appstore.service)")
		fmt.Println("  - sudoers file (/etc/sudoers.d/pve-appstore)")
		fmt.Println("  - binary (/opt/pve-appstore/pve-appstore)")
		fmt.Println("  - SPA assets (/opt/pve-appstore/web/)")
		fmt.Println()

		// Always kept
		fmt.Println("The following will NOT be removed:")
		fmt.Println("  - Proxmox pool and API user/token")
		fmt.Println("  - Any installed LXC containers")
		fmt.Println()

		if !confirm(reader, "Proceed with uninstall?") {
			fmt.Println("Uninstall cancelled.")
			return nil
		}

		fmt.Println()

		// Step 1: Always remove service infrastructure
		run := func(name string, fn func()) {
			fmt.Printf("  → %s...\n", name)
			fn()
		}

		run("Stopping service", func() {
			exec.Command("systemctl", "stop", "pve-appstore.service").Run()
			exec.Command("systemctl", "disable", "pve-appstore.service").Run()
		})
		run("Removing systemd unit", func() {
			os.Remove("/etc/systemd/system/pve-appstore.service")
			exec.Command("systemctl", "daemon-reload").Run()
		})
		run("Removing sudoers", func() {
			os.Remove("/etc/sudoers.d/pve-appstore")
		})
		run("Removing binary and SPA", func() {
			os.Remove(config.DefaultInstallDir + "/pve-appstore")
			os.RemoveAll(config.DefaultInstallDir + "/web")
			// Remove install dir only if empty
			os.Remove(config.DefaultInstallDir)
		})

		// Step 2: Ask about config
		fmt.Println()
		if confirm(reader, "Also remove configuration? (/etc/pve-appstore/)") {
			run("Removing configuration", func() {
				os.RemoveAll("/etc/pve-appstore")
			})
		} else {
			fmt.Println("  Keeping configuration.")
		}

		// Step 3: Ask about data (jobs.db, catalog cache)
		if confirm(reader, "Also remove data? (/var/lib/pve-appstore/ — job history, catalog cache)") {
			run("Removing data", func() {
				os.RemoveAll(config.DefaultDataDir)
			})
		} else {
			fmt.Println("  Keeping data.")
		}

		// Step 4: Ask about logs
		if confirm(reader, "Also remove logs? (/var/log/pve-appstore/)") {
			run("Removing logs", func() {
				os.RemoveAll(config.DefaultLogDir)
			})
		} else {
			fmt.Println("  Keeping logs.")
		}

		// Step 5: Ask about Proxmox API token/user/role
		if confirm(reader, "Also revoke Proxmox API token and remove appstore@pve user/role?") {
			run("Revoking API token", func() {
				exec.Command("pveum", "user", "token", "remove", "appstore@pve", "appstore").Run()
			})
			run("Removing Proxmox user", func() {
				exec.Command("pveum", "user", "delete", "appstore@pve").Run()
			})
			run("Removing AppStoreRole", func() {
				exec.Command("pveum", "role", "delete", "AppStoreRole").Run()
			})
		} else {
			fmt.Println("  Keeping Proxmox API credentials.")
		}

		// Step 6: Ask about system user
		if confirm(reader, "Also remove the 'appstore' system user?") {
			run("Removing system user", func() {
				exec.Command("userdel", config.ServiceUser).Run()
			})
		} else {
			fmt.Println("  Keeping system user.")
		}

		fmt.Println()
		fmt.Println("PVE App Store service has been removed.")
		fmt.Println()
		fmt.Println("Your installed containers are still running. To manage them manually:")
		fmt.Println("  pct list              # list all containers")
		fmt.Println("  pct stop <ctid>       # stop a container")
		fmt.Println("  pct destroy <ctid>    # remove a container")
		fmt.Println()

		return nil
	},
}

// confirm asks a y/N question and defaults to No.
func confirm(reader *bufio.Reader, prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}
