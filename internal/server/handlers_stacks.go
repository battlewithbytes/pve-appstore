package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/battlewithbytes/pve-appstore/internal/engine"
)

// --- Stack handlers ---

func (s *Server) handleCreateStack(w http.ResponseWriter, r *http.Request) {
	if s.engineStackSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	var req engine.StackCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	job, err := s.engineStackSvc.StartStack(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleListStacks(w http.ResponseWriter, r *http.Request) {
	if s.engineStackSvc == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"stacks": []interface{}{}, "total": 0})
		return
	}

	stacks, err := s.engineStackSvc.ListStacksEnriched()
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
	if s.engineStackSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	detail, err := s.engineStackSvc.GetStackDetail(id)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("stack %q not found", id))
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleStartStack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.engineStackSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}
	if err := s.engineStackSvc.StartStackContainer(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started", "stack_id": id})
}

func (s *Server) handleStopStack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.engineStackSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}
	if err := s.engineStackSvc.StopStackContainer(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped", "stack_id": id})
}

func (s *Server) handleRestartStack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.engineStackSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}
	if err := s.engineStackSvc.RestartStackContainer(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarted", "stack_id": id})
}

func (s *Server) handleUninstallStack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.engineStackSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	job, err := s.engineStackSvc.UninstallStack(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleEditStack(w http.ResponseWriter, r *http.Request) {
	stackID := r.PathValue("id")

	if s.engineStackSvc == nil {
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

	job, err := s.engineStackSvc.EditStack(stackID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleValidateStack(w http.ResponseWriter, r *http.Request) {
	if s.engineStackSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	var req engine.StackCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result := s.engineStackSvc.ValidateStack(req)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleStackTerminal(w http.ResponseWriter, r *http.Request) {
	stackID := r.PathValue("id")

	if s.engineStackSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	stack, err := s.engineStackSvc.GetStack(stackID)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("stack %q not found", stackID))
		return
	}

	// Rewrite PathValue to use CTID and delegate to common terminal handler
	s.handleTerminalForCTID(w, r, stack.CTID)
}

func (s *Server) handleStackJournalLogs(w http.ResponseWriter, r *http.Request) {
	stackID := r.PathValue("id")

	if s.engineStackSvc == nil {
		writeError(w, http.StatusServiceUnavailable, "engine not available")
		return
	}

	stack, err := s.engineStackSvc.GetStack(stackID)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("stack %q not found", stackID))
		return
	}

	s.handleJournalLogsForCTID(w, r, stack.CTID)
}
