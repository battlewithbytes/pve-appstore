package main

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
	"github.com/battlewithbytes/pve-appstore/internal/config"
	"github.com/battlewithbytes/pve-appstore/internal/engine"
	"github.com/battlewithbytes/pve-appstore/internal/proxmox"
	"github.com/battlewithbytes/pve-appstore/internal/server"
	"github.com/battlewithbytes/pve-appstore/web"
	"github.com/spf13/cobra"
)

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

var (
	serveConfigPath string
	serveCatalogDir string
	serveDataDir    string
)

func init() {
	serveCmd.Flags().StringVar(&serveConfigPath, "config", config.DefaultConfigPath, "path to config file")
	serveCmd.Flags().StringVar(&serveCatalogDir, "catalog-dir", "", "load catalog from local directory (instead of git)")
	serveCmd.Flags().StringVar(&serveDataDir, "data-dir", config.DefaultDataDir, "path to data directory (for jobs.db)")
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the PVE App Store service",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(serveConfigPath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}

		fmt.Printf("PVE App Store starting...\n")
		fmt.Printf("  node:    %s\n", cfg.NodeName)
		fmt.Printf("  pool:    %s\n", cfg.Pool)
		fmt.Printf("  listen:  %s:%d\n", cfg.Service.BindAddress, cfg.Service.Port)
		fmt.Printf("  catalog: %s (%s)\n", cfg.Catalog.URL, cfg.Catalog.Branch)
		fmt.Printf("  auth:    %s\n", cfg.Auth.Mode)
		fmt.Printf("  gpu:     enabled=%v policy=%s\n", cfg.GPU.Enabled, cfg.GPU.Policy)

		// Initialize catalog
		cat := catalog.New(cfg.Catalog.URL, cfg.Catalog.Branch, serveDataDir)

		if serveCatalogDir != "" {
			fmt.Printf("  catalog: loading from local directory %s\n", serveCatalogDir)
			if err := cat.LoadLocal(serveCatalogDir); err != nil {
				return fmt.Errorf("loading local catalog: %w", err)
			}
		} else {
			fmt.Printf("  catalog: fetching from %s...\n", cfg.Catalog.URL)
			if err := cat.Refresh(); err != nil {
				fmt.Fprintf(os.Stderr, "warning: catalog refresh failed: %v (starting with empty catalog)\n", err)
			}
		}

		fmt.Printf("  apps:    %d loaded\n", cat.AppCount())

		// Initialize Proxmox API client
		pmClient, err := proxmox.NewClient(proxmox.ClientConfig{
			BaseURL:       cfg.Proxmox.BaseURL,
			Node:          cfg.NodeName,
			TokenID:       cfg.Proxmox.TokenID,
			TokenSecret:   cfg.Proxmox.TokenSecret,
			TLSSkipVerify: cfg.Proxmox.TLSSkipVerify,
			TLSCACertPath: cfg.Proxmox.TLSCACertPath,
		})
		if err != nil {
			return fmt.Errorf("creating proxmox client: %w", err)
		}
		cm := proxmox.NewManager(pmClient)

		// Initialize install engine
		os.MkdirAll(serveDataDir, 0750)
		eng, err := engine.New(cfg, cat, serveDataDir, cm)
		if err != nil {
			return fmt.Errorf("initializing engine: %w", err)
		}
		defer eng.Close()
		fmt.Printf("  engine:  ready (db: %s/jobs.db)\n", serveDataDir)

		// SPA assets: prefer disk in dev mode, fall back to embedded FS
		var spaFS fs.FS
		if info, err := os.Stat("web/frontend/dist"); err == nil && info.IsDir() {
			spaFS = os.DirFS("web/frontend/dist")
			fmt.Printf("  spa:     serving from disk (web/frontend/dist)\n")
		} else if sub, err := fs.Sub(web.FrontendFS, "frontend/dist"); err == nil {
			spaFS = sub
			fmt.Printf("  spa:     serving from embedded binary\n")
		} else {
			fmt.Println("  spa:     no frontend build found (API-only mode)")
		}

		// Start server
		srv := server.New(cfg, cat, eng, spaFS)

		// Graceful shutdown
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			addr := srv.Addr()
			if strings.HasPrefix(addr, "0.0.0.0:") {
				if ip := getPrimaryIP(); ip != "" {
					fmt.Printf("\nListening on http://%s (http://%s)\n", addr, ip+addr[len("0.0.0.0"):])
				} else {
					fmt.Printf("\nListening on http://%s\n", addr)
				}
			} else {
				fmt.Printf("\nListening on http://%s\n", addr)
			}
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				fmt.Fprintf(os.Stderr, "server error: %v\n", err)
				os.Exit(1)
			}
		}()

		<-sig
		fmt.Println("\nShutting down...")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	},
}
