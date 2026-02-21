// Command pve-appstore-helper is a privileged daemon that exposes validated
// container operations over a Unix domain socket. It runs as root and replaces
// the sudoers-based privilege escalation used by the main service.
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/battlewithbytes/pve-appstore/internal/config"
	"github.com/battlewithbytes/pve-appstore/internal/helper"
	_ "modernc.org/sqlite" // SQLite driver for database/sql
)

const (
	socketPath = "/run/pve-appstore/helper.sock"
	configPath = config.DefaultConfigPath
	dbPath     = config.DefaultDataDir + "/jobs.db"
	auditPath  = config.DefaultLogDir + "/helper-audit.log"
)

func main() {
	if os.Getuid() != 0 {
		fmt.Fprintln(os.Stderr, "pve-appstore-helper must run as root")
		os.Exit(1)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	srv, err := helper.NewServer(helper.ServerConfig{
		SocketPath: socketPath,
		DBPath:     dbPath,
		AuditPath:  auditPath,
		Config:     cfg,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create helper server: %v\n", err)
		os.Exit(1)
	}
	defer srv.Close()

	// Remove stale socket
	os.Remove(socketPath)

	// Create listener with restricted permissions (0660 root:appstore)
	oldMask := syscall.Umask(0117) // ~0660
	listener, err := net.Listen("unix", socketPath)
	syscall.Umask(oldMask)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to listen on %s: %v\n", socketPath, err)
		os.Exit(1)
	}
	defer listener.Close()

	// Set socket and directory group to appstore so the main service can connect
	if gid := lookupGroupID(config.ServiceGroup); gid >= 0 {
		os.Chown("/run/pve-appstore", 0, gid)
		os.Chmod("/run/pve-appstore", 0750)
		os.Chown(socketPath, 0, gid)
	}

	fmt.Printf("pve-appstore-helper listening on %s\n", socketPath)

	httpSrv := &http.Server{
		Handler:     srv.Handler(),
		ConnContext: helper.ConnContext,
	}

	// Graceful shutdown on SIGINT/SIGTERM
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := httpSrv.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	}()

	// SIGHUP reloads config
	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	go func() {
		for range sighup {
			fmt.Println("Reloading config...")
			newCfg, err := config.Load(configPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "config reload failed: %v\n", err)
				continue
			}
			srv.ReloadConfig(newCfg)
			fmt.Println("Config reloaded")
		}
	}()

	<-sig
	fmt.Println("\nShutting down...")
	httpSrv.Shutdown(context.Background())
}

// lookupGroupID returns the GID for a group name, or -1 if not found.
func lookupGroupID(name string) int {
	data, err := os.ReadFile("/etc/group")
	if err != nil {
		return -1
	}
	for _, line := range splitLines(data) {
		parts := splitColon(line)
		if len(parts) >= 3 && parts[0] == name {
			var gid int
			fmt.Sscanf(parts[2], "%d", &gid)
			return gid
		}
	}
	return -1
}

func splitLines(data []byte) []string {
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, string(data[start:i]))
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}

func splitColon(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
