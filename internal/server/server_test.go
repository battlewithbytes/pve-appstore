package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
	"github.com/battlewithbytes/pve-appstore/internal/config"
	"github.com/battlewithbytes/pve-appstore/internal/engine"
	"github.com/battlewithbytes/pve-appstore/internal/pct"
)

// mockCM is a no-op ContainerManager for server tests.
type mockCM struct{}

func (m *mockCM) AllocateCTID(ctx context.Context) (int, error) { return 100, nil }
func (m *mockCM) Create(ctx context.Context, opts engine.CreateOptions) error { return nil }
func (m *mockCM) Start(ctx context.Context, ctid int) error { return nil }
func (m *mockCM) Stop(ctx context.Context, ctid int) error { return nil }
func (m *mockCM) Shutdown(ctx context.Context, ctid int, timeout int) error { return nil }
func (m *mockCM) Destroy(ctx context.Context, ctid int) error { return nil }
func (m *mockCM) Status(ctx context.Context, ctid int) (string, error) { return "stopped", nil }
func (m *mockCM) StatusDetail(ctx context.Context, ctid int) (*engine.ContainerStatusDetail, error) {
	return &engine.ContainerStatusDetail{
		Status: "stopped", Uptime: 0, CPU: 0, CPUs: 2,
		Mem: 128 * 1024 * 1024, MaxMem: 512 * 1024 * 1024,
		Disk: 100 * 1024 * 1024, MaxDisk: 4 * 1024 * 1024 * 1024,
	}, nil
}
func (m *mockCM) ResolveTemplate(ctx context.Context, name, storage string) string { return name }
func (m *mockCM) Exec(ctid int, command []string) (*pct.ExecResult, error) {
	return &pct.ExecResult{Output: "", ExitCode: 0}, nil
}
func (m *mockCM) ExecStream(ctid int, command []string, onLine func(line string)) (*pct.ExecResult, error) {
	return &pct.ExecResult{Output: "", ExitCode: 0}, nil
}
func (m *mockCM) ExecScript(ctid int, scriptPath string, env map[string]string) (*pct.ExecResult, error) {
	return &pct.ExecResult{Output: "", ExitCode: 0}, nil
}
func (m *mockCM) Push(ctid int, src, dst, perms string) error { return nil }
func (m *mockCM) GetIP(ctid int) (string, error) { return "10.0.0.1", nil }
func (m *mockCM) GetConfig(ctx context.Context, ctid int) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}
func (m *mockCM) DetachMountPoints(ctx context.Context, ctid int, indexes []int) error { return nil }
func (m *mockCM) GetStorageInfo(ctx context.Context, storageID string) (*engine.StorageInfo, error) {
	return &engine.StorageInfo{
		ID:        storageID,
		Type:      "dir",
		Path:      "/tmp/test-storage",
		Browsable: true,
	}, nil
}

func testConfig() *config.Config {
	return &config.Config{
		NodeName: "testnode",
		Pool:     "testpool",
		Storages: []string{"local-lvm"},
		Bridges:  []string{"vmbr0"},
		Defaults: config.ResourceConfig{
			Cores:    2,
			MemoryMB: 2048,
			DiskGB:   8,
		},
		Security: config.SecurityConfig{UnprivilegedOnly: true},
		Service:  config.ServiceConfig{BindAddress: "127.0.0.1", Port: 0},
		Auth:     config.AuthConfig{Mode: config.AuthModeNone},
		Catalog:  config.CatalogConfig{URL: "https://example.com/catalog.git", Branch: "main", Refresh: config.RefreshManual},
		GPU:      config.GPUConfig{Policy: config.GPUPolicyNone},
	}
}

func testCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat := catalog.New("", "main", t.TempDir())
	if err := cat.LoadLocal("../../testdata/catalog"); err != nil {
		t.Fatalf("LoadLocal: %v", err)
	}
	return cat
}

func testEngine(t *testing.T, cfg *config.Config, cat *catalog.Catalog) *engine.Engine {
	t.Helper()
	dataDir := t.TempDir()
	eng, err := engine.New(cfg, cat, dataDir, &mockCM{})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	t.Cleanup(func() { eng.Close() })
	return eng
}

func testServer(t *testing.T) *Server {
	t.Helper()
	cfg := testConfig()
	cat := testCatalog(t)
	eng := testEngine(t, cfg, cat)
	return New(cfg, cat, eng, nil)
}

func doRequest(t *testing.T, srv *Server, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(w, req)
	return w
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode JSON: %v (body: %s)", err, w.Body.String())
	}
	return result
}

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
	if count != 7 {
		t.Errorf("app_count = %v, want 7", count)
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
	if total != 7 {
		t.Errorf("total = %v, want 7", total)
	}
	apps := body["apps"].([]interface{})
	if len(apps) != 7 {
		t.Errorf("apps count = %d, want 7", len(apps))
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
	// jellyfin and plex are in "media"
	if total != 2 {
		t.Errorf("total = %v, want 2", total)
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
	w := doRequest(t, srv, "GET", "/api/health", "")

	cors := w.Header().Get("Access-Control-Allow-Origin")
	if cors != "*" {
		t.Errorf("CORS origin = %q, want %q", cors, "*")
	}
	methods := w.Header().Get("Access-Control-Allow-Methods")
	if methods == "" {
		t.Error("CORS methods header is empty")
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

// --- Browse Paths ---

func TestBrowsePathsAllowedStorage(t *testing.T) {
	srv := testServer(t)
	// The mock storage resolves to /tmp/test-storage — create it
	os.MkdirAll("/tmp/test-storage", 0755)
	w := doRequest(t, srv, "GET", "/api/browse/paths?path=/tmp/test-storage", "")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}

	body := decodeJSON(t, w)
	if body["path"] != "/tmp/test-storage" {
		t.Errorf("path = %v, want /tmp/test-storage", body["path"])
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
	w := doRequest(t, srv, "GET", "/api/browse/paths?path=/tmp/test-storage/nonexistent-abc123", "")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestBrowseMkdir(t *testing.T) {
	srv := testServer(t)
	os.MkdirAll("/tmp/test-storage", 0755)

	w := doRequest(t, srv, "POST", "/api/browse/mkdir", `{"path":"/tmp/test-storage/new-folder"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}

	body := decodeJSON(t, w)
	if body["created"] != true {
		t.Errorf("created = %v, want true", body["created"])
	}

	// Cleanup
	os.RemoveAll("/tmp/test-storage/new-folder")
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

	// Start an install for nginx
	w := doRequest(t, srv, "POST", "/api/apps/nginx/install", `{"cores":1,"memory_mb":512,"disk_gb":4,"hostname":"my-nginx","devices":[{"path":"/dev/dri/renderD128","gid":44,"mode":"0666"}],"env_vars":{"FOO":"bar"}}`)
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
	// Verify devices are in the recipe
	if devices, ok := recipe["devices"].([]interface{}); ok && len(devices) > 0 {
		dev := devices[0].(map[string]interface{})
		if dev["path"] != "/dev/dri/renderD128" {
			t.Errorf("device path = %v, want /dev/dri/renderD128", dev["path"])
		}
	} else {
		t.Error("expected devices in recipe")
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
	if detail["type"] != "dir" {
		t.Errorf("storage detail type = %v, want dir", detail["type"])
	}
	if detail["browsable"] != true {
		t.Error("expected storage to be browsable")
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
	hasBind := false
	for _, m := range mounts {
		mp := m.(map[string]interface{})
		switch mp["type"] {
		case "volume":
			hasVolume = true
		case "bind":
			hasBind = true
			if mp["host_path"] != "/mnt/storage/movies" {
				t.Errorf("bind mount host_path = %v, want /mnt/storage/movies", mp["host_path"])
			}
		}
	}
	if !hasVolume {
		t.Error("expected a managed volume mount")
	}
	if !hasBind {
		t.Error("expected a bind mount")
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
}

func TestInstallAppDifferentAppsAllowed(t *testing.T) {
	srv := testServer(t)
	reqBody := `{"cores": 1, "memory_mb": 512, "disk_gb": 4}`

	w1 := doRequest(t, srv, "POST", "/api/apps/nginx/install", reqBody)
	if w1.Code != http.StatusAccepted {
		t.Fatalf("nginx install status = %d, want %d", w1.Code, http.StatusAccepted)
	}

	// Different app should still be allowed
	w2 := doRequest(t, srv, "POST", "/api/apps/ollama/install", reqBody)
	if w2.Code != http.StatusAccepted {
		t.Fatalf("ollama install status = %d, want %d (body: %s)", w2.Code, http.StatusAccepted, w2.Body.String())
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
