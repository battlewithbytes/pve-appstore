package server

import (
	"context"
	"net/http"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/engine"
	"github.com/battlewithbytes/pve-appstore/internal/installer"
	"github.com/battlewithbytes/pve-appstore/internal/pct"
	"github.com/battlewithbytes/pve-appstore/internal/updater"
	"github.com/battlewithbytes/pve-appstore/internal/version"
)

func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	status, err := s.updater.CheckLatestRelease(version.Version)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to check for updates: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleApplyUpdate(w http.ResponseWriter, r *http.Request) {
	status, err := s.updater.CheckLatestRelease(version.Version)
	if err != nil {
		writeError(w, http.StatusBadGateway, "failed to check for updates: "+err.Error())
		return
	}
	if !status.Available || status.Release == nil {
		writeError(w, http.StatusBadRequest, "no update available")
		return
	}

	if !updater.ScriptExists() {
		writeError(w, http.StatusBadRequest, "Update script not found. Run `sudo pve-appstore self-update` from CLI for the first update.")
		return
	}

	// Download the new binary
	if err := updater.DownloadBinary(status.Release.DownloadURL, updater.TempBinary); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to download update: "+err.Error())
		return
	}

	// Send response before restarting
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "updating",
		"version": status.Release.Version,
	})

	// Flush response then apply update in background
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		if pct.Helper != nil {
			pct.Helper.ApplyUpdate()
		} else {
			updater.ApplyUpdateSudo(updater.TempBinary)
		}
	}()
}

func (s *Server) handleListGPUs(w http.ResponseWriter, r *http.Request) {
	// Fetch PCI device info via Proxmox REST API (works as appstore user)
	var pciDevices []installer.PCIGPUInfo
	if s.engine != nil {
		if devices, err := s.engine.ListPCIDevices(context.Background()); err == nil {
			for _, d := range devices {
				pciDevices = append(pciDevices, installer.PCIGPUInfo{
					ID:         d.ID,
					Vendor:     d.Vendor,
					VendorName: d.VendorName,
					DeviceName: d.DeviceName,
				})
			}
		}
	}
	gpus := installer.DiscoverGPUs(pciDevices)

	type gpuItem struct {
		Path string `json:"path"`
		Type string `json:"type"`
		Name string `json:"name"`
	}
	type gpuDeviceRef struct {
		Path string `json:"path"`
	}
	type gpuInstallItem struct {
		ID      string         `json:"id"`
		AppName string         `json:"app_name"`
		CTID    int            `json:"ctid"`
		Devices []gpuDeviceRef `json:"devices"`
	}
	type gpuStackItem struct {
		ID      string         `json:"id"`
		Name    string         `json:"name"`
		CTID    int            `json:"ctid"`
		Devices []gpuDeviceRef `json:"devices"`
	}

	var items []gpuItem
	for _, g := range gpus {
		items = append(items, gpuItem{Path: g.Path, Type: g.Type, Name: g.Name})
	}
	if items == nil {
		items = []gpuItem{}
	}

	var gpuInstalls []gpuInstallItem
	var gpuStacks []gpuStackItem

	if s.engine != nil {
		if installs, err := s.engine.ListInstalls(); err == nil {
			for _, inst := range installs {
				if len(inst.Devices) > 0 && inst.Status != "uninstalled" {
					var devs []gpuDeviceRef
					for _, d := range inst.Devices {
						devs = append(devs, gpuDeviceRef{Path: d.Path})
					}
					gpuInstalls = append(gpuInstalls, gpuInstallItem{
						ID: inst.ID, AppName: inst.AppName, CTID: inst.CTID, Devices: devs,
					})
				}
			}
		}
		if stacks, err := s.engine.ListStacks(); err == nil {
			for _, st := range stacks {
				if len(st.Devices) > 0 && st.Status != "uninstalled" {
					var devs []gpuDeviceRef
					for _, d := range st.Devices {
						devs = append(devs, gpuDeviceRef{Path: d.Path})
					}
					gpuStacks = append(gpuStacks, gpuStackItem{
						ID: st.ID, Name: st.Name, CTID: st.CTID, Devices: devs,
					})
				}
			}
		}
	}

	if gpuInstalls == nil {
		gpuInstalls = []gpuInstallItem{}
	}
	if gpuStacks == nil {
		gpuStacks = []gpuStackItem{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"gpus":          items,
		"gpu_installs":  gpuInstalls,
		"gpu_stacks":    gpuStacks,
		"driver_status": engine.DetectDriverStatus(),
	})
}
