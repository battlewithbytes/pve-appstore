package server

import (
	"encoding/json"
	"fmt"
	"net/http"
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
		Defaults  *settingsDefaults  `json:"defaults"`
		Developer *settingsDeveloper `json:"developer"`
		Catalog   *settingsCatalog   `json:"catalog"`
		GPU       *settingsGPU       `json:"gpu"`
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
	if req.Developer != nil {
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

	if err := s.cfg.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid settings: %v", err))
		return
	}

	if err := s.cfg.Save(s.configPath); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to save config: %v", err))
		return
	}

	s.handleGetSettings(w, r)
}

