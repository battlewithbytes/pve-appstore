package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/engine"
	"github.com/battlewithbytes/pve-appstore/internal/version"
	"gopkg.in/yaml.v3"
)

type storageDefault struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Browsable bool   `json:"browsable"`
	Path      string `json:"path,omitempty"`
}

func (s *Server) handleConfigDefaults(w http.ResponseWriter, r *http.Request) {
	// Build enriched storage details from resolved metadata
	var details []storageDefault
	for _, sm := range s.storageMetas {
		details = append(details, storageDefault{
			ID:        sm.ID,
			Type:      sm.Type,
			Browsable: sm.Browsable,
			Path:      sm.Path,
		})
	}
	if details == nil {
		details = []storageDefault{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"storages":        s.cfg.Storages,
		"storage_details": details,
		"bridges":         s.cfg.Bridges,
		"defaults": map[string]interface{}{
			"cores":     s.cfg.Defaults.Cores,
			"memory_mb": s.cfg.Defaults.MemoryMB,
			"disk_gb":   s.cfg.Defaults.DiskGB,
		},
	})
}

// exportRecipe is a portable install recipe suitable for apply/restore.
type exportRecipe struct {
	AppID          string                     `json:"app_id" yaml:"app_id"`
	Storage        string                     `json:"storage" yaml:"storage"`
	Bridge         string                     `json:"bridge" yaml:"bridge"`
	Cores          int                        `json:"cores" yaml:"cores"`
	MemoryMB       int                        `json:"memory_mb" yaml:"memory_mb"`
	DiskGB         int                        `json:"disk_gb" yaml:"disk_gb"`
	Hostname       string                     `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	OnBoot         *bool                      `json:"onboot,omitempty" yaml:"onboot,omitempty"`
	Unprivileged   *bool                      `json:"unprivileged,omitempty" yaml:"unprivileged,omitempty"`
	Inputs         map[string]string          `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Devices        []engine.DevicePassthrough `json:"devices,omitempty" yaml:"devices,omitempty"`
	EnvVars        map[string]string          `json:"env_vars,omitempty" yaml:"env_vars,omitempty"`
	Ports          []exportPort               `json:"ports,omitempty" yaml:"ports,omitempty"`
	BindMounts     map[string]string          `json:"bind_mounts,omitempty" yaml:"bind_mounts,omitempty"`
	VolumeStorages map[string]string          `json:"volume_storages,omitempty" yaml:"volume_storages,omitempty"`
	ExtraMounts    []engine.ExtraMountRequest `json:"extra_mounts,omitempty" yaml:"extra_mounts,omitempty"`
}

type exportPort struct {
	Key      string `json:"key" yaml:"key"`
	Label    string `json:"label" yaml:"label"`
	Value    int    `json:"value" yaml:"value"`
	Protocol string `json:"protocol" yaml:"protocol"`
}

// exportStackRecipe is a portable stack recipe suitable for apply/restore.
type exportStackRecipe struct {
	Name           string                     `json:"name" yaml:"name"`
	Apps           []exportStackApp           `json:"apps" yaml:"apps"`
	Storage        string                     `json:"storage" yaml:"storage"`
	Bridge         string                     `json:"bridge" yaml:"bridge"`
	Cores          int                        `json:"cores" yaml:"cores"`
	MemoryMB       int                        `json:"memory_mb" yaml:"memory_mb"`
	DiskGB         int                        `json:"disk_gb" yaml:"disk_gb"`
	Hostname       string                     `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	OnBoot         *bool                      `json:"onboot,omitempty" yaml:"onboot,omitempty"`
	Unprivileged   *bool                      `json:"unprivileged,omitempty" yaml:"unprivileged,omitempty"`
	Devices        []engine.DevicePassthrough `json:"devices,omitempty" yaml:"devices,omitempty"`
	EnvVars        map[string]string          `json:"env_vars,omitempty" yaml:"env_vars,omitempty"`
	BindMounts     map[string]string          `json:"bind_mounts,omitempty" yaml:"bind_mounts,omitempty"`
	VolumeStorages map[string]string          `json:"volume_storages,omitempty" yaml:"volume_storages,omitempty"`
	ExtraMounts    []engine.ExtraMountRequest `json:"extra_mounts,omitempty" yaml:"extra_mounts,omitempty"`
}

type exportStackApp struct {
	AppID  string            `json:"app_id" yaml:"app_id"`
	Inputs map[string]string `json:"inputs,omitempty" yaml:"inputs,omitempty"`
}

func buildStackRecipe(stack *engine.Stack) exportStackRecipe {
	recipe := exportStackRecipe{
		Name:     stack.Name,
		Storage:  stack.Storage,
		Bridge:   stack.Bridge,
		Cores:    stack.Cores,
		MemoryMB: stack.MemoryMB,
		DiskGB:   stack.DiskGB,
		Hostname: stack.Hostname,
		Devices:  stack.Devices,
		EnvVars:  stack.EnvVars,
	}
	if stack.OnBoot {
		b := true
		recipe.OnBoot = &b
	}
	if stack.Unprivileged {
		b := true
		recipe.Unprivileged = &b
	}

	// Convert stack apps to portable format
	for _, sa := range stack.Apps {
		recipe.Apps = append(recipe.Apps, exportStackApp{
			AppID:  sa.AppID,
			Inputs: sa.Inputs,
		})
	}

	// Split mount points into bind_mounts, volume_storages, extra_mounts
	for _, mp := range stack.MountPoints {
		switch mp.Type {
		case "bind":
			if mp.HostPath != "" {
				if strings.HasPrefix(mp.Name, "extra-") {
					recipe.ExtraMounts = append(recipe.ExtraMounts, engine.ExtraMountRequest{
						HostPath:  mp.HostPath,
						MountPath: mp.MountPath,
						ReadOnly:  mp.ReadOnly,
					})
				} else {
					if recipe.BindMounts == nil {
						recipe.BindMounts = make(map[string]string)
					}
					recipe.BindMounts[mp.Name] = mp.HostPath
				}
			}
		case "volume":
			if mp.Storage != "" && mp.Storage != stack.Storage {
				if recipe.VolumeStorages == nil {
					recipe.VolumeStorages = make(map[string]string)
				}
				recipe.VolumeStorages[mp.Name] = mp.Storage
			}
		}
	}

	return recipe
}

func (s *Server) buildRecipe(inst *engine.Install) exportRecipe {
	recipe := exportRecipe{
		AppID:    inst.AppID,
		Storage:  inst.Storage,
		Bridge:   inst.Bridge,
		Cores:    inst.Cores,
		MemoryMB: inst.MemoryMB,
		DiskGB:   inst.DiskGB,
		Hostname: inst.Hostname,
		Inputs:   inst.Inputs,
		Devices:  inst.Devices,
		EnvVars:  inst.EnvVars,
	}
	if inst.OnBoot {
		b := true
		recipe.OnBoot = &b
	}
	if inst.Unprivileged {
		b := true
		recipe.Unprivileged = &b
	}

	// Split mount points back into bind_mounts and volume_storages
	for _, mp := range inst.MountPoints {
		switch mp.Type {
		case "bind":
			if mp.HostPath != "" {
				if recipe.BindMounts == nil {
					recipe.BindMounts = make(map[string]string)
				}
				// Extra user-added mounts go to ExtraMounts
				if strings.HasPrefix(mp.Name, "extra-") {
					recipe.ExtraMounts = append(recipe.ExtraMounts, engine.ExtraMountRequest{
						HostPath:  mp.HostPath,
						MountPath: mp.MountPath,
						ReadOnly:  mp.ReadOnly,
					})
				} else {
					recipe.BindMounts[mp.Name] = mp.HostPath
				}
			}
		case "volume":
			if mp.Storage != "" && mp.Storage != inst.Storage {
				if recipe.VolumeStorages == nil {
					recipe.VolumeStorages = make(map[string]string)
				}
				recipe.VolumeStorages[mp.Name] = mp.Storage
			}
		}
	}

	// Extract ports from inputs by checking manifest input specs
	if s.catalogSvc != nil {
		if app, ok := s.catalogSvc.GetApp(inst.AppID); ok {
			for _, input := range app.Inputs {
				val, exists := inst.Inputs[input.Key]
				if !exists {
					continue
				}
				isPort := strings.Contains(input.Key, "port") ||
					(input.Validation != nil && input.Validation.Min != nil && *input.Validation.Min >= 1024 &&
						input.Validation.Max != nil && *input.Validation.Max <= 65535)
				if isPort {
					portVal, _ := strconv.Atoi(val)
					if portVal > 0 {
						recipe.Ports = append(recipe.Ports, exportPort{
							Key:      input.Key,
							Label:    input.Label,
							Value:    portVal,
							Protocol: "tcp",
						})
					}
				}
			}
		}
	}

	return recipe
}

func (s *Server) handleConfigExport(w http.ResponseWriter, r *http.Request) {
	if s.engineConfigSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	installs, err := s.engineConfigSvc.ListInstalls()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if installs == nil {
		installs = []*engine.Install{}
	}

	recipes := make([]exportRecipe, 0, len(installs))
	for _, inst := range installs {
		recipes = append(recipes, s.buildRecipe(inst))
	}

	// Include stacks
	stacks, _ := s.engineConfigSvc.ListStacks()
	stackRecipes := make([]exportStackRecipe, 0)
	for _, st := range stacks {
		if st.Status == "uninstalled" {
			continue
		}
		stackRecipes = append(stackRecipes, buildStackRecipe(st))
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"exported_at": time.Now().Format(time.RFC3339),
		"node":        s.cfg.NodeName,
		"version":     version.Version,
		"recipes":     recipes,
		"stacks":      stackRecipes,
		"installs":    installs,
	})
}

func (s *Server) handleConfigApply(w http.ResponseWriter, r *http.Request) {
	if s.engineConfigSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}
	if s.catalogSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "catalog service not available")
		return
	}

	var req struct {
		Recipes []engine.InstallRequest     `json:"recipes"`
		Stacks  []engine.StackCreateRequest `json:"stacks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Recipes) == 0 && len(req.Stacks) == 0 {
		writeError(w, http.StatusBadRequest, "no recipes or stacks provided")
		return
	}

	// Validate all app IDs in recipes exist in catalog
	for _, recipe := range req.Recipes {
		if _, ok := s.catalogSvc.GetApp(recipe.AppID); !ok {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("app %q not found in catalog", recipe.AppID))
			return
		}
	}

	// Validate all app IDs in stacks exist in catalog
	for _, stack := range req.Stacks {
		for _, app := range stack.Apps {
			if _, ok := s.catalogSvc.GetApp(app.AppID); !ok {
				writeError(w, http.StatusBadRequest, fmt.Sprintf("stack %q: app %q not found in catalog", stack.Name, app.AppID))
				return
			}
		}
	}

	type jobResult struct {
		AppID string `json:"app_id"`
		JobID string `json:"job_id"`
	}
	var jobs []jobResult
	for _, recipe := range req.Recipes {
		job, err := s.engineConfigSvc.StartInstall(recipe)
		if err != nil {
			if _, ok := err.(*engine.ErrDuplicate); ok {
				continue // skip already-installed apps during bulk apply
			}
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to start install for %s: %v", recipe.AppID, err))
			return
		}
		jobs = append(jobs, jobResult{AppID: recipe.AppID, JobID: job.ID})
	}

	type stackJobResult struct {
		Name  string `json:"name"`
		JobID string `json:"job_id"`
	}
	var stackJobs []stackJobResult
	for _, stack := range req.Stacks {
		job, err := s.engineConfigSvc.StartStack(stack)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to start stack %s: %v", stack.Name, err))
			return
		}
		stackJobs = append(stackJobs, stackJobResult{Name: stack.Name, JobID: job.ID})
	}

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"jobs":       jobs,
		"stack_jobs": stackJobs,
	})
}

func (s *Server) handleConfigApplyPreview(w http.ResponseWriter, r *http.Request) {
	if s.catalogSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "catalog service not available")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read request body")
		return
	}
	if len(body) == 0 {
		writeError(w, http.StatusBadRequest, "empty request body")
		return
	}

	type parsedConfig struct {
		Recipes []exportRecipe      `json:"recipes" yaml:"recipes"`
		Stacks  []exportStackRecipe `json:"stacks" yaml:"stacks"`
	}

	var parsed parsedConfig

	// Try JSON first, then YAML
	if err := json.Unmarshal(body, &parsed); err != nil {
		if err2 := yaml.Unmarshal(body, &parsed); err2 != nil {
			// Try as bare array of recipes
			var recipes []exportRecipe
			if err3 := json.Unmarshal(body, &recipes); err3 == nil {
				parsed.Recipes = recipes
			} else if err4 := yaml.Unmarshal(body, &recipes); err4 == nil {
				parsed.Recipes = recipes
			} else {
				writeError(w, http.StatusBadRequest, "could not parse as JSON or YAML")
				return
			}
		}
	}

	// Validate app IDs exist in catalog
	var errors []string
	for i, r := range parsed.Recipes {
		if r.AppID == "" {
			errors = append(errors, fmt.Sprintf("recipe[%d]: missing app_id", i))
			continue
		}
		if _, ok := s.catalogSvc.GetApp(r.AppID); !ok {
			errors = append(errors, fmt.Sprintf("recipe[%d]: app %q not found in catalog", i, r.AppID))
		}
	}
	for i, st := range parsed.Stacks {
		if st.Name == "" {
			errors = append(errors, fmt.Sprintf("stack[%d]: missing name", i))
		}
		for j, app := range st.Apps {
			if app.AppID == "" {
				errors = append(errors, fmt.Sprintf("stack[%d].apps[%d]: missing app_id", i, j))
				continue
			}
			if _, ok := s.catalogSvc.GetApp(app.AppID); !ok {
				errors = append(errors, fmt.Sprintf("stack[%d].apps[%d]: app %q not found in catalog", i, j, app.AppID))
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"recipes": parsed.Recipes,
		"stacks":  parsed.Stacks,
		"errors":  errors,
	})
}

func (s *Server) handleConfigExportDownload(w http.ResponseWriter, r *http.Request) {
	if s.engineConfigSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	installs, err := s.engineConfigSvc.ListInstalls()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if installs == nil {
		installs = []*engine.Install{}
	}

	recipes := make([]exportRecipe, 0, len(installs))
	for _, inst := range installs {
		recipes = append(recipes, s.buildRecipe(inst))
	}

	// Include stacks
	stacks, _ := s.engineConfigSvc.ListStacks()
	stackRecipes := make([]exportStackRecipe, 0)
	for _, st := range stacks {
		if st.Status == "uninstalled" {
			continue
		}
		stackRecipes = append(stackRecipes, buildStackRecipe(st))
	}

	data := map[string]interface{}{
		"exported_at": time.Now().Format(time.RFC3339),
		"node":        s.cfg.NodeName,
		"version":     version.Version,
		"recipes":     recipes,
		"stacks":      stackRecipes,
	}

	yamlData, err := yaml.Marshal(data)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to marshal YAML")
		return
	}

	filename := fmt.Sprintf("pve-appstore-backup-%s.yml", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Write(yamlData)
}
