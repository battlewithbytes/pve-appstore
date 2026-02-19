package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/battlewithbytes/pve-appstore/internal/updater"
	"github.com/battlewithbytes/pve-appstore/internal/version"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(selfUpdateCmd)
}

var selfUpdateCmd = &cobra.Command{
	Use:   "self-update",
	Short: "Update pve-appstore to the latest release",
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Getuid() != 0 {
			return fmt.Errorf("self-update must be run as root")
		}

		fmt.Printf("Current version: %s\n", version.Version)
		fmt.Println("Checking for updates...")

		u := updater.New()
		status, err := u.CheckLatestRelease(version.Version)
		if err != nil {
			return fmt.Errorf("failed to check for updates: %w", err)
		}

		fmt.Printf("Latest version:  %s\n", status.Latest)

		if !status.Available {
			fmt.Println("\nAlready up to date.")
			return nil
		}

		if status.Release == nil || status.Release.DownloadURL == "" {
			return fmt.Errorf("no binary found for this architecture")
		}

		fmt.Printf("\nDownloading v%s...\n", status.Release.Version)
		tmpPath := updater.TempBinary
		if err := updater.DownloadBinary(status.Release.DownloadURL, tmpPath); err != nil {
			return fmt.Errorf("download failed: %w", err)
		}

		// Deploy update script if missing (enables future web updates)
		if !updater.ScriptExists() {
			fmt.Println("Installing update helper script...")
			if err := updater.DeployUpdateScript(); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not deploy update script: %v\n", err)
			}
			if err := appendUpdateSudoers(); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not update sudoers: %v\n", err)
			}
		}

		fmt.Println("Applying update...")
		if err := updater.ApplyUpdateDirect(tmpPath, updater.BinaryPath); err != nil {
			return fmt.Errorf("update failed: %w", err)
		}

		fmt.Printf("\nSuccessfully updated to v%s. Service restarted.\n", status.Release.Version)
		return nil
	},
}

// appendUpdateSudoers adds the update.sh entry to the existing sudoers file if not present.
func appendUpdateSudoers() error {
	const sudoersPath = "/etc/sudoers.d/pve-appstore"
	const marker = "/opt/pve-appstore/update.sh"
	const entry = "appstore ALL=(root) NOPASSWD: /usr/bin/nsenter --mount=/proc/1/ns/mnt -- /opt/pve-appstore/update.sh *\n"

	data, err := os.ReadFile(sudoersPath)
	if err != nil {
		return err
	}

	if strings.Contains(string(data), marker) {
		return nil
	}

	f, err := os.OpenFile(sudoersPath, os.O_APPEND|os.O_WRONLY, 0440)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(entry)
	return err
}
