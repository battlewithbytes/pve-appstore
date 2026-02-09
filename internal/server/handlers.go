package server

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
	"github.com/battlewithbytes/pve-appstore/internal/engine"
	"github.com/battlewithbytes/pve-appstore/internal/version"
	"gopkg.in/yaml.v3"
)

//go:embed assets/default-icon.png
var defaultIconPNG []byte

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "ok",
		"version":   version.Version,
		"node":      s.cfg.NodeName,
		"app_count": s.catalog.AppCount(),
	})
}

func (s *Server) handleListApps(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	category := r.URL.Query().Get("category")
	sortBy := r.URL.Query().Get("sort")

	var apps = s.catalog.List()

	// Apply search filter
	if query != "" {
		apps = s.catalog.Search(query)
	}

	// Apply category filter
	if category != "" {
		filtered := make([]*appResponse, 0)
		for _, app := range apps {
			for _, c := range app.Categories {
				if strings.EqualFold(c, category) {
					filtered = append(filtered, toAppResponse(app))
					break
				}
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"apps":  filtered,
			"total": len(filtered),
		})
		return
	}

	// Convert to response format
	resp := make([]*appResponse, 0, len(apps))
	for _, app := range apps {
		resp = append(resp, toAppResponse(app))
	}

	// Sort
	switch sortBy {
	case "name":
		sort.Slice(resp, func(i, j int) bool {
			return resp[i].Name < resp[j].Name
		})
	default: // "updated" or default — sort by name for now
		sort.Slice(resp, func(i, j int) bool {
			return resp[i].Name < resp[j].Name
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"apps":  resp,
		"total": len(resp),
	})
}

func (s *Server) handleGetApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, ok := s.catalog.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("app %q not found", id))
		return
	}

	writeJSON(w, http.StatusOK, app)
}

func (s *Server) handleGetAppReadme(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, ok := s.catalog.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("app %q not found", id))
		return
	}

	if app.ReadmePath == "" {
		writeError(w, http.StatusNotFound, "no readme available")
		return
	}

	data, err := os.ReadFile(app.ReadmePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read readme")
		return
	}

	w.Header().Set("Content-Type", "text/markdown")
	w.Write(data)
}

func (s *Server) handleGetAppIcon(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, ok := s.catalog.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("app %q not found", id))
		return
	}

	if app.IconPath != "" {
		http.ServeFile(w, r, app.IconPath)
		return
	}

	// Serve embedded default icon
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(defaultIconPNG)
}

func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	cats := s.catalog.Categories()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"categories": cats,
	})
}

func (s *Server) handleCatalogRefresh(w http.ResponseWriter, r *http.Request) {
	if err := s.catalog.Refresh(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("refresh failed: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "refreshed",
		"app_count": s.catalog.AppCount(),
	})
}

// appResponse is a lightweight summary for the app list.
type appResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Categories  []string `json:"categories"`
	Tags        []string `json:"tags"`
	HasIcon     bool     `json:"has_icon"`
	Official    bool     `json:"official"`
	GPURequired bool     `json:"gpu_required"`
	GPUSupport  []string `json:"gpu_support,omitempty"`
}

func toAppResponse(app *catalog.AppManifest) *appResponse {
	return &appResponse{
		ID:          app.ID,
		Name:        app.Name,
		Description: app.Description,
		Version:     app.Version,
		Categories:  app.Categories,
		Tags:        app.Tags,
		HasIcon:     true, // default icon served when app has none
		Official:    app.Official,
		GPURequired: app.GPU.Required,
		GPUSupport:  app.GPU.Supported,
	}
}

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

// --- Install/Jobs handlers ---

func (s *Server) handleInstallApp(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var req engine.InstallRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	req.AppID = appID

	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "install engine not available")
		return
	}

	job, err := s.engine.StartInstall(req)
	if err != nil {
		if dupErr, ok := err.(*engine.ErrDuplicate); ok {
			resp := map[string]string{"error": dupErr.Message}
			if dupErr.InstallID != "" {
				resp["existing_install_id"] = dupErr.InstallID
			}
			if dupErr.JobID != "" {
				resp["existing_job_id"] = dupErr.JobID
			}
			writeJSON(w, http.StatusConflict, resp)
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleAppStatus(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	resp := map[string]interface{}{
		"installed":  false,
		"job_active": false,
	}

	if s.engine != nil {
		if inst, exists := s.engine.HasActiveInstallForApp(appID); exists {
			resp["installed"] = true
			resp["install_id"] = inst.ID
			resp["install_status"] = inst.Status
			resp["ctid"] = inst.CTID
		}
		if job, exists := s.engine.HasActiveJobForApp(appID); exists {
			resp["job_active"] = true
			resp["job_id"] = job.ID
			resp["job_state"] = job.State
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	if err := s.engine.CancelJob(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled", "job_id": id})
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"jobs": []interface{}{}, "total": 0})
		return
	}

	jobs, err := s.engine.ListJobs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if jobs == nil {
		jobs = []*engine.Job{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"jobs":  jobs,
		"total": len(jobs),
	})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	job, err := s.engine.GetJob(id)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("job %q not found", id))
		return
	}

	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleGetJobLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	afterStr := r.URL.Query().Get("after")

	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	afterID := 0
	if afterStr != "" {
		afterID, _ = strconv.Atoi(afterStr)
	}

	logs, lastID, err := s.engine.GetLogsSince(id, afterID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if logs == nil {
		logs = []*engine.LogEntry{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"logs":    logs,
		"last_id": lastID,
	})
}

func (s *Server) handleListInstalls(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"installs": []interface{}{}, "total": 0})
		return
	}

	installs, err := s.engine.ListInstallsEnriched()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if installs == nil {
		installs = []*engine.InstallListItem{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"installs": installs,
		"total":    len(installs),
	})
}

func (s *Server) handleStartContainer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}
	if err := s.engine.StartContainer(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started", "install_id": id})
}

func (s *Server) handleStopContainer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}
	if err := s.engine.StopContainer(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped", "install_id": id})
}

func (s *Server) handleRestartContainer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}
	if err := s.engine.RestartContainer(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarted", "install_id": id})
}

func (s *Server) handleGetInstall(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	detail, err := s.engine.GetInstallDetail(id)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("install %q not found", id))
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleUninstall(w http.ResponseWriter, r *http.Request) {
	installID := r.PathValue("id")

	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	// Parse optional keep_volumes from body (defaults to true if app has volumes)
	var req struct {
		KeepVolumes *bool `json:"keep_volumes"`
	}
	if r.Body != nil && r.ContentLength > 0 {
		json.NewDecoder(r.Body).Decode(&req)
	}

	keepVolumes := false
	if req.KeepVolumes != nil {
		keepVolumes = *req.KeepVolumes
	} else {
		// Default: keep volumes if the install has mount points
		inst, err := s.engine.GetInstall(installID)
		if err == nil && len(inst.MountPoints) > 0 {
			keepVolumes = true
		}
	}

	job, err := s.engine.Uninstall(installID, keepVolumes)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, job)
}

// exportRecipe is a portable install recipe suitable for apply/restore.
type exportRecipe struct {
	AppID          string                      `json:"app_id" yaml:"app_id"`
	Storage        string                      `json:"storage" yaml:"storage"`
	Bridge         string                      `json:"bridge" yaml:"bridge"`
	Cores          int                         `json:"cores" yaml:"cores"`
	MemoryMB       int                         `json:"memory_mb" yaml:"memory_mb"`
	DiskGB         int                         `json:"disk_gb" yaml:"disk_gb"`
	Hostname       string                      `json:"hostname,omitempty" yaml:"hostname,omitempty"`
	OnBoot         *bool                       `json:"onboot,omitempty" yaml:"onboot,omitempty"`
	Unprivileged   *bool                       `json:"unprivileged,omitempty" yaml:"unprivileged,omitempty"`
	Inputs         map[string]string           `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Devices        []engine.DevicePassthrough  `json:"devices,omitempty" yaml:"devices,omitempty"`
	EnvVars        map[string]string           `json:"env_vars,omitempty" yaml:"env_vars,omitempty"`
	Ports          []exportPort                `json:"ports,omitempty" yaml:"ports,omitempty"`
	BindMounts     map[string]string           `json:"bind_mounts,omitempty" yaml:"bind_mounts,omitempty"`
	VolumeStorages map[string]string           `json:"volume_storages,omitempty" yaml:"volume_storages,omitempty"`
	ExtraMounts    []engine.ExtraMountRequest  `json:"extra_mounts,omitempty" yaml:"extra_mounts,omitempty"`
}

type exportPort struct {
	Key      string `json:"key" yaml:"key"`
	Label    string `json:"label" yaml:"label"`
	Value    int    `json:"value" yaml:"value"`
	Protocol string `json:"protocol" yaml:"protocol"`
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
	if app, ok := s.catalog.Get(inst.AppID); ok {
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

	return recipe
}

func (s *Server) handleConfigExport(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	installs, err := s.engine.ListInstalls()
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

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"exported_at": time.Now().Format(time.RFC3339),
		"node":        s.cfg.NodeName,
		"recipes":     recipes,
		"installs":    installs,
	})
}

func (s *Server) handleConfigApply(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	var req struct {
		Recipes []engine.InstallRequest `json:"recipes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Recipes) == 0 {
		writeError(w, http.StatusBadRequest, "no recipes provided")
		return
	}

	// Validate all app IDs exist in catalog
	for _, recipe := range req.Recipes {
		if _, ok := s.catalog.Get(recipe.AppID); !ok {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("app %q not found in catalog", recipe.AppID))
			return
		}
	}

	type jobResult struct {
		AppID string `json:"app_id"`
		JobID string `json:"job_id"`
	}
	var jobs []jobResult
	for _, recipe := range req.Recipes {
		job, err := s.engine.StartInstall(recipe)
		if err != nil {
			if _, ok := err.(*engine.ErrDuplicate); ok {
				continue // skip already-installed apps during bulk apply
			}
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to start install for %s: %v", recipe.AppID, err))
			return
		}
		jobs = append(jobs, jobResult{AppID: recipe.AppID, JobID: job.ID})
	}

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"jobs": jobs,
	})
}

func (s *Server) handleConfigExportDownload(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	installs, err := s.engine.ListInstalls()
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

	data := map[string]interface{}{
		"exported_at": time.Now().Format(time.RFC3339),
		"node":        s.cfg.NodeName,
		"recipes":     recipes,
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

func (s *Server) handleReinstall(w http.ResponseWriter, r *http.Request) {
	installID := r.PathValue("id")

	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	var req engine.ReinstallRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	job, err := s.engine.Reinstall(installID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleBrowseStorages(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"storages": s.cfg.Storages,
	})
}

func (s *Server) handleBrowseMounts(w http.ResponseWriter, r *http.Request) {
	type mountInfo struct {
		Path   string `json:"path"`
		FsType string `json:"fs_type"`
		Device string `json:"device"`
	}

	var mounts []mountInfo
	for _, sm := range s.storageMetas {
		if sm.Browsable && sm.Path != "" {
			mounts = append(mounts, mountInfo{
				Path:   sm.Path,
				FsType: sm.Type,
				Device: sm.ID,
			})
		}
	}
	if mounts == nil {
		mounts = []mountInfo{}
	}
	sort.Slice(mounts, func(i, j int) bool { return mounts[i].Path < mounts[j].Path })

	writeJSON(w, http.StatusOK, map[string]interface{}{"mounts": mounts})
}

func (s *Server) handleBrowseMkdir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}

	cleaned := filepath.Clean(req.Path)
	if !s.isPathAllowed(cleaned) {
		writeError(w, http.StatusForbidden, "path is not under a configured storage")
		return
	}

	if err := os.MkdirAll(cleaned, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("mkdir failed: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"path": cleaned, "created": true})
}

func (s *Server) handleBrowsePaths(w http.ResponseWriter, r *http.Request) {
	root := r.URL.Query().Get("path")
	if root == "" {
		root = "/"
	}
	root = filepath.Clean(root)

	if !s.isPathAllowed(root) {
		writeError(w, http.StatusForbidden, "path is not under a configured storage")
		return
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("cannot read %s: %v", root, err))
		return
	}

	type dirEntry struct {
		Name  string `json:"name"`
		Path  string `json:"path"`
		IsDir bool   `json:"is_dir"`
	}
	dirs := []dirEntry{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dirs = append(dirs, dirEntry{
			Name:  e.Name(),
			Path:  filepath.Join(root, e.Name()),
			IsDir: e.IsDir(),
		})
	}
	sort.Slice(dirs, func(i, j int) bool {
		if dirs[i].IsDir != dirs[j].IsDir {
			return dirs[i].IsDir
		}
		return dirs[i].Name < dirs[j].Name
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":    root,
		"entries": dirs,
	})
}

// --- Stack handlers ---

func (s *Server) handleCreateStack(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	var req engine.StackCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	job, err := s.engine.StartStack(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleListStacks(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"stacks": []interface{}{}, "total": 0})
		return
	}

	stacks, err := s.engine.ListStacksEnriched()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if stacks == nil {
		stacks = []*engine.StackListItem{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"stacks": stacks,
		"total":  len(stacks),
	})
}

func (s *Server) handleGetStack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	detail, err := s.engine.GetStackDetail(id)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("stack %q not found", id))
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleStartStack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}
	if err := s.engine.StartStackContainer(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started", "stack_id": id})
}

func (s *Server) handleStopStack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}
	if err := s.engine.StopStackContainer(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped", "stack_id": id})
}

func (s *Server) handleRestartStack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}
	if err := s.engine.RestartStackContainer(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarted", "stack_id": id})
}

func (s *Server) handleUninstallStack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	job, err := s.engine.UninstallStack(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleValidateStack(w http.ResponseWriter, r *http.Request) {
	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	var req engine.StackCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result := s.engine.ValidateStack(req)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleStackTerminal(w http.ResponseWriter, r *http.Request) {
	stackID := r.PathValue("id")

	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	stack, err := s.engine.GetStack(stackID)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("stack %q not found", stackID))
		return
	}

	// Rewrite PathValue to use CTID and delegate to common terminal handler
	s.handleTerminalForCTID(w, r, stack.CTID)
}

func (s *Server) handleStackJournalLogs(w http.ResponseWriter, r *http.Request) {
	stackID := r.PathValue("id")

	if s.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	stack, err := s.engine.GetStack(stackID)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("stack %q not found", stackID))
		return
	}

	s.handleJournalLogsForCTID(w, r, stack.CTID)
}
