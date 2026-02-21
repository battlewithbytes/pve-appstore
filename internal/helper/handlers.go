package helper

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const pctBin = "/usr/sbin/pct"

// --- POST /v1/pct/exec ---

type pctExecRequest struct {
	CTID    int      `json:"ctid"`
	Command []string `json:"command"`
}

type pctExecResponse struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
}

func (s *Server) handlePctExec(w http.ResponseWriter, r *http.Request) {
	var req pctExecRequest
	if err := decodeBody(r, &req); err != nil {
		writeHelperError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.validateCTID(req.CTID); err != nil {
		writeHelperError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := validateCommand(req.Command); err != nil {
		writeHelperError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Acquire exec semaphore
	select {
	case s.execSem <- struct{}{}:
		defer func() { <-s.execSem }()
	default:
		writeHelperError(w, http.StatusTooManyRequests, "too many concurrent exec operations")
		return
	}

	args := append([]string{"exec", strconv.Itoa(req.CTID), "--"}, req.Command...)
	cmd := exec.Command(pctBin, args...)
	out, err := cmd.CombinedOutput()

	resp := pctExecResponse{
		Output:   string(out),
		ExitCode: 0,
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			resp.ExitCode = exitErr.ExitCode()
		} else {
			writeHelperError(w, http.StatusInternalServerError, fmt.Sprintf("pct exec: %v", err))
			return
		}
	}
	writeHelperJSON(w, http.StatusOK, resp)
}

// --- POST /v1/pct/push ---

type pctPushRequest struct {
	CTID  int    `json:"ctid"`
	Src   string `json:"src"`
	Dst   string `json:"dst"`
	Perms string `json:"perms"`
}

func (s *Server) handlePctPush(w http.ResponseWriter, r *http.Request) {
	var req pctPushRequest
	if err := decodeBody(r, &req); err != nil {
		writeHelperError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.validateCTID(req.CTID); err != nil {
		writeHelperError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := validatePushSrc(req.Src); err != nil {
		writeHelperError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := validatePerms(req.Perms); err != nil {
		writeHelperError(w, http.StatusBadRequest, err.Error())
		return
	}

	args := []string{"push", strconv.Itoa(req.CTID), req.Src, req.Dst}
	if req.Perms != "" {
		args = append(args, "--perms", req.Perms)
	}
	cmd := exec.Command(pctBin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		writeHelperError(w, http.StatusInternalServerError, fmt.Sprintf("pct push: %s: %v", strings.TrimSpace(string(out)), err))
		return
	}
	outStr := strings.TrimSpace(string(out))
	if strings.Contains(outStr, "failed to") {
		writeHelperError(w, http.StatusInternalServerError, fmt.Sprintf("pct push: %s", outStr))
		return
	}
	writeHelperJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- POST /v1/pct/set ---

type pctSetRequest struct {
	CTID   int    `json:"ctid"`
	Option string `json:"option"`
	Value  string `json:"value"`
}

func (s *Server) handlePctSet(w http.ResponseWriter, r *http.Request) {
	var req pctSetRequest
	if err := decodeBody(r, &req); err != nil {
		writeHelperError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.validateCTID(req.CTID); err != nil {
		writeHelperError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := validatePctSetOption(req.Option); err != nil {
		writeHelperError(w, http.StatusForbidden, err.Error())
		return
	}

	// Validate value based on option type
	if devOptionRe.MatchString(req.Option) {
		if err := validateDevValue(req.Value); err != nil {
			writeHelperError(w, http.StatusBadRequest, err.Error())
			return
		}
	} else if mpOptionRe.MatchString(req.Option) {
		if err := s.validateMpValue(req.Value); err != nil {
			writeHelperError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	// Acquire per-CTID lock for config-modifying operations
	mu := s.getCTIDLock(req.CTID)
	mu.Lock()
	defer mu.Unlock()

	cmd := exec.Command(pctBin, "set", strconv.Itoa(req.CTID), req.Option, req.Value)
	out, err := cmd.CombinedOutput()
	if err != nil {
		writeHelperError(w, http.StatusInternalServerError, fmt.Sprintf("pct set: %s: %v", strings.TrimSpace(string(out)), err))
		return
	}
	writeHelperJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- POST /v1/conf/append ---

type confAppendRequest struct {
	CTID  int      `json:"ctid"`
	Lines []string `json:"lines"`
}

func (s *Server) handleConfAppend(w http.ResponseWriter, r *http.Request) {
	var req confAppendRequest
	if err := decodeBody(r, &req); err != nil {
		writeHelperError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.validateCTID(req.CTID); err != nil {
		writeHelperError(w, http.StatusForbidden, err.Error())
		return
	}
	if len(req.Lines) == 0 {
		writeHelperError(w, http.StatusBadRequest, "no config lines provided")
		return
	}

	// Validate each line (S4)
	for _, line := range req.Lines {
		if err := s.validateConfLine(line); err != nil {
			writeHelperError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	// Acquire per-CTID lock
	mu := s.getCTIDLock(req.CTID)
	mu.Lock()
	defer mu.Unlock()

	// Construct path server-side (S3 defense-in-depth)
	confPath := fmt.Sprintf("/etc/pve/lxc/%d.conf", req.CTID)

	// Open file directly (helper runs as root, no need for tee)
	f, err := os.OpenFile(confPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		writeHelperError(w, http.StatusInternalServerError, fmt.Sprintf("opening config: %v", err))
		return
	}
	defer f.Close()

	content := strings.Join(req.Lines, "\n") + "\n"
	if _, err := f.WriteString(content); err != nil {
		writeHelperError(w, http.StatusInternalServerError, fmt.Sprintf("writing config: %v", err))
		return
	}

	writeHelperJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- POST /v1/fs/mkdir ---

type fsMkdirRequest struct {
	Path string `json:"path"`
}

func (s *Server) handleFsMkdir(w http.ResponseWriter, r *http.Request) {
	var req fsMkdirRequest
	if err := decodeBody(r, &req); err != nil {
		writeHelperError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.validatePath(req.Path); err != nil {
		writeHelperError(w, http.StatusForbidden, err.Error())
		return
	}

	if err := os.MkdirAll(req.Path, 0755); err != nil {
		writeHelperError(w, http.StatusInternalServerError, fmt.Sprintf("mkdir: %v", err))
		return
	}
	writeHelperJSON(w, http.StatusOK, map[string]string{"status": "ok", "path": req.Path})
}

// --- POST /v1/fs/chown ---

type fsChownRequest struct {
	Path      string `json:"path"`
	UID       int    `json:"uid"`
	GID       int    `json:"gid"`
	Recursive bool   `json:"recursive"`
}

func (s *Server) handleFsChown(w http.ResponseWriter, r *http.Request) {
	var req fsChownRequest
	if err := decodeBody(r, &req); err != nil {
		writeHelperError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.validatePath(req.Path); err != nil {
		writeHelperError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := validateChownOwnership(req.UID, req.GID); err != nil {
		writeHelperError(w, http.StatusForbidden, err.Error())
		return
	}

	if req.Recursive {
		// Use filepath.Walk for recursive chown (Go stdlib, no shell)
		err := filepath.Walk(req.Path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // skip errors (e.g. permission denied on individual files)
			}
			return os.Chown(path, req.UID, req.GID)
		})
		if err != nil {
			writeHelperError(w, http.StatusInternalServerError, fmt.Sprintf("chown: %v", err))
			return
		}
	} else {
		if err := os.Chown(req.Path, req.UID, req.GID); err != nil {
			writeHelperError(w, http.StatusInternalServerError, fmt.Sprintf("chown: %v", err))
			return
		}
	}
	writeHelperJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- POST /v1/fs/rm ---

type fsRmRequest struct {
	Path string `json:"path"`
}

func (s *Server) handleFsRm(w http.ResponseWriter, r *http.Request) {
	var req fsRmRequest
	if err := decodeBody(r, &req); err != nil {
		writeHelperError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.validateRmPath(req.Path); err != nil {
		writeHelperError(w, http.StatusForbidden, err.Error())
		return
	}

	if err := os.RemoveAll(req.Path); err != nil {
		writeHelperError(w, http.StatusInternalServerError, fmt.Sprintf("rm: %v", err))
		return
	}
	writeHelperJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- POST /v1/update ---

func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if err := validateUpdatePath(); err != nil {
		writeHelperError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Only one update at a time
	if !s.updateLock.TryLock() {
		writeHelperError(w, http.StatusConflict, "an update is already in progress")
		return
	}

	// Run update.sh detached so it survives the service restart
	updateScript := "/opt/pve-appstore/update.sh"
	cmd := exec.Command("setsid", updateScript, hardcodedUpdateBinaryPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		s.updateLock.Unlock()
		writeHelperError(w, http.StatusInternalServerError, fmt.Sprintf("starting update: %v", err))
		return
	}
	// Don't wait — the script restarts services; unlock happens when helper restarts
	writeHelperJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
