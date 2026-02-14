package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/battlewithbytes/pve-appstore/internal/engine"
)

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

	if s.engineInstallSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "install engine not available")
		return
	}

	job, err := s.engineInstallSvc.StartInstall(req)
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

	if s.engineInstallSvc != nil {
		if inst, exists := s.engineInstallSvc.HasActiveInstallForApp(appID); exists {
			resp["installed"] = true
			resp["install_id"] = inst.ID
			resp["install_status"] = inst.Status
			resp["ctid"] = inst.CTID
			resp["app_source"] = inst.AppSource
		}
		if job, exists := s.engineInstallSvc.HasActiveJobForApp(appID); exists {
			resp["job_active"] = true
			resp["job_id"] = job.ID
			resp["job_state"] = job.State
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if s.engineInstallSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	if err := s.engineInstallSvc.CancelJob(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled", "job_id": id})
}

func (s *Server) handleClearJobs(w http.ResponseWriter, r *http.Request) {
	if s.engineInstallSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}
	n, err := s.engineInstallSvc.ClearJobs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"deleted": n})
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	if s.engineInstallSvc == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"jobs": []interface{}{}, "total": 0})
		return
	}

	jobs, err := s.engineInstallSvc.ListJobs()
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

	if s.engineInstallSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	job, err := s.engineInstallSvc.GetJob(id)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("job %q not found", id))
		return
	}

	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleGetJobLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	afterStr := r.URL.Query().Get("after")

	if s.engineInstallSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	afterID := 0
	if afterStr != "" {
		afterID, _ = strconv.Atoi(afterStr)
	}

	logs, lastID, err := s.engineInstallSvc.GetLogsSince(id, afterID)
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
	if s.engineInstallSvc == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"installs": []interface{}{}, "total": 0})
		return
	}

	installs, err := s.engineInstallSvc.ListInstallsEnriched()
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
	if s.engineInstallSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}
	if err := s.engineInstallSvc.StartContainer(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started", "install_id": id})
}

func (s *Server) handleStopContainer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.engineInstallSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}
	if err := s.engineInstallSvc.StopContainer(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped", "install_id": id})
}

func (s *Server) handleRestartContainer(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.engineInstallSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}
	if err := s.engineInstallSvc.RestartContainer(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarted", "install_id": id})
}

func (s *Server) handleGetInstall(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if s.engineInstallSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	detail, err := s.engineInstallSvc.GetInstallDetail(id)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("install %q not found", id))
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleUninstall(w http.ResponseWriter, r *http.Request) {
	installID := r.PathValue("id")

	if s.engineInstallSvc == nil {
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
		inst, err := s.engineInstallSvc.GetInstall(installID)
		if err == nil && len(inst.MountPoints) > 0 {
			keepVolumes = true
		}
	}

	job, err := s.engineInstallSvc.Uninstall(installID, keepVolumes)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handlePurgeInstall(w http.ResponseWriter, r *http.Request) {
	installID := r.PathValue("id")

	if s.engineInstallSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	if err := s.engineInstallSvc.PurgeInstall(installID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "purged"})
}

func (s *Server) handleReinstall(w http.ResponseWriter, r *http.Request) {
	installID := r.PathValue("id")

	if s.engineInstallSvc == nil {
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

	job, err := s.engineInstallSvc.Reinstall(installID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	installID := r.PathValue("id")

	if s.engineInstallSvc == nil {
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

	job, err := s.engineInstallSvc.Update(installID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleEditInstall(w http.ResponseWriter, r *http.Request) {
	installID := r.PathValue("id")

	if s.engineInstallSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	var req engine.EditRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	job, err := s.engineInstallSvc.EditInstall(installID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleReconfigure(w http.ResponseWriter, r *http.Request) {
	installID := r.PathValue("id")

	if s.engineInstallSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	var req engine.ReconfigureRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	inst, err := s.engineInstallSvc.ReconfigureInstall(installID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, inst)
}
