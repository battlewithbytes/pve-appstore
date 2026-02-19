package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/battlewithbytes/pve-appstore/internal/config"
)

// --- Settings handlers ---

type settingsResponse struct {
	Defaults  settingsDefaults  `json:"defaults"`
	Storages  []string          `json:"storages"`
	Bridges   []string          `json:"bridges"`
	Developer settingsDeveloper `json:"developer"`
	Service   settingsService   `json:"service"`
	Auth      settingsAuth      `json:"auth"`
	Catalog   settingsCatalog   `json:"catalog"`
	GPU       settingsGPU       `json:"gpu"`
}

type settingsDefaults struct {
	Cores    int `json:"cores"`
	MemoryMB int `json:"memory_mb"`
	DiskGB   int `json:"disk_gb"`
}

type settingsDeveloper struct {
	Enabled bool `json:"enabled"`
}

type settingsService struct {
	Port int `json:"port"`
}

type settingsAuth struct {
	Mode string `json:"mode"`
}

type settingsCatalog struct {
	Refresh string `json:"refresh"`
}

type settingsGPU struct {
	Enabled bool   `json:"enabled"`
	Policy  string `json:"policy"`
}

type settingsAuthUpdate struct {
	Mode     string `json:"mode"`
	Password string `json:"password,omitempty"`
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	resp := settingsResponse{
		Defaults: settingsDefaults{
			Cores:    s.cfg.Defaults.Cores,
			MemoryMB: s.cfg.Defaults.MemoryMB,
			DiskGB:   s.cfg.Defaults.DiskGB,
		},
		Storages:  s.cfg.Storages,
		Bridges:   s.cfg.Bridges,
		Developer: settingsDeveloper{
			Enabled: s.cfg.Developer.Enabled,
		},
		Service:   settingsService{Port: s.cfg.Service.Port},
		Auth:      settingsAuth{Mode: s.cfg.Auth.Mode},
		Catalog:   settingsCatalog{Refresh: s.cfg.Catalog.Refresh},
		GPU:       settingsGPU{Enabled: s.cfg.GPU.Enabled, Policy: s.cfg.GPU.Policy},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Defaults  *settingsDefaults    `json:"defaults"`
		Storages  *[]string            `json:"storages"`
		Bridges   *[]string            `json:"bridges"`
		Developer *settingsDeveloper   `json:"developer"`
		Catalog   *settingsCatalog     `json:"catalog"`
		GPU       *settingsGPU         `json:"gpu"`
		Auth      *settingsAuthUpdate  `json:"auth"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Defaults != nil {
		if req.Defaults.Cores > 0 {
			s.cfg.Defaults.Cores = req.Defaults.Cores
		}
		if req.Defaults.MemoryMB > 0 {
			s.cfg.Defaults.MemoryMB = req.Defaults.MemoryMB
		}
		if req.Defaults.DiskGB > 0 {
			s.cfg.Defaults.DiskGB = req.Defaults.DiskGB
		}
	}
	if req.Storages != nil {
		if len(*req.Storages) == 0 {
			writeError(w, http.StatusBadRequest, "at least one storage is required")
			return
		}
		s.cfg.Storages = *req.Storages
	}
	if req.Bridges != nil {
		if len(*req.Bridges) == 0 {
			writeError(w, http.StatusBadRequest, "at least one bridge is required")
			return
		}
		s.cfg.Bridges = *req.Bridges
	}
	if req.Developer != nil {
		// When disabling developer mode, undeploy all dev apps (and stacks) from catalog
		if !req.Developer.Enabled && s.cfg.Developer.Enabled && s.devSvc != nil {
			if apps, err := s.devSvc.List(); err == nil {
				for _, meta := range apps {
					if meta.Status == "deployed" {
						if s.catalogSvc != nil {
							s.catalogSvc.RemoveDevApp(meta.ID)
						}
						s.devSvc.SetStatus(meta.ID, "draft")
					}
				}
			}
			if stacks, err := s.devSvc.ListStacks(); err == nil {
				for _, meta := range stacks {
					if meta.Status == "deployed" {
						if s.catalogSvc != nil {
							s.catalogSvc.RemoveDevStack(meta.ID)
						}
						s.devSvc.SetStackStatus(meta.ID, "draft")
					}
				}
			}
		}
		s.cfg.Developer.Enabled = req.Developer.Enabled
	}
	if req.Catalog != nil && req.Catalog.Refresh != "" {
		s.cfg.Catalog.Refresh = req.Catalog.Refresh
	}
	if req.GPU != nil {
		s.cfg.GPU.Enabled = req.GPU.Enabled
		if req.GPU.Policy != "" {
			s.cfg.GPU.Policy = req.GPU.Policy
		}
	}
	if req.Auth != nil {
		mode := req.Auth.Mode
		if mode != config.AuthModeNone && mode != config.AuthModePassword {
			writeError(w, http.StatusBadRequest, "auth mode must be \"none\" or \"password\"")
			return
		}
		if mode == config.AuthModePassword {
			if req.Auth.Password != "" {
				if len(req.Auth.Password) < 8 {
					writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
					return
				}
				hash, err := bcrypt.GenerateFromPassword([]byte(req.Auth.Password), bcrypt.DefaultCost)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "failed to hash password")
					return
				}
				s.cfg.Auth.PasswordHash = string(hash)
			} else if s.cfg.Auth.Mode != config.AuthModePassword {
				// Switching from none→password without providing a password
				writeError(w, http.StatusBadRequest, "password is required when enabling authentication")
				return
			}
			// Generate HMAC secret if not set
			if s.cfg.Auth.HMACSecret == "" {
				secret := make([]byte, 32)
				if _, err := rand.Read(secret); err != nil {
					writeError(w, http.StatusInternalServerError, "failed to generate session secret")
					return
				}
				s.cfg.Auth.HMACSecret = hex.EncodeToString(secret)
			}
		}
		s.cfg.Auth.Mode = mode
	}

	if err := s.cfg.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid settings: %v", err))
		return
	}

	if err := s.cfg.Save(s.configPath); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to save config: %v", err))
		return
	}

	// Re-resolve storage metadata if storages changed
	if req.Storages != nil {
		s.resolveStorageMetas()
	}

	// If auth mode is now password, issue a session cookie so the user stays logged in
	if req.Auth != nil && s.cfg.Auth.Mode == config.AuthModePassword {
		expires := time.Now().Add(sessionMaxAge)
		token := signToken(s.cfg.Auth.HMACSecret, expires)
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    token,
			Path:     "/",
			MaxAge:   int(sessionMaxAge.Seconds()),
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}

	s.handleGetSettings(w, r)
}

type discoverBridgeItem struct {
	Name      string `json:"name"`
	CIDR      string `json:"cidr,omitempty"`
	Gateway   string `json:"gateway,omitempty"`
	Ports     string `json:"ports,omitempty"`
	Comment   string `json:"comment,omitempty"`
	VLANAware bool   `json:"vlan_aware,omitempty"`
	VLANs     string `json:"vlans,omitempty"`
}

// handleDiscoverResources returns available storages and bridges from the system.
func (s *Server) handleDiscoverResources(w http.ResponseWriter, r *http.Request) {
	type storageItem struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	}

	ctx := context.Background()

	var storages []storageItem
	if s.engine != nil {
		if list, err := s.engine.ListStorages(ctx); err == nil {
			for _, si := range list {
				storages = append(storages, storageItem{ID: si.ID, Type: si.Type})
			}
		}
	}
	if storages == nil {
		storages = []storageItem{}
	}

	var bridges []discoverBridgeItem
	if s.engine != nil {
		if list, err := s.engine.ListBridges(ctx); err == nil {
			for _, bi := range list {
				bridges = append(bridges, discoverBridgeItem{
					Name:      bi.Name,
					CIDR:      bi.CIDR,
					Gateway:   bi.Gateway,
					Ports:     bi.Ports,
					Comment:   bi.Comment,
					VLANAware: bi.VLANAware,
					VLANs:     bi.VLANs,
				})
			}
		}
	}
	// Fallback to shell if engine not available
	if bridges == nil {
		bridges = discoverBridgesShell()
	}
	if bridges == nil {
		bridges = []discoverBridgeItem{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"storages": storages,
		"bridges":  bridges,
	})
}

// discoverBridgesShell returns vmbr* bridges via shell command (fallback when engine is nil).
func discoverBridgesShell() []discoverBridgeItem {
	out, err := exec.Command("ip", "-brief", "link", "show").Output()
	if err != nil {
		return nil
	}
	var bridges []discoverBridgeItem
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 1 && strings.HasPrefix(fields[0], "vmbr") {
			bridges = append(bridges, discoverBridgeItem{Name: fields[0]})
		}
	}
	return bridges
}
