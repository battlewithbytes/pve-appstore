package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
	"github.com/battlewithbytes/pve-appstore/internal/config"
	"github.com/battlewithbytes/pve-appstore/internal/devmode"
	"github.com/battlewithbytes/pve-appstore/internal/engine"
	"github.com/battlewithbytes/pve-appstore/internal/pct"
)

// mockCM is a no-op ContainerManager for server tests.
type mockCM struct {
	storagePath string
}

func (m *mockCM) AllocateCTID(ctx context.Context) (int, error)                    { return 100, nil }
func (m *mockCM) Create(ctx context.Context, opts engine.CreateOptions) error      { return nil }
func (m *mockCM) Start(ctx context.Context, ctid int) error                        { return nil }
func (m *mockCM) Stop(ctx context.Context, ctid int) error                         { return nil }
func (m *mockCM) Shutdown(ctx context.Context, ctid int, timeout int) error        { return nil }
func (m *mockCM) Destroy(ctx context.Context, ctid int, keepVolumes ...bool) error { return nil }
func (m *mockCM) Status(ctx context.Context, ctid int) (string, error)             { return "stopped", nil }
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
func (m *mockCM) GetIP(ctid int) (string, error)              { return "10.0.0.1", nil }
func (m *mockCM) GetConfig(ctx context.Context, ctid int) (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}
func (m *mockCM) UpdateConfig(ctx context.Context, ctid int, params url.Values) error {
	return nil
}
func (m *mockCM) DetachMountPoints(ctx context.Context, ctid int, indexes []int) error { return nil }
func (m *mockCM) ConfigureDevices(ctid int, devices []engine.DevicePassthrough) error  { return nil }
func (m *mockCM) MountHostPath(ctid int, mpIndex int, hostPath, containerPath string, readOnly bool) error {
	return nil
}
func (m *mockCM) AppendLXCConfig(ctid int, lines []string) error { return nil }
func (m *mockCM) GetStorageInfo(ctx context.Context, storageID string) (*engine.StorageInfo, error) {
	path := m.storagePath
	if path == "" {
		path = "/tmp/test-storage"
	}
	return &engine.StorageInfo{
		ID:        storageID,
		Type:      "dir",
		Path:      path,
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
	storageDir := filepath.Join(t.TempDir(), "test-storage")
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		t.Fatalf("mkdir storage: %v", err)
	}
	eng, err := engine.New(cfg, cat, dataDir, &mockCM{storagePath: storageDir})
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

func waitForJobTerminalState(t *testing.T, srv *Server, jobID string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		w := doRequest(t, srv, "GET", "/api/jobs/"+jobID, "")
		if w.Code == http.StatusOK {
			body := decodeJSON(t, w)
			if st, _ := body["state"].(string); st == "completed" || st == "failed" || st == "cancelled" {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach terminal state in time", jobID)
}

type catalogSvcStub struct {
	CatalogService
	appCountFn func() int
	listFn     func() []*catalog.AppManifest
	getAppFn   func(id string) (*catalog.AppManifest, bool)
}

func (s catalogSvcStub) AppCount() int {
	if s.appCountFn != nil {
		return s.appCountFn()
	}
	return 0
}

func (s catalogSvcStub) ListApps() []*catalog.AppManifest {
	if s.listFn != nil {
		return s.listFn()
	}
	return nil
}

func (s catalogSvcStub) GetApp(id string) (*catalog.AppManifest, bool) {
	if s.getAppFn != nil {
		return s.getAppFn(id)
	}
	return nil, false
}

type devSvcStub struct {
	DevService
	listFn func() ([]devmode.DevAppMeta, error)
	forkFn func(newID, sourceDir string) error
	getFn  func(id string) (*devmode.DevApp, error)
}

func (s devSvcStub) List() ([]devmode.DevAppMeta, error) {
	if s.listFn != nil {
		return s.listFn()
	}
	return nil, nil
}

func (s devSvcStub) Fork(newID, sourceDir string) error {
	if s.forkFn != nil {
		return s.forkFn(newID, sourceDir)
	}
	return nil
}

func (s devSvcStub) Get(id string) (*devmode.DevApp, error) {
	if s.getFn != nil {
		return s.getFn(id)
	}
	return nil, fmt.Errorf("not found")
}

type engineSvcStub struct {
	EngineService
	listJobsFn           func() ([]*engine.Job, error)
	listInstallsFn       func() ([]*engine.InstallListItem, error)
	startInstallFn       func(req engine.InstallRequest) (*engine.Job, error)
	startStackFn         func(req engine.StackCreateRequest) (*engine.Job, error)
	listRawFn            func() ([]*engine.Install, error)
	listStacksFn         func() ([]*engine.Stack, error)
	listStacksEnrichedFn func() ([]*engine.StackListItem, error)
	getStackDetailFn     func(id string) (*engine.StackDetail, error)
	validateStackFn      func(req engine.StackCreateRequest) map[string]interface{}
}

func (s engineSvcStub) ListJobs() ([]*engine.Job, error) {
	if s.listJobsFn != nil {
		return s.listJobsFn()
	}
	return nil, nil
}

func (s engineSvcStub) ListInstallsEnriched() ([]*engine.InstallListItem, error) {
	if s.listInstallsFn != nil {
		return s.listInstallsFn()
	}
	return nil, nil
}

func (s engineSvcStub) StartInstall(req engine.InstallRequest) (*engine.Job, error) {
	if s.startInstallFn != nil {
		return s.startInstallFn(req)
	}
	return nil, nil
}

func (s engineSvcStub) StartStack(req engine.StackCreateRequest) (*engine.Job, error) {
	if s.startStackFn != nil {
		return s.startStackFn(req)
	}
	return nil, nil
}

func (s engineSvcStub) ListInstalls() ([]*engine.Install, error) {
	if s.listRawFn != nil {
		return s.listRawFn()
	}
	return nil, nil
}

func (s engineSvcStub) ListStacks() ([]*engine.Stack, error) {
	if s.listStacksFn != nil {
		return s.listStacksFn()
	}
	return nil, nil
}

func setEngineServices(srv *Server, svc engineSvcStub) {
	srv.engineInstallSvc = svc
	srv.engineStackSvc = svc
	srv.engineConfigSvc = svc
}

func (s engineSvcStub) ListStacksEnriched() ([]*engine.StackListItem, error) {
	if s.listStacksEnrichedFn != nil {
		return s.listStacksEnrichedFn()
	}
	return nil, nil
}

func (s engineSvcStub) GetStackDetail(id string) (*engine.StackDetail, error) {
	if s.getStackDetailFn != nil {
		return s.getStackDetailFn(id)
	}
	return nil, fmt.Errorf("not found")
}

func (s engineSvcStub) ValidateStack(req engine.StackCreateRequest) map[string]interface{} {
	if s.validateStackFn != nil {
		return s.validateStackFn(req)
	}
	return map[string]interface{}{"valid": false}
}
