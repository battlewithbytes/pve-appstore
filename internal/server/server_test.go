package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
	"github.com/battlewithbytes/pve-appstore/internal/config"
	"github.com/battlewithbytes/pve-appstore/internal/devmode"
	"github.com/battlewithbytes/pve-appstore/internal/engine"
)

// --- Health ---

func TestHealthEndpoint(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/health", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := decodeJSON(t, w)
	if body["status"] != "ok" {
		t.Errorf("status = %v, want %q", body["status"], "ok")
	}
	if body["node"] != "testnode" {
		t.Errorf("node = %v, want %q", body["node"], "testnode")
	}
	count := body["app_count"].(float64)
	if count != 12 {
		t.Errorf("app_count = %v, want 12", count)
	}
}

// --- Apps ---

func TestListAppsEndpoint(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/apps", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := decodeJSON(t, w)
	total := body["total"].(float64)
	if total != 12 {
		t.Errorf("total = %v, want 12", total)
	}
	apps := body["apps"].([]interface{})
	if len(apps) != 12 {
		t.Errorf("apps count = %d, want 12", len(apps))
	}
}

func TestHealthUsesCatalogServiceWithoutCatalog(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	srv.catalogSvc = catalogSvcStub{
		appCountFn: func() int { return 7 },
	}

	w := doRequest(t, srv, "GET", "/api/health", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := decodeJSON(t, w)
	if int(body["app_count"].(float64)) != 7 {
		t.Fatalf("app_count = %v, want 7", body["app_count"])
	}
}

func TestListAppsSearch(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/apps?q=nginx", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := decodeJSON(t, w)
	total := body["total"].(float64)
	if total < 1 {
		t.Errorf("total = %v, want >= 1", total)
	}
}

func TestListAppsSearchNoResults(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/apps?q=nonexistentapp12345", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := decodeJSON(t, w)
	total := body["total"].(float64)
	if total != 0 {
		t.Errorf("total = %v, want 0", total)
	}
}

func TestListAppsCategoryFilter(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/apps?category=media", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := decodeJSON(t, w)
	total := body["total"].(float64)
	// jellyfin, plex, and qbittorrent are in "media"
	if total != 3 {
		t.Errorf("total = %v, want 3", total)
	}
}

func TestListAppsSort(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/apps?sort=name", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := decodeJSON(t, w)
	apps := body["apps"].([]interface{})
	if len(apps) < 2 {
		t.Fatal("expected at least 2 apps")
	}
	// Verify sorted by name (first should be Crawl4AI)
	first := apps[0].(map[string]interface{})
	if first["name"] != "Crawl4AI" {
		t.Errorf("first app = %q, want %q", first["name"], "Crawl4AI")
	}
}

func TestGetAppEndpoint(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/apps/nginx", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var app map[string]interface{}
	json.NewDecoder(w.Body).Decode(&app)

	if app["id"] != "nginx" {
		t.Errorf("id = %v, want %q", app["id"], "nginx")
	}
	if app["name"] != "Nginx" {
		t.Errorf("name = %v, want %q", app["name"], "Nginx")
	}
}

func TestGetAppNotFound(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/apps/nonexistent", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetAppReadmeNotFound(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/apps/nonexistent/readme", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// --- Categories ---

func TestCategoriesEndpoint(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/categories", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := decodeJSON(t, w)
	cats := body["categories"].([]interface{})
	if len(cats) == 0 {
		t.Error("expected at least one category")
	}

	// Check that "media" is in the list (from jellyfin/plex)
	found := false
	for _, c := range cats {
		if c.(string) == "media" {
			found = true
			break
		}
	}
	if !found {
		t.Error("categories should contain 'media'")
	}
}

// --- Jobs (empty initially) ---

func TestListJobsEmpty(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/jobs", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := decodeJSON(t, w)
	total := body["total"].(float64)
	if total != 0 {
		t.Errorf("total = %v, want 0", total)
	}
}

func TestGetJobNotFound(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/jobs/nonexistent", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetJobLogsNotFound(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/jobs/nonexistent/logs", "")

	// Logs for nonexistent job should still return 200 with empty logs
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := decodeJSON(t, w)
	lastID := body["last_id"].(float64)
	if lastID != 0 {
		t.Errorf("last_id = %v, want 0", lastID)
	}
}

// --- Installs (empty initially) ---

func TestListInstallsEmpty(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/installs", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := decodeJSON(t, w)
	total := body["total"].(float64)
	if total != 0 {
		t.Errorf("total = %v, want 0", total)
	}
}

// --- Get Install Detail ---

func TestGetInstallDetailNotFound(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/installs/nonexistent", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetInstallDetailAfterInstall(t *testing.T) {
	srv := testServer(t)

	// Start an install
	w := doRequest(t, srv, "POST", "/api/apps/nginx/install", `{"cores":1,"memory_mb":512,"disk_gb":4}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("install status = %d", w.Code)
	}

	var job map[string]interface{}
	json.NewDecoder(w.Body).Decode(&job)
	jobID := job["id"].(string)

	// Wait for async install to complete
	time.Sleep(500 * time.Millisecond)

	// Check if the job completed (it may have failed due to no real pct, but install record might exist)
	w2 := doRequest(t, srv, "GET", "/api/installs", "")
	body := decodeJSON(t, w2)
	installs := body["installs"].([]interface{})

	if len(installs) == 0 {
		// Job may have failed — check job state to confirm
		w3 := doRequest(t, srv, "GET", "/api/jobs/"+jobID, "")
		var gotJob map[string]interface{}
		json.NewDecoder(w3.Body).Decode(&gotJob)
		t.Skipf("Install job state=%v error=%v — skipping detail test", gotJob["state"], gotJob["error"])
	}

	// Get the install detail
	inst := installs[0].(map[string]interface{})
	instID := inst["id"].(string)

	w4 := doRequest(t, srv, "GET", "/api/installs/"+instID, "")
	if w4.Code != http.StatusOK {
		t.Fatalf("get install detail status = %d, body = %s", w4.Code, w4.Body.String())
	}

	detail := decodeJSON(t, w4)
	if detail["app_id"] != "nginx" {
		t.Errorf("app_id = %v, want nginx", detail["app_id"])
	}
	// Should have live status from mock
	if detail["live"] == nil {
		t.Error("expected live status data from mock")
	}
}

// --- Install App ---

func TestInstallAppCreatesJob(t *testing.T) {
	srv := testServer(t)
	reqBody := `{"cores": 1, "memory_mb": 512, "disk_gb": 4}`
	w := doRequest(t, srv, "POST", "/api/apps/nginx/install", reqBody)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusAccepted, w.Body.String())
	}

	var job map[string]interface{}
	json.NewDecoder(w.Body).Decode(&job)

	if job["app_id"] != "nginx" {
		t.Errorf("app_id = %v, want %q", job["app_id"], "nginx")
	}
	if job["type"] != "install" {
		t.Errorf("type = %v, want %q", job["type"], "install")
	}
	if job["id"] == nil || job["id"] == "" {
		t.Error("job should have an ID")
	}

	// Wait for async goroutine to run and the job to appear in the list
	time.Sleep(200 * time.Millisecond)

	w2 := doRequest(t, srv, "GET", "/api/jobs", "")
	body := decodeJSON(t, w2)
	total := body["total"].(float64)
	if total < 1 {
		t.Errorf("total = %v, want >= 1", total)
	}
}

func TestInstallAppNotFound(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "POST", "/api/apps/nonexistent/install", `{}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestInstallAppBadBody(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "POST", "/api/apps/nginx/install", "not json")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestInstallAppWithStaticIP(t *testing.T) {
	srv := testServer(t)
	reqBody := `{"cores": 1, "memory_mb": 512, "disk_gb": 4, "ip_address": "192.168.1.100"}`
	w := doRequest(t, srv, "POST", "/api/apps/nginx/install", reqBody)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusAccepted, w.Body.String())
	}

	var job map[string]interface{}
	json.NewDecoder(w.Body).Decode(&job)

	if job["app_id"] != "nginx" {
		t.Errorf("app_id = %v, want %q", job["app_id"], "nginx")
	}

	// Wait for async job to complete
	time.Sleep(200 * time.Millisecond)

	// Fetch the job and verify IP was stored
	jobID := job["id"].(string)
	w2 := doRequest(t, srv, "GET", "/api/jobs/"+jobID, "")
	body := decodeJSON(t, w2)
	if ipAddr, ok := body["ip_address"].(string); !ok || ipAddr != "192.168.1.100" {
		t.Errorf("ip_address = %v, want %q", body["ip_address"], "192.168.1.100")
	}
}

// --- Uninstall ---

func TestUninstallNotFound(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "POST", "/api/installs/nonexistent/uninstall", "")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// --- CORS ---

func TestCORSHeaders(t *testing.T) {
	srv := testServer(t)

	// Same-origin request should get CORS headers
	req := httptest.NewRequest("GET", "/api/health", nil)
	req.Host = "localhost:8088"
	req.Header.Set("Origin", "http://localhost:8088")
	w := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)

	cors := w.Header().Get("Access-Control-Allow-Origin")
	if cors != "http://localhost:8088" {
		t.Errorf("CORS origin = %q, want %q", cors, "http://localhost:8088")
	}
	creds := w.Header().Get("Access-Control-Allow-Credentials")
	if creds != "true" {
		t.Errorf("CORS credentials = %q, want %q", creds, "true")
	}
	methods := w.Header().Get("Access-Control-Allow-Methods")
	if methods == "" {
		t.Error("CORS methods header is empty")
	}

	// Cross-origin request from unknown origin should NOT get CORS origin header
	req2 := httptest.NewRequest("GET", "/api/health", nil)
	req2.Host = "localhost:8088"
	req2.Header.Set("Origin", "http://evil.com")
	w2 := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w2, req2)

	cors2 := w2.Header().Get("Access-Control-Allow-Origin")
	if cors2 != "" {
		t.Errorf("CORS origin for cross-origin = %q, want empty", cors2)
	}
}

func TestOptionsPreflight(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "OPTIONS", "/api/health", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

// --- Content-Type ---

func TestJSONContentType(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/health", "")

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// --- Auth modes ---

func TestAuthNonePassthrough(t *testing.T) {
	// With auth=none, protected endpoints should still be accessible
	srv := testServer(t)

	// catalog refresh is auth-protected
	w := doRequest(t, srv, "POST", "/api/catalog/refresh", "")
	// Should succeed (not 401)
	if w.Code == http.StatusUnauthorized {
		t.Errorf("auth=none should not require authentication")
	}
}

func TestAuthPasswordRequired(t *testing.T) {
	cfg := testConfig()
	cfg.Auth.Mode = config.AuthModePassword
	cfg.Auth.PasswordHash = "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ012" // dummy hash
	cat := testCatalog(t)
	eng := testEngine(t, cfg, cat)
	srv := New(cfg, cat, eng, nil)

	// Protected endpoint without auth should return 401
	w := doRequest(t, srv, "POST", "/api/catalog/refresh", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// Non-protected endpoint should still work
	w2 := doRequest(t, srv, "GET", "/api/health", "")
	if w2.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", w2.Code, http.StatusOK)
	}
}

// --- Job + Logs lifecycle ---

func TestJobLogsAfterInstall(t *testing.T) {
	srv := testServer(t)

	// Start an install
	w := doRequest(t, srv, "POST", "/api/apps/nginx/install", `{"cores":1,"memory_mb":512,"disk_gb":4}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("install status = %d", w.Code)
	}

	var job map[string]interface{}
	json.NewDecoder(w.Body).Decode(&job)
	jobID := job["id"].(string)

	// Wait for async goroutine to run (it will fail quickly since no real pct)
	time.Sleep(500 * time.Millisecond)

	// Get the job
	w2 := doRequest(t, srv, "GET", "/api/jobs/"+jobID, "")
	if w2.Code != http.StatusOK {
		t.Fatalf("get job status = %d", w2.Code)
	}

	var gotJob map[string]interface{}
	json.NewDecoder(w2.Body).Decode(&gotJob)
	if gotJob["id"] != jobID {
		t.Errorf("job id = %v, want %v", gotJob["id"], jobID)
	}

	// Get logs
	w3 := doRequest(t, srv, "GET", "/api/jobs/"+jobID+"/logs", "")
	if w3.Code != http.StatusOK {
		t.Fatalf("get logs status = %d", w3.Code)
	}

	var logsResp map[string]interface{}
	json.NewDecoder(w3.Body).Decode(&logsResp)
	logs := logsResp["logs"].([]interface{})
	if len(logs) == 0 {
		t.Error("expected at least 1 log entry after install attempt")
	}

	// Test logs with ?after= cursor
	lastID := int(logsResp["last_id"].(float64))
	w4 := doRequest(t, srv, "GET", fmt.Sprintf("/api/jobs/%s/logs?after=%d", jobID, lastID), "")
	if w4.Code != http.StatusOK {
		t.Fatalf("get logs with after status = %d", w4.Code)
	}
}

// --- No engine ---

func TestNoEngineJobsReturnsEmpty(t *testing.T) {
	cfg := testConfig()
	cat := testCatalog(t)
	srv := New(cfg, cat, nil, nil) // nil engine

	w := doRequest(t, srv, "GET", "/api/jobs", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := decodeJSON(t, w)
	if body["total"].(float64) != 0 {
		t.Errorf("total = %v, want 0", body["total"])
	}
}

func TestListJobsUsesEngineServiceWithoutEngine(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	setEngineServices(srv, engineSvcStub{
		listJobsFn: func() ([]*engine.Job, error) {
			return []*engine.Job{{ID: "job-1", AppID: "nginx"}}, nil
		},
	})

	w := doRequest(t, srv, "GET", "/api/jobs", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := decodeJSON(t, w)
	if int(body["total"].(float64)) != 1 {
		t.Fatalf("total = %v, want 1", body["total"])
	}
}

func TestListInstallsUsesEngineServiceWithoutEngine(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	setEngineServices(srv, engineSvcStub{
		listInstallsFn: func() ([]*engine.InstallListItem, error) {
			return []*engine.InstallListItem{{
				Install: engine.Install{ID: "inst-1", AppID: "nginx"},
			}}, nil
		},
	})

	w := doRequest(t, srv, "GET", "/api/installs", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := decodeJSON(t, w)
	if int(body["total"].(float64)) != 1 {
		t.Fatalf("total = %v, want 1", body["total"])
	}
}

func TestNoEngineInstallsReturnsEmpty(t *testing.T) {
	cfg := testConfig()
	cat := testCatalog(t)
	srv := New(cfg, cat, nil, nil)

	w := doRequest(t, srv, "GET", "/api/installs", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := decodeJSON(t, w)
	if body["total"].(float64) != 0 {
		t.Errorf("total = %v, want 0", body["total"])
	}
}

func TestNoEngineInstallReturns503(t *testing.T) {
	cfg := testConfig()
	cat := testCatalog(t)
	srv := New(cfg, cat, nil, nil)

	w := doRequest(t, srv, "POST", "/api/apps/nginx/install", `{}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestNoEngineGetJobReturns503(t *testing.T) {
	cfg := testConfig()
	cat := testCatalog(t)
	srv := New(cfg, cat, nil, nil)

	w := doRequest(t, srv, "GET", "/api/jobs/test-id", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestNoEngineTerminalReturns503(t *testing.T) {
	cfg := testConfig()
	cat := testCatalog(t)
	srv := New(cfg, cat, nil, nil)

	w := doRequest(t, srv, "GET", "/api/installs/test-id/terminal", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestNoEngineJournalLogsReturns503(t *testing.T) {
	cfg := testConfig()
	cat := testCatalog(t)
	srv := New(cfg, cat, nil, nil)

	w := doRequest(t, srv, "GET", "/api/installs/test-id/logs", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// --- Browse Paths ---

func TestBrowsePathsAllowedStorage(t *testing.T) {
	srv := testServer(t)
	if len(srv.allowedPaths) == 0 {
		t.Fatalf("no allowed paths configured")
	}
	root := srv.allowedPaths[0]
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatalf("mkdir storage root: %v", err)
	}
	w := doRequest(t, srv, "GET", "/api/browse/paths?path="+root, "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}

	body := decodeJSON(t, w)
	if body["path"] != root {
		t.Errorf("path = %v, want %s", body["path"], root)
	}
}

func TestBrowsePathsForbidden(t *testing.T) {
	srv := testServer(t)
	// /root is NOT under /tmp/test-storage, should be forbidden
	w := doRequest(t, srv, "GET", "/api/browse/paths?path=/root", "")

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestBrowsePathsInvalid(t *testing.T) {
	srv := testServer(t)
	if len(srv.allowedPaths) == 0 {
		t.Fatalf("no allowed paths configured")
	}
	root := srv.allowedPaths[0]
	w := doRequest(t, srv, "GET", "/api/browse/paths?path="+filepath.Join(root, "nonexistent-abc123"), "")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestBrowseMkdir(t *testing.T) {
	srv := testServer(t)
	if len(srv.allowedPaths) == 0 {
		t.Fatalf("no allowed paths configured")
	}
	root := srv.allowedPaths[0]
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatalf("mkdir storage root: %v", err)
	}
	newPath := filepath.Join(root, "new-folder")

	w := doRequest(t, srv, "POST", "/api/browse/mkdir", fmt.Sprintf(`{"path":%q}`, newPath))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}

	body := decodeJSON(t, w)
	if body["created"] != true {
		t.Errorf("created = %v, want true", body["created"])
	}

	// Cleanup
	os.RemoveAll(newPath)
}

func TestBrowseMkdirForbidden(t *testing.T) {
	srv := testServer(t)

	w := doRequest(t, srv, "POST", "/api/browse/mkdir", `{"path":"/tmp/not-allowed/folder"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

// --- Config Export/Apply ---

func TestConfigExportWithInstalls(t *testing.T) {
	srv := testServer(t)

	// Start an install for nginx (devices are determined from gpu_profile, not request)
	w := doRequest(t, srv, "POST", "/api/apps/nginx/install", `{"cores":1,"memory_mb":512,"disk_gb":4,"hostname":"my-nginx","env_vars":{"FOO":"bar"}}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("install status = %d (body: %s)", w.Code, w.Body.String())
	}

	// Wait for async install to complete
	time.Sleep(500 * time.Millisecond)

	// Check that installs exist
	w2 := doRequest(t, srv, "GET", "/api/installs", "")
	body := decodeJSON(t, w2)
	installs := body["installs"].([]interface{})
	if len(installs) == 0 {
		t.Skip("Install job failed — skipping export test")
	}

	// Export
	w3 := doRequest(t, srv, "GET", "/api/config/export", "")
	if w3.Code != http.StatusOK {
		t.Fatalf("export status = %d (body: %s)", w3.Code, w3.Body.String())
	}

	exportBody := decodeJSON(t, w3)
	if exportBody["node"] != "testnode" {
		t.Errorf("export node = %v, want testnode", exportBody["node"])
	}
	recipes := exportBody["recipes"].([]interface{})
	if len(recipes) == 0 {
		t.Fatal("expected at least 1 recipe in export")
	}

	recipe := recipes[0].(map[string]interface{})
	if recipe["app_id"] != "nginx" {
		t.Errorf("recipe app_id = %v, want nginx", recipe["app_id"])
	}
	if recipe["hostname"] != "my-nginx" {
		t.Errorf("recipe hostname = %v, want my-nginx", recipe["hostname"])
	}
	// Devices are determined from GPU profile, not request — verify nil/empty is fine
	if devices, ok := recipe["devices"].([]interface{}); ok && len(devices) > 0 {
		// If GPU profile matched and host has device, it would appear here
		_ = devices
	}
	// Verify env_vars
	if ev, ok := recipe["env_vars"].(map[string]interface{}); ok {
		if ev["FOO"] != "bar" {
			t.Errorf("env_var FOO = %v, want bar", ev["FOO"])
		}
	} else {
		t.Error("expected env_vars in recipe")
	}

	// Also verify raw installs are present
	rawInstalls := exportBody["installs"].([]interface{})
	if len(rawInstalls) == 0 {
		t.Error("expected raw installs in export")
	}
}

func TestConfigApplyCreatesJobs(t *testing.T) {
	srv := testServer(t)
	reqBody := `{"recipes":[{"app_id":"nginx","cores":1,"memory_mb":512,"disk_gb":4}]}`
	w := doRequest(t, srv, "POST", "/api/config/apply", reqBody)

	if w.Code != http.StatusAccepted {
		t.Fatalf("apply status = %d (body: %s)", w.Code, w.Body.String())
	}

	body := decodeJSON(t, w)
	jobs := body["jobs"].([]interface{})
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}

	job := jobs[0].(map[string]interface{})
	if job["app_id"] != "nginx" {
		t.Errorf("job app_id = %v, want nginx", job["app_id"])
	}
	if job["job_id"] == nil || job["job_id"] == "" {
		t.Error("job should have a job_id")
	}
}

func TestConfigApplyBadAppID(t *testing.T) {
	srv := testServer(t)
	reqBody := `{"recipes":[{"app_id":"nonexistent-app","cores":1,"memory_mb":512,"disk_gb":4}]}`
	w := doRequest(t, srv, "POST", "/api/config/apply", reqBody)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("apply status = %d, want %d (body: %s)", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestConfigApplyAndPreviewValidationPaths(t *testing.T) {
	cases := []struct {
		name     string
		method   string
		path     string
		body     string
		wantCode int
		contains string
	}{
		{
			name:     "apply invalid json",
			method:   "POST",
			path:     "/api/config/apply",
			body:     `{"recipes":`,
			wantCode: http.StatusBadRequest,
			contains: "invalid request body",
		},
		{
			name:     "apply empty payload",
			method:   "POST",
			path:     "/api/config/apply",
			body:     `{}`,
			wantCode: http.StatusBadRequest,
			contains: "no recipes or stacks provided",
		},
		{
			name:     "apply unknown app",
			method:   "POST",
			path:     "/api/config/apply",
			body:     `{"recipes":[{"app_id":"does-not-exist"}]}`,
			wantCode: http.StatusBadRequest,
			contains: "does-not-exist",
		},
		{
			name:     "preview empty body",
			method:   "POST",
			path:     "/api/config/apply/preview",
			body:     "",
			wantCode: http.StatusBadRequest,
			contains: "empty request body",
		},
		{
			name:     "preview missing app_id",
			method:   "POST",
			path:     "/api/config/apply/preview",
			body:     `{"recipes":[{}]}`,
			wantCode: http.StatusOK,
			contains: "recipe[0]: missing app_id",
		},
		{
			name:     "preview unknown app",
			method:   "POST",
			path:     "/api/config/apply/preview",
			body:     `{"stacks":[{"name":"web","apps":[{"app_id":"does-not-exist"}]}]}`,
			wantCode: http.StatusOK,
			contains: "stack[0].apps[0]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := testServer(t)
			w := doRequest(t, srv, tc.method, tc.path, tc.body)
			if w.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, tc.wantCode, w.Body.String())
			}
			if tc.contains != "" && !strings.Contains(w.Body.String(), tc.contains) {
				t.Fatalf("body %q does not contain %q", w.Body.String(), tc.contains)
			}
		})
	}
}

func TestConfigExportDownload(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/config/export/download", "")

	if w.Code != http.StatusOK {
		t.Fatalf("download status = %d (body: %s)", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "yaml") {
		t.Errorf("Content-Type = %q, want yaml", ct)
	}

	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Errorf("Content-Disposition = %q, want attachment", cd)
	}
}

func TestConfigDefaultsWithStorageDetails(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/config/defaults", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	body := decodeJSON(t, w)
	details := body["storage_details"].([]interface{})
	if len(details) != 1 {
		t.Fatalf("expected 1 storage_detail, got %d", len(details))
	}

	detail := details[0].(map[string]interface{})
	if detail["id"] != "local-lvm" {
		t.Errorf("storage detail id = %v, want local-lvm", detail["id"])
	}
	if detail["type"] != "lvmthin" {
		t.Errorf("storage detail type = %v, want lvmthin", detail["type"])
	}

	// Verify capacity fields are present
	totalGB := detail["total_gb"].(float64)
	if totalGB != 100 {
		t.Errorf("total_gb = %v, want 100", totalGB)
	}
	availGB := detail["available_gb"].(float64)
	if availGB != 80 {
		t.Errorf("available_gb = %v, want 80", availGB)
	}
}

func TestNoEngineConfigExportReturns503(t *testing.T) {
	cfg := testConfig()
	cat := testCatalog(t)
	srv := New(cfg, cat, nil, nil)

	w := doRequest(t, srv, "GET", "/api/config/export", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestNoEngineConfigApplyReturns503(t *testing.T) {
	cfg := testConfig()
	cat := testCatalog(t)
	srv := New(cfg, cat, nil, nil)

	w := doRequest(t, srv, "POST", "/api/config/apply", `{"recipes":[{"app_id":"nginx"}]}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestConfigExportUsesServiceWithoutEngine(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	setEngineServices(srv, engineSvcStub{
		listRawFn: func() ([]*engine.Install, error) {
			return []*engine.Install{{
				ID:       "inst-1",
				AppID:    "nginx",
				AppName:  "Nginx",
				Storage:  "local-lvm",
				Bridge:   "vmbr0",
				Cores:    2,
				MemoryMB: 512,
				DiskGB:   4,
				Inputs:   map[string]string{"http_port": "8080"},
			}}, nil
		},
		listStacksFn: func() ([]*engine.Stack, error) {
			return []*engine.Stack{}, nil
		},
	})
	srv.catalogSvc = catalogSvcStub{
		getAppFn: func(id string) (*catalog.AppManifest, bool) {
			return &catalog.AppManifest{
				ID: "nginx",
				Inputs: []catalog.InputSpec{
					{Key: "http_port", Label: "HTTP Port"},
				},
			}, true
		},
	}

	w := doRequest(t, srv, "GET", "/api/config/export", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	if body["node"] != "testnode" {
		t.Fatalf("node = %v, want testnode", body["node"])
	}
	recipes := body["recipes"].([]interface{})
	if len(recipes) != 1 {
		t.Fatalf("recipes len = %d, want 1", len(recipes))
	}
}

func TestConfigApplyUsesServicesWithoutEngineCatalog(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	srv.catalogSvc = catalogSvcStub{
		getAppFn: func(id string) (*catalog.AppManifest, bool) {
			return &catalog.AppManifest{ID: id}, true
		},
	}
	setEngineServices(srv, engineSvcStub{
		startInstallFn: func(req engine.InstallRequest) (*engine.Job, error) {
			return &engine.Job{ID: "job-install", AppID: req.AppID}, nil
		},
		startStackFn: func(req engine.StackCreateRequest) (*engine.Job, error) {
			return &engine.Job{ID: "job-stack", AppID: "stack"}, nil
		},
	})

	reqBody := `{
		"recipes":[{"app_id":"nginx"}],
		"stacks":[{"name":"web","apps":[{"app_id":"nginx"}]}]
	}`
	w := doRequest(t, srv, "POST", "/api/config/apply", reqBody)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusAccepted, w.Body.String())
	}
	body := decodeJSON(t, w)
	jobs := body["jobs"].([]interface{})
	if len(jobs) != 1 {
		t.Fatalf("jobs len = %d, want 1", len(jobs))
	}
	stackJobs := body["stack_jobs"].([]interface{})
	if len(stackJobs) != 1 {
		t.Fatalf("stack_jobs len = %d, want 1", len(stackJobs))
	}
}

// --- Install with bind mounts ---

func TestInstallAppWithBindMounts(t *testing.T) {
	srv := testServer(t)
	reqBody := `{"cores": 1, "memory_mb": 512, "disk_gb": 4, "bind_mounts": {"media": "/mnt/storage/movies"}}`
	w := doRequest(t, srv, "POST", "/api/apps/jellyfin/install", reqBody)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusAccepted, w.Body.String())
	}

	var job map[string]interface{}
	json.NewDecoder(w.Body).Decode(&job)
	if job["app_id"] != "jellyfin" {
		t.Errorf("app_id = %v, want %q", job["app_id"], "jellyfin")
	}

	// Wait for async job to start processing
	time.Sleep(200 * time.Millisecond)

	// Verify mount points on the job include bind mount
	jobID := job["id"].(string)
	w2 := doRequest(t, srv, "GET", fmt.Sprintf("/api/jobs/%s", jobID), "")
	if w2.Code != http.StatusOK {
		t.Fatalf("get job status = %d", w2.Code)
	}
	var gotJob map[string]interface{}
	json.NewDecoder(w2.Body).Decode(&gotJob)
	mounts := gotJob["mount_points"].([]interface{})
	if len(mounts) < 1 {
		t.Fatal("expected at least 1 mount point")
	}

	// Check that we have both volume and bind mount types
	hasVolume := false
	hasMediaBind := false
	for _, m := range mounts {
		mp := m.(map[string]interface{})
		switch mp["type"] {
		case "volume":
			hasVolume = true
		case "bind":
			if mp["name"] == "media" {
				hasMediaBind = true
				if mp["host_path"] != "/mnt/storage/movies" {
					t.Errorf("media bind mount host_path = %v, want /mnt/storage/movies", mp["host_path"])
				}
			}
		}
	}
	if !hasVolume {
		t.Error("expected a managed volume mount")
	}
	if !hasMediaBind {
		t.Error("expected a media bind mount")
	}
}

func TestInstallAppDuplicateReturns409(t *testing.T) {
	srv := testServer(t)
	reqBody := `{"cores": 1, "memory_mb": 512, "disk_gb": 4}`

	// First install should succeed
	w1 := doRequest(t, srv, "POST", "/api/apps/nginx/install", reqBody)
	if w1.Code != http.StatusAccepted {
		t.Fatalf("first install status = %d, want %d (body: %s)", w1.Code, http.StatusAccepted, w1.Body.String())
	}
	firstJob := decodeJSON(t, w1)
	firstJobID, _ := firstJob["id"].(string)

	// Second install of same app should return 409
	w2 := doRequest(t, srv, "POST", "/api/apps/nginx/install", reqBody)
	if w2.Code != http.StatusConflict {
		t.Fatalf("duplicate install status = %d, want %d (body: %s)", w2.Code, http.StatusConflict, w2.Body.String())
	}

	body := decodeJSON(t, w2)
	if body["error"] == nil || body["error"] == "" {
		t.Error("expected error message in 409 response")
	}
	if body["existing_job_id"] == nil || body["existing_job_id"] == "" {
		t.Error("expected existing_job_id in 409 response")
	}

	if firstJobID != "" {
		waitForJobTerminalState(t, srv, firstJobID)
	}
}

func TestInstallAppDifferentAppsAllowed(t *testing.T) {
	srv := testServer(t)
	reqBody := `{"cores": 1, "memory_mb": 512, "disk_gb": 4}`

	w1 := doRequest(t, srv, "POST", "/api/apps/nginx/install", reqBody)
	if w1.Code != http.StatusAccepted {
		t.Fatalf("nginx install status = %d, want %d", w1.Code, http.StatusAccepted)
	}
	job1 := decodeJSON(t, w1)
	job1ID, _ := job1["id"].(string)

	// Different app should still be allowed
	w2 := doRequest(t, srv, "POST", "/api/apps/ollama/install", reqBody)
	if w2.Code != http.StatusAccepted {
		t.Fatalf("ollama install status = %d, want %d (body: %s)", w2.Code, http.StatusAccepted, w2.Body.String())
	}
	job2 := decodeJSON(t, w2)
	job2ID, _ := job2["id"].(string)

	if job1ID != "" {
		waitForJobTerminalState(t, srv, job1ID)
	}
	if job2ID != "" {
		waitForJobTerminalState(t, srv, job2ID)
	}
}

// --- Stacks ---

func TestListStacksEmpty(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/stacks", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := decodeJSON(t, w)
	total := body["total"].(float64)
	if total != 0 {
		t.Errorf("total = %v, want 0", total)
	}
}

func TestGetStackNotFound(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/stacks/nonexistent", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestCreateStackCreatesJob(t *testing.T) {
	srv := testServer(t)
	reqBody := `{"name":"test-stack","apps":[{"app_id":"nginx"},{"app_id":"ollama"}],"cores":2,"memory_mb":2048,"disk_gb":16}`
	w := doRequest(t, srv, "POST", "/api/stacks", reqBody)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusAccepted, w.Body.String())
	}

	var job map[string]interface{}
	json.NewDecoder(w.Body).Decode(&job)

	if job["type"] != "stack" {
		t.Errorf("type = %v, want %q", job["type"], "stack")
	}
	if job["stack_id"] == nil || job["stack_id"] == "" {
		t.Error("job should have a stack_id")
	}

	// Wait for async goroutine to run
	time.Sleep(500 * time.Millisecond)

	// Verify job exists
	jobID := job["id"].(string)
	w2 := doRequest(t, srv, "GET", "/api/jobs/"+jobID, "")
	if w2.Code != http.StatusOK {
		t.Fatalf("get job status = %d", w2.Code)
	}
}

func TestCreateStackBadApp(t *testing.T) {
	srv := testServer(t)
	reqBody := `{"name":"test-stack","apps":[{"app_id":"nonexistent"}]}`
	w := doRequest(t, srv, "POST", "/api/stacks", reqBody)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreateStackNoName(t *testing.T) {
	srv := testServer(t)
	reqBody := `{"apps":[{"app_id":"nginx"}]}`
	w := doRequest(t, srv, "POST", "/api/stacks", reqBody)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreateStackNoApps(t *testing.T) {
	srv := testServer(t)
	reqBody := `{"name":"empty-stack","apps":[]}`
	w := doRequest(t, srv, "POST", "/api/stacks", reqBody)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestValidateStack(t *testing.T) {
	srv := testServer(t)
	reqBody := `{"name":"test","apps":[{"app_id":"nginx"},{"app_id":"ollama"}]}`
	w := doRequest(t, srv, "POST", "/api/stacks/validate", reqBody)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}

	body := decodeJSON(t, w)
	if body["valid"] != true {
		t.Errorf("valid = %v, want true", body["valid"])
	}
	if body["recommended"] == nil {
		t.Error("expected recommended resources")
	}
}

func TestValidateStackBadApp(t *testing.T) {
	srv := testServer(t)
	reqBody := `{"name":"test","apps":[{"app_id":"nonexistent"}]}`
	w := doRequest(t, srv, "POST", "/api/stacks/validate", reqBody)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := decodeJSON(t, w)
	if body["valid"] != false {
		t.Errorf("valid = %v, want false", body["valid"])
	}
}

func TestNoEngineStacksReturnsEmpty(t *testing.T) {
	cfg := testConfig()
	cat := testCatalog(t)
	srv := New(cfg, cat, nil, nil)

	w := doRequest(t, srv, "GET", "/api/stacks", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := decodeJSON(t, w)
	if body["total"].(float64) != 0 {
		t.Errorf("total = %v, want 0", body["total"])
	}
}

func TestListStacksUsesEngineServiceWithoutEngine(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	setEngineServices(srv, engineSvcStub{
		listStacksEnrichedFn: func() ([]*engine.StackListItem, error) {
			return []*engine.StackListItem{{Stack: engine.Stack{ID: "stack-1", Name: "web"}}}, nil
		},
	})

	w := doRequest(t, srv, "GET", "/api/stacks", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := decodeJSON(t, w)
	if int(body["total"].(float64)) != 1 {
		t.Fatalf("total = %v, want 1", body["total"])
	}
}

func TestGetStackUsesEngineServiceWithoutEngine(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	setEngineServices(srv, engineSvcStub{
		getStackDetailFn: func(id string) (*engine.StackDetail, error) {
			return &engine.StackDetail{Stack: engine.Stack{ID: id, Name: "web"}}, nil
		},
	})

	w := doRequest(t, srv, "GET", "/api/stacks/stack-1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	if body["id"] != "stack-1" {
		t.Fatalf("id = %v, want stack-1", body["id"])
	}
}

func TestValidateStackUsesEngineServiceWithoutEngine(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	setEngineServices(srv, engineSvcStub{
		validateStackFn: func(req engine.StackCreateRequest) map[string]interface{} {
			return map[string]interface{}{
				"valid":       true,
				"recommended": map[string]interface{}{"cores": 2},
			}
		},
	})

	w := doRequest(t, srv, "POST", "/api/stacks/validate", `{"name":"x","apps":[{"app_id":"nginx"}]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := decodeJSON(t, w)
	if body["valid"] != true {
		t.Fatalf("valid = %v, want true", body["valid"])
	}
}

func TestDevListAppsUsesDevServiceWithoutStore(t *testing.T) {
	cfg := testConfig()
	cfg.Developer.Enabled = true
	srv := New(cfg, nil, nil, nil)
	srv.devSvc = devSvcStub{
		listFn: func() ([]devmode.DevAppMeta, error) {
			return []devmode.DevAppMeta{{ID: "my-app", Name: "My App"}}, nil
		},
	}

	w := doRequest(t, srv, "GET", "/api/dev/apps", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	if int(body["total"].(float64)) != 1 {
		t.Fatalf("total = %v, want 1", body["total"])
	}
}

func TestDevForkUsesServicesWithoutStoreCatalog(t *testing.T) {
	cfg := testConfig()
	cfg.Developer.Enabled = true
	srv := New(cfg, nil, nil, nil)
	srv.catalogSvc = catalogSvcStub{
		getAppFn: func(id string) (*catalog.AppManifest, bool) {
			return &catalog.AppManifest{ID: id, DirPath: "/tmp/source-app"}, true
		},
	}
	srv.devSvc = devSvcStub{
		forkFn: func(newID, sourceDir string) error { return nil },
		getFn: func(id string) (*devmode.DevApp, error) {
			return &devmode.DevApp{DevAppMeta: devmode.DevAppMeta{ID: id, Name: "Forked"}}, nil
		},
	}

	w := doRequest(t, srv, "POST", "/api/dev/fork", `{"source_id":"nginx","new_id":"nginx-fork"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusCreated, w.Body.String())
	}
	body := decodeJSON(t, w)
	if body["id"] != "nginx-fork" {
		t.Fatalf("id = %v, want nginx-fork", body["id"])
	}
}

func TestStackCreateAndList(t *testing.T) {
	srv := testServer(t)
	reqBody := `{"name":"my-stack","apps":[{"app_id":"nginx"}],"cores":2,"memory_mb":1024,"disk_gb":8}`
	w := doRequest(t, srv, "POST", "/api/stacks", reqBody)
	if w.Code != http.StatusAccepted {
		t.Fatalf("create status = %d (body: %s)", w.Code, w.Body.String())
	}

	// Wait for async install to complete
	time.Sleep(500 * time.Millisecond)

	// List stacks
	w2 := doRequest(t, srv, "GET", "/api/stacks", "")
	body := decodeJSON(t, w2)
	stacks := body["stacks"].([]interface{})
	if len(stacks) == 0 {
		// Job may have failed — check it
		var job map[string]interface{}
		json.NewDecoder(w.Body).Decode(&job)
		t.Skipf("Stack job may have failed — no stacks in list")
	}
}

func TestReconfigureInstall(t *testing.T) {
	srv := testServer(t)

	// Start an install first
	w := doRequest(t, srv, "POST", "/api/apps/nginx/install", `{"cores":1,"memory_mb":512,"disk_gb":4}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("install status = %d (body: %s)", w.Code, w.Body.String())
	}

	// Wait for async install to complete
	time.Sleep(500 * time.Millisecond)

	// Check if install exists
	w2 := doRequest(t, srv, "GET", "/api/installs", "")
	body := decodeJSON(t, w2)
	installs := body["installs"].([]interface{})
	if len(installs) == 0 {
		t.Skip("Install job failed — skipping reconfigure test")
	}

	inst := installs[0].(map[string]interface{})
	instID := inst["id"].(string)

	// Reconfigure: change cores and memory
	w3 := doRequest(t, srv, "POST", "/api/installs/"+instID+"/reconfigure", `{"cores":4,"memory_mb":2048}`)
	if w3.Code != http.StatusOK {
		t.Fatalf("reconfigure status = %d, want %d (body: %s)", w3.Code, http.StatusOK, w3.Body.String())
	}

	result := decodeJSON(t, w3)
	if result["cores"].(float64) != 4 {
		t.Errorf("cores = %v, want 4", result["cores"])
	}
	if result["memory_mb"].(float64) != 2048 {
		t.Errorf("memory_mb = %v, want 2048", result["memory_mb"])
	}
}

func TestReconfigureInstallNotFound(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "POST", "/api/installs/nonexistent/reconfigure", `{"cores":4}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// --- Settings ---

func TestGetSettingsEndpoint(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/settings", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := decodeJSON(t, w)
	defaults := body["defaults"].(map[string]interface{})
	if defaults["cores"].(float64) != 2 {
		t.Errorf("defaults.cores = %v, want 2", defaults["cores"])
	}
	if defaults["memory_mb"].(float64) != 2048 {
		t.Errorf("defaults.memory_mb = %v, want 2048", defaults["memory_mb"])
	}
	storages := body["storages"].([]interface{})
	if len(storages) != 1 || storages[0] != "local-lvm" {
		t.Errorf("storages = %v, want [local-lvm]", storages)
	}
	bridges := body["bridges"].([]interface{})
	if len(bridges) != 1 || bridges[0] != "vmbr0" {
		t.Errorf("bridges = %v, want [vmbr0]", bridges)
	}
}

// --- Cancel Job ---

func TestCancelJobViaStub(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	cancelled := false
	setEngineServices(srv, engineSvcStub{
		cancelJobFn: func(id string) error {
			if id == "job-1" {
				cancelled = true
				return nil
			}
			return fmt.Errorf("not found")
		},
	})

	w := doRequest(t, srv, "POST", "/api/jobs/job-1/cancel", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	if !cancelled {
		t.Error("expected cancel to be called")
	}
}

func TestCancelJobNotFound(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	setEngineServices(srv, engineSvcStub{
		cancelJobFn: func(id string) error {
			return fmt.Errorf("job %q not found", id)
		},
	})

	w := doRequest(t, srv, "POST", "/api/jobs/nonexistent/cancel", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCancelJobNoEngine(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)

	w := doRequest(t, srv, "POST", "/api/jobs/job-1/cancel", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// --- Clear Jobs ---

func TestClearJobsViaStub(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	setEngineServices(srv, engineSvcStub{
		clearJobsFn: func() (int64, error) {
			return 3, nil
		},
	})

	w := doRequest(t, srv, "DELETE", "/api/jobs", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	if body["deleted"].(float64) != 3 {
		t.Errorf("deleted = %v, want 3", body["deleted"])
	}
}

func TestClearJobsNoEngine(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)

	w := doRequest(t, srv, "DELETE", "/api/jobs", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// --- Container Lifecycle via Stubs ---

func TestStartContainerViaStub(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	setEngineServices(srv, engineSvcStub{
		startContainerFn: func(id string) error { return nil },
	})

	w := doRequest(t, srv, "POST", "/api/installs/inst-1/start", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	if body["status"] != "started" {
		t.Errorf("status = %v, want started", body["status"])
	}
}

func TestStopContainerViaStub(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	setEngineServices(srv, engineSvcStub{
		stopContainerFn: func(id string) error { return nil },
	})

	w := doRequest(t, srv, "POST", "/api/installs/inst-1/stop", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	if body["status"] != "stopped" {
		t.Errorf("status = %v, want stopped", body["status"])
	}
}

func TestRestartContainerViaStub(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	setEngineServices(srv, engineSvcStub{
		restartContainerFn: func(id string) error { return nil },
	})

	w := doRequest(t, srv, "POST", "/api/installs/inst-1/restart", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	if body["status"] != "restarted" {
		t.Errorf("status = %v, want restarted", body["status"])
	}
}

func TestContainerLifecycleNoEngine(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)

	for _, op := range []string{"start", "stop", "restart"} {
		w := doRequest(t, srv, "POST", "/api/installs/inst-1/"+op, "")
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: status = %d, want %d", op, w.Code, http.StatusServiceUnavailable)
		}
	}
}

// --- Stack Lifecycle via Stubs ---

func TestStartStackViaStub(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	setEngineServices(srv, engineSvcStub{
		startStackContFn: func(id string) error { return nil },
	})

	w := doRequest(t, srv, "POST", "/api/stacks/stack-1/start", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	if body["status"] != "started" {
		t.Errorf("status = %v, want started", body["status"])
	}
}

func TestStopStackViaStub(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	setEngineServices(srv, engineSvcStub{
		stopStackContFn: func(id string) error { return nil },
	})

	w := doRequest(t, srv, "POST", "/api/stacks/stack-1/stop", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	if body["status"] != "stopped" {
		t.Errorf("status = %v, want stopped", body["status"])
	}
}

func TestRestartStackViaStub(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	setEngineServices(srv, engineSvcStub{
		restartStackContFn: func(id string) error { return nil },
	})

	w := doRequest(t, srv, "POST", "/api/stacks/stack-1/restart", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	if body["status"] != "restarted" {
		t.Errorf("status = %v, want restarted", body["status"])
	}
}

func TestUninstallStackViaStub(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	setEngineServices(srv, engineSvcStub{
		uninstallStackFn: func(id string) (*engine.Job, error) {
			return &engine.Job{ID: "uninstall-job-1", Type: "uninstall"}, nil
		},
	})

	w := doRequest(t, srv, "POST", "/api/stacks/stack-1/uninstall", "")
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusAccepted, w.Body.String())
	}
}

func TestStackLifecycleNoEngine(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)

	for _, op := range []string{"start", "stop", "restart", "uninstall"} {
		w := doRequest(t, srv, "POST", "/api/stacks/stack-1/"+op, "")
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: status = %d, want %d", op, w.Code, http.StatusServiceUnavailable)
		}
	}
}

// --- Browse Storages / Mounts ---

func TestBrowseStorages(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/browse/storages", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := decodeJSON(t, w)
	storages := body["storages"].([]interface{})
	if len(storages) != 1 || storages[0] != "local-lvm" {
		t.Errorf("storages = %v, want [local-lvm]", storages)
	}
}

func TestBrowseMounts(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/browse/mounts", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	body := decodeJSON(t, w)
	mounts := body["mounts"].([]interface{})
	// We have 1 storage with a browsable path from the mock
	if len(mounts) != 1 {
		t.Fatalf("mounts count = %d, want 1", len(mounts))
	}
	mount := mounts[0].(map[string]interface{})
	if mount["device"] != "local-lvm" {
		t.Errorf("mount device = %v, want local-lvm", mount["device"])
	}
}

// --- Dev Mode Gating ---

func TestDevModeDisabledReturns403(t *testing.T) {
	cfg := testConfig()
	cfg.Developer.Enabled = false
	srv := New(cfg, nil, nil, nil)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/dev/apps"},
		{"POST", "/api/dev/apps"},
		{"POST", "/api/dev/fork"},
		{"GET", "/api/dev/templates"},
	}

	for _, ep := range endpoints {
		w := doRequest(t, srv, ep.method, ep.path, "")
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want %d", ep.method, ep.path, w.Code, http.StatusForbidden)
		}
	}
}

// --- Purge Install ---

func TestPurgeInstallViaStub(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)
	setEngineServices(srv, engineSvcStub{
		purgeInstallFn: func(id string) error { return nil },
	})

	w := doRequest(t, srv, "DELETE", "/api/installs/inst-1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	if body["status"] != "purged" {
		t.Errorf("status = %v, want purged", body["status"])
	}
}

func TestPurgeInstallNoEngine(t *testing.T) {
	cfg := testConfig()
	srv := New(cfg, nil, nil, nil)

	w := doRequest(t, srv, "DELETE", "/api/installs/inst-1", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// --- App Icon ---

func TestGetAppIconDefault(t *testing.T) {
	srv := testServer(t)
	// nginx app exists but has no custom icon — should serve default icon
	w := doRequest(t, srv, "GET", "/api/apps/nginx/icon", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "image/png") {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if w.Body.Len() == 0 {
		t.Error("expected non-empty icon body")
	}
}

func TestGetAppIconNotFound(t *testing.T) {
	srv := testServer(t)
	w := doRequest(t, srv, "GET", "/api/apps/nonexistent/icon", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// --- App Status ---

func TestAppStatusEndpoint(t *testing.T) {
	srv := testServer(t)

	// Before install: not installed, no active job
	w1 := doRequest(t, srv, "GET", "/api/apps/nginx/status", "")
	if w1.Code != http.StatusOK {
		t.Fatalf("status = %d", w1.Code)
	}
	body1 := decodeJSON(t, w1)
	if body1["installed"] != false {
		t.Error("expected installed=false before install")
	}
	if body1["job_active"] != false {
		t.Error("expected job_active=false before install")
	}

	// Start install
	doRequest(t, srv, "POST", "/api/apps/nginx/install", `{"cores":1,"memory_mb":512,"disk_gb":4}`)

	// After install: should show active job
	w2 := doRequest(t, srv, "GET", "/api/apps/nginx/status", "")
	if w2.Code != http.StatusOK {
		t.Fatalf("status = %d", w2.Code)
	}
	body2 := decodeJSON(t, w2)
	if body2["job_active"] != true {
		t.Error("expected job_active=true after install")
	}
	if body2["job_id"] == nil || body2["job_id"] == "" {
		t.Error("expected job_id in status response")
	}
}

// --- Dev Handler Tests ---

func devServer(t *testing.T, devSvc devSvcStub, catSvc *catalogSvcStub) *Server {
	t.Helper()
	cfg := testConfig()
	cfg.Developer.Enabled = true
	srv := New(cfg, nil, nil, nil)
	srv.devSvc = devSvc
	if catSvc != nil {
		srv.catalogSvc = *catSvc
	}
	return srv
}

func TestDevCreateAppSuccess(t *testing.T) {
	srv := devServer(t, devSvcStub{
		createFn: func(id, template string) error { return nil },
		getFn: func(id string) (*devmode.DevApp, error) {
			return &devmode.DevApp{DevAppMeta: devmode.DevAppMeta{ID: id, Name: "Test"}}, nil
		},
	}, nil)

	w := doRequest(t, srv, "POST", "/api/dev/apps", `{"id":"my-app","template":"basic"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusCreated, w.Body.String())
	}
	body := decodeJSON(t, w)
	if body["id"] != "my-app" {
		t.Errorf("id = %v, want my-app", body["id"])
	}
}

func TestDevCreateAppInvalidBody(t *testing.T) {
	srv := devServer(t, devSvcStub{}, nil)

	w := doRequest(t, srv, "POST", "/api/dev/apps", `{bad json}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDevCreateAppMissingID(t *testing.T) {
	srv := devServer(t, devSvcStub{}, nil)

	w := doRequest(t, srv, "POST", "/api/dev/apps", `{"template":"basic"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDevCreateAppDevModeOff(t *testing.T) {
	cfg := testConfig()
	cfg.Developer.Enabled = false
	srv := New(cfg, nil, nil, nil)

	w := doRequest(t, srv, "POST", "/api/dev/apps", `{"id":"test"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestDevGetAppFound(t *testing.T) {
	srv := devServer(t, devSvcStub{
		getFn: func(id string) (*devmode.DevApp, error) {
			return &devmode.DevApp{DevAppMeta: devmode.DevAppMeta{ID: id, Name: "My App"}}, nil
		},
	}, nil)

	w := doRequest(t, srv, "GET", "/api/dev/apps/my-app", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	if body["id"] != "my-app" {
		t.Errorf("id = %v, want my-app", body["id"])
	}
}

func TestDevGetAppNotFound(t *testing.T) {
	srv := devServer(t, devSvcStub{
		getFn: func(id string) (*devmode.DevApp, error) {
			return nil, fmt.Errorf("not found")
		},
	}, nil)

	w := doRequest(t, srv, "GET", "/api/dev/apps/nonexistent", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestDevSaveManifestSuccess(t *testing.T) {
	saved := false
	srv := devServer(t, devSvcStub{
		saveManifestFn: func(id string, data []byte) error {
			saved = true
			return nil
		},
		isDeployedFn: func(id string) bool { return false },
	}, nil)

	w := doRequest(t, srv, "PUT", "/api/dev/apps/my-app/manifest", "id: my-app\nname: My App")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	if !saved {
		t.Error("expected save to be called")
	}
}

func TestDevSaveManifestError(t *testing.T) {
	srv := devServer(t, devSvcStub{
		saveManifestFn: func(id string, data []byte) error {
			return fmt.Errorf("invalid YAML")
		},
	}, nil)

	w := doRequest(t, srv, "PUT", "/api/dev/apps/my-app/manifest", "bad: [yaml")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDevSaveScriptSuccess(t *testing.T) {
	saved := false
	srv := devServer(t, devSvcStub{
		saveScriptFn: func(id string, data []byte) error {
			saved = true
			return nil
		},
		isDeployedFn: func(id string) bool { return false },
	}, nil)

	w := doRequest(t, srv, "PUT", "/api/dev/apps/my-app/script", "#!/usr/bin/env python3\nprint('hello')")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	if !saved {
		t.Error("expected save to be called")
	}
}

func TestDevDeleteAppSuccess(t *testing.T) {
	deleted := false
	srv := devServer(t, devSvcStub{
		deleteFn: func(id string) error {
			deleted = true
			return nil
		},
	}, &catalogSvcStub{
		removeDevAppFn: func(id string) {},
	})

	w := doRequest(t, srv, "DELETE", "/api/dev/apps/my-app", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	if !deleted {
		t.Error("expected delete to be called")
	}
}

func TestDevDeleteAppError(t *testing.T) {
	srv := devServer(t, devSvcStub{
		deleteFn: func(id string) error {
			return fmt.Errorf("not found")
		},
	}, &catalogSvcStub{
		removeDevAppFn: func(id string) {},
	})

	w := doRequest(t, srv, "DELETE", "/api/dev/apps/nonexistent", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDevDeploySuccess(t *testing.T) {
	// Create a temp dir that passes devmode.Validate()
	appDir := t.TempDir()
	manifest := `id: test-app
name: Test App
version: '1.0.0'
description: A test application
categories:
  - utility
lxc:
  ostemplate: debian-12
  defaults:
    cores: 1
    memory_mb: 512
    disk_gb: 4
provisioning:
  script: install.py
`
	script := `from appstore import BaseApp, run

class TestApp(BaseApp):
    def install(self):
        pass

run(TestApp)
`
	os.WriteFile(filepath.Join(appDir, "app.yml"), []byte(manifest), 0644)
	os.MkdirAll(filepath.Join(appDir, "provision"), 0755)
	os.WriteFile(filepath.Join(appDir, "provision", "install.py"), []byte(script), 0644)

	srv := devServer(t, devSvcStub{
		appDirFn:    func(id string) string { return appDir },
		setStatusFn: func(id, status string) error { return nil },
		parseManifestFn: func(id string) (*catalog.AppManifest, error) {
			return &catalog.AppManifest{ID: "test-app", Name: "Test App"}, nil
		},
	}, &catalogSvcStub{
		mergeDevAppFn: func(app *catalog.AppManifest) {},
	})

	w := doRequest(t, srv, "POST", "/api/dev/apps/test-app/deploy", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	if body["status"] != "deployed" {
		t.Errorf("status = %v, want deployed", body["status"])
	}
}

func TestDevUndeploySuccess(t *testing.T) {
	statusSet := ""
	srv := devServer(t, devSvcStub{
		setStatusFn: func(id, status string) error {
			statusSet = status
			return nil
		},
	}, &catalogSvcStub{
		removeDevAppFn: func(id string) {},
	})

	w := doRequest(t, srv, "POST", "/api/dev/apps/test/undeploy", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	if statusSet != "draft" {
		t.Errorf("status set to %q, want %q", statusSet, "draft")
	}
}

func TestDevValidateSuccess(t *testing.T) {
	appDir := t.TempDir()
	os.WriteFile(filepath.Join(appDir, "app.yml"), []byte("id: test\nname: Test\nversion: '1.0'\ndescription: test\ncategories:\n  - utility\nlxc:\n  ostemplate: debian-12\n  defaults:\n    cores: 1\n    memory_mb: 512\n    disk_gb: 4\n"), 0644)
	os.MkdirAll(filepath.Join(appDir, "provision"), 0755)
	os.WriteFile(filepath.Join(appDir, "provision", "install.py"), []byte("from appstore import BaseApp, run\nclass T(BaseApp):\n    def install(self):\n        pass\nrun(T)\n"), 0644)

	srv := devServer(t, devSvcStub{
		appDirFn: func(id string) string { return appDir },
	}, nil)

	w := doRequest(t, srv, "POST", "/api/dev/apps/test/validate", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestDevValidateNotFound(t *testing.T) {
	srv := devServer(t, devSvcStub{
		appDirFn: func(id string) string { return "/nonexistent/path" },
	}, nil)

	w := doRequest(t, srv, "POST", "/api/dev/apps/test/validate", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestDevListTemplates(t *testing.T) {
	srv := devServer(t, devSvcStub{}, nil)

	w := doRequest(t, srv, "GET", "/api/dev/templates", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	if body["templates"] == nil {
		t.Error("expected templates in response")
	}
}

func TestDevGetFileSuccess(t *testing.T) {
	srv := devServer(t, devSvcStub{
		readFileFn: func(id, relPath string) ([]byte, error) {
			return []byte("file content"), nil
		},
	}, nil)

	w := doRequest(t, srv, "GET", "/api/dev/apps/my-app/file?path=provision/install.py", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	body := decodeJSON(t, w)
	if body["content"] != "file content" {
		t.Errorf("content = %v, want 'file content'", body["content"])
	}
}

func TestDevGetFileNotFound(t *testing.T) {
	srv := devServer(t, devSvcStub{
		readFileFn: func(id, relPath string) ([]byte, error) {
			return nil, fmt.Errorf("file not found")
		},
	}, nil)

	w := doRequest(t, srv, "GET", "/api/dev/apps/my-app/file?path=nonexistent.py", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestDevGetFileMissingPath(t *testing.T) {
	srv := devServer(t, devSvcStub{}, nil)

	w := doRequest(t, srv, "GET", "/api/dev/apps/my-app/file", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDevSaveFileSuccess(t *testing.T) {
	saved := false
	srv := devServer(t, devSvcStub{
		saveFileFn: func(id, relPath string, data []byte) error {
			saved = true
			return nil
		},
	}, nil)

	w := doRequest(t, srv, "PUT", "/api/dev/apps/my-app/file", `{"path":"provision/helper.py","content":"# helper"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}
	if !saved {
		t.Error("expected save to be called")
	}
}

func TestDevSaveFileMissingPath(t *testing.T) {
	srv := devServer(t, devSvcStub{}, nil)

	w := doRequest(t, srv, "PUT", "/api/dev/apps/my-app/file", `{"content":"data"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// --- Reconciliation tests ---

func TestReconcileVersionMatch_AutoUndeploys(t *testing.T) {
	cfg := testConfig()
	cfg.Developer.Enabled = true
	srv := New(cfg, nil, nil, nil)

	removedApps := map[string]bool{}
	statusUpdates := map[string]string{}

	srv.catalogSvc = catalogSvcStub{
		getShadowedFn: func(id string) (*catalog.AppManifest, bool) {
			if id == "plex" {
				return &catalog.AppManifest{ID: "plex", Version: "1.41.0"}, true
			}
			return nil, false
		},
		removeDevAppFn: func(id string) { removedApps[id] = true },
	}
	srv.devSvc = devSvcStub{
		listFn: func() ([]devmode.DevAppMeta, error) {
			return []devmode.DevAppMeta{
				{ID: "plex", Version: "1.41.0", Status: "deployed"},
			}, nil
		},
		setStatusFn: func(id, status string) error {
			statusUpdates[id] = status
			return nil
		},
	}

	merged := srv.ReconcileDevApps()

	if !removedApps["plex"] {
		t.Error("expected plex dev overlay to be removed")
	}
	if statusUpdates["plex"] != "draft" {
		t.Errorf("expected plex status reset to draft, got %q", statusUpdates["plex"])
	}
	if len(merged) != 1 || merged[0] != "plex" {
		t.Errorf("expected merged=[plex], got %v", merged)
	}
}

func TestReconcileNewerCatalog_AutoUndeploys(t *testing.T) {
	cfg := testConfig()
	cfg.Developer.Enabled = true
	srv := New(cfg, nil, nil, nil)

	removedApps := map[string]bool{}

	srv.catalogSvc = catalogSvcStub{
		getShadowedFn: func(id string) (*catalog.AppManifest, bool) {
			if id == "plex" {
				return &catalog.AppManifest{ID: "plex", Version: "2.0.0"}, true
			}
			return nil, false
		},
		removeDevAppFn: func(id string) { removedApps[id] = true },
	}
	srv.devSvc = devSvcStub{
		listFn: func() ([]devmode.DevAppMeta, error) {
			return []devmode.DevAppMeta{
				{ID: "plex", Version: "1.41.0", Status: "deployed"},
			}, nil
		},
		setStatusFn: func(id, status string) error { return nil },
	}

	merged := srv.ReconcileDevApps()

	if !removedApps["plex"] {
		t.Error("expected plex dev overlay to be removed when catalog has newer version")
	}
	if len(merged) != 1 {
		t.Errorf("expected 1 merged, got %d", len(merged))
	}
}

func TestReconcileOlderCatalog_NoUndeploy(t *testing.T) {
	cfg := testConfig()
	cfg.Developer.Enabled = true
	srv := New(cfg, nil, nil, nil)

	removedApps := map[string]bool{}

	srv.catalogSvc = catalogSvcStub{
		getShadowedFn: func(id string) (*catalog.AppManifest, bool) {
			if id == "plex" {
				return &catalog.AppManifest{ID: "plex", Version: "1.40.0"}, true
			}
			return nil, false
		},
		removeDevAppFn: func(id string) { removedApps[id] = true },
	}
	srv.devSvc = devSvcStub{
		listFn: func() ([]devmode.DevAppMeta, error) {
			return []devmode.DevAppMeta{
				{ID: "plex", Version: "1.41.0", Status: "deployed"},
			}, nil
		},
	}

	merged := srv.ReconcileDevApps()

	if removedApps["plex"] {
		t.Error("should NOT undeploy when catalog has older version")
	}
	if len(merged) != 0 {
		t.Errorf("expected 0 merged, got %d", len(merged))
	}
}

func TestReconcileDraftApp_NoUndeploy(t *testing.T) {
	cfg := testConfig()
	cfg.Developer.Enabled = true
	srv := New(cfg, nil, nil, nil)

	removedApps := map[string]bool{}

	srv.catalogSvc = catalogSvcStub{
		getShadowedFn: func(id string) (*catalog.AppManifest, bool) {
			return &catalog.AppManifest{ID: id, Version: "1.41.0"}, true
		},
		removeDevAppFn: func(id string) { removedApps[id] = true },
	}
	srv.devSvc = devSvcStub{
		listFn: func() ([]devmode.DevAppMeta, error) {
			return []devmode.DevAppMeta{
				{ID: "plex", Version: "1.41.0", Status: "draft"},
			}, nil
		},
	}

	merged := srv.ReconcileDevApps()

	if removedApps["plex"] {
		t.Error("should NOT undeploy a draft app")
	}
	if len(merged) != 0 {
		t.Errorf("expected 0 merged, got %d", len(merged))
	}
}

func TestReconcileNoShadow_NoUndeploy(t *testing.T) {
	cfg := testConfig()
	cfg.Developer.Enabled = true
	srv := New(cfg, nil, nil, nil)

	removedApps := map[string]bool{}

	srv.catalogSvc = catalogSvcStub{
		getShadowedFn: func(id string) (*catalog.AppManifest, bool) {
			return nil, false // no shadowed version — brand new dev app
		},
		removeDevAppFn: func(id string) { removedApps[id] = true },
	}
	srv.devSvc = devSvcStub{
		listFn: func() ([]devmode.DevAppMeta, error) {
			return []devmode.DevAppMeta{
				{ID: "my-new-app", Version: "1.0.0", Status: "deployed"},
			}, nil
		},
	}

	merged := srv.ReconcileDevApps()

	if removedApps["my-new-app"] {
		t.Error("should NOT undeploy when no catalog version exists")
	}
	if len(merged) != 0 {
		t.Errorf("expected 0 merged, got %d", len(merged))
	}
}
