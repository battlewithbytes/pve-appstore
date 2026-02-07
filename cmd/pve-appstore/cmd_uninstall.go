package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/battlewithbytes/pve-appstore/internal/config"
	"github.com/battlewithbytes/pve-appstore/internal/proxmox"
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
		fmt.Println("You will be given the option to remove installed containers.")
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

		// Step 2: Offer to remove managed containers (before config removal)
		containersRemoved := offerContainerRemoval(reader)

		// Step 3: Ask about config
		fmt.Println()
		if confirm(reader, "Also remove configuration? (/etc/pve-appstore/)") {
			run("Removing configuration", func() {
				os.RemoveAll("/etc/pve-appstore")
			})
		} else {
			fmt.Println("  Keeping configuration.")
		}

		// Step 4: Ask about data (jobs.db, catalog cache)
		if confirm(reader, "Also remove data? (/var/lib/pve-appstore/ — job history, catalog cache)") {
			run("Removing data", func() {
				os.RemoveAll(config.DefaultDataDir)
			})
		} else {
			fmt.Println("  Keeping data.")
		}

		// Step 5: Ask about logs
		if confirm(reader, "Also remove logs? (/var/log/pve-appstore/)") {
			run("Removing logs", func() {
				os.RemoveAll(config.DefaultLogDir)
			})
		} else {
			fmt.Println("  Keeping logs.")
		}

		// Step 6: Ask about Proxmox API token/user/role
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

		// Step 7: Ask about system user
		if confirm(reader, "Also remove the 'appstore' system user?") {
			run("Removing system user", func() {
				exec.Command("userdel", config.ServiceUser).Run()
			})
		} else {
			fmt.Println("  Keeping system user.")
		}

		fmt.Println()
		fmt.Println("PVE App Store service has been removed.")

		if !containersRemoved {
			fmt.Println()
			fmt.Println("Your installed containers are still running. To manage them manually:")
			fmt.Println("  pct list              # list all containers")
			fmt.Println("  pct stop <ctid>       # stop a container")
			fmt.Println("  pct destroy <ctid>    # remove a container")
		}

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

// isManaged returns true if the container tags contain "appstore" and "managed".
func isManaged(tags string) bool {
	hasAppstore := false
	hasManaged := false
	for _, t := range strings.Split(tags, ";") {
		t = strings.TrimSpace(t)
		if t == "appstore" {
			hasAppstore = true
		}
		if t == "managed" {
			hasManaged = true
		}
	}
	return hasAppstore && hasManaged
}

// offerContainerRemoval tries to connect to Proxmox, lists managed containers,
// and offers to remove them. Returns true if any containers were removed.
func offerContainerRemoval(reader *bufio.Reader) bool {
	fmt.Println()

	// Load config to get Proxmox credentials
	cfg, err := config.Load(config.DefaultConfigPath)
	if err != nil {
		fmt.Println("  Note: Could not load config — skipping container removal.")
		fmt.Printf("  (%s)\n", err)
		return false
	}

	if cfg.Proxmox.TokenID == "" || cfg.Proxmox.TokenSecret == "" || cfg.Proxmox.BaseURL == "" {
		fmt.Println("  Note: No Proxmox API credentials configured — skipping container removal.")
		return false
	}

	client, err := proxmox.NewClient(proxmox.ClientConfig{
		BaseURL:       cfg.Proxmox.BaseURL,
		Node:          cfg.NodeName,
		TokenID:       cfg.Proxmox.TokenID,
		TokenSecret:   cfg.Proxmox.TokenSecret,
		TLSSkipVerify: cfg.Proxmox.TLSSkipVerify,
		TLSCACertPath: cfg.Proxmox.TLSCACertPath,
	})
	if err != nil {
		fmt.Println("  Note: Could not create Proxmox client — skipping container removal.")
		fmt.Printf("  (%s)\n", err)
		return false
	}

	ctx := context.Background()
	containers, err := client.ListContainers(ctx)
	if err != nil {
		fmt.Println("  Note: Could not list containers from Proxmox API — skipping container removal.")
		fmt.Printf("  (%s)\n", err)
		return false
	}

	// Filter to managed containers
	var managed []proxmox.ContainerInfo
	for _, ct := range containers {
		if isManaged(ct.Tags) {
			managed = append(managed, ct)
		}
	}

	if len(managed) == 0 {
		fmt.Println("  No managed app containers found.")
		return false
	}

	// Sort by VMID for consistent display
	sort.Slice(managed, func(i, j int) bool {
		return managed[i].VMID < managed[j].VMID
	})

	fmt.Printf("  Found %d managed app container(s).\n", len(managed))
	if !confirm(reader, "Also remove installed app containers?") {
		fmt.Println("  Keeping containers.")
		return false
	}

	// Build MultiSelect options
	allLabel := fmt.Sprintf("All containers (%d)", len(managed))
	options := []huh.Option[string]{
		huh.NewOption(allLabel, "all"),
	}
	for _, ct := range managed {
		label := fmt.Sprintf("CT %d — %s [%s]", ct.VMID, ct.Name, ct.Status)
		options = append(options, huh.NewOption(label, strconv.Itoa(ct.VMID)))
	}

	var selected []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select containers to remove:").
				Options(options...).
				Value(&selected),
		),
	)

	if err := form.Run(); err != nil {
		fmt.Printf("  Selection cancelled: %s\n", err)
		return false
	}

	if len(selected) == 0 {
		fmt.Println("  No containers selected.")
		return false
	}

	// Resolve "all" sentinel to individual VMIDs
	removeAll := false
	for _, s := range selected {
		if s == "all" {
			removeAll = true
			break
		}
	}

	var toRemove []int
	if removeAll {
		for _, ct := range managed {
			toRemove = append(toRemove, ct.VMID)
		}
	} else {
		for _, s := range selected {
			vmid, _ := strconv.Atoi(s)
			toRemove = append(toRemove, vmid)
		}
	}

	// Build a lookup of managed containers by VMID for status checks
	managedByID := make(map[int]proxmox.ContainerInfo, len(managed))
	for _, ct := range managed {
		managedByID[ct.VMID] = ct
	}

	// Remove selected containers
	anyRemoved := false
	for _, vmid := range toRemove {
		ct := managedByID[vmid]
		fmt.Printf("  → Removing CT %d (%s)...\n", vmid, ct.Name)

		// Stop if running
		if ct.Status == "running" {
			fmt.Printf("    Shutting down (30s timeout)...")
			err := client.Shutdown(ctx, vmid, 30)
			if err != nil {
				fmt.Printf(" failed, force-stopping...")
				if stopErr := client.Stop(ctx, vmid); stopErr != nil {
					fmt.Printf("\n    Error stopping CT %d: %s — skipping\n", vmid, stopErr)
					continue
				}
			}
			fmt.Println(" stopped.")
		}

		// Destroy
		if err := client.Destroy(ctx, vmid); err != nil {
			fmt.Printf("    Error destroying CT %d: %s\n", vmid, err)
			continue
		}
		fmt.Printf("    CT %d destroyed.\n", vmid)
		anyRemoved = true
	}

	return anyRemoved
}
