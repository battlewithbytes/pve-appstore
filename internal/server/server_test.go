package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
func (m *mockCM) ResolveTemplate(ctx context.Context, name, storage string) string { return name }
func (m *mockCM) Exec(ctid int, command []string) (*pct.ExecResult, error) {
	return &pct.ExecResult{Output: "", ExitCode: 0}, nil
}
func (m *mockCM) ExecScript(ctid int, scriptPath string, env map[string]string) (*pct.ExecResult, error) {
	return &pct.ExecResult{Output: "", ExitCode: 0}, nil
}
func (m *mockCM) Push(ctid int, src, dst, perms string) error { return nil }
func (m *mockCM) GetIP(ctid int) (string, error) { return "10.0.0.1", nil }

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
