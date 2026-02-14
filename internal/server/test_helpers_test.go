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
func (m *mockCM) ListStorages(ctx context.Context) ([]engine.StorageInfo, error) {
	return []engine.StorageInfo{
		{ID: "local-lvm", Type: "lvmthin", Content: "rootdir,images"},
		{ID: "local", Type: "dir", Content: "iso,vztmpl,rootdir", Path: "/var/lib/vz", Browsable: true},
	}, nil
}
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
	appCountFn      func() int
	listFn          func() []*catalog.AppManifest
	getAppFn        func(id string) (*catalog.AppManifest, bool)
	searchFn        func(query string) []*catalog.AppManifest
	categoriesFn    func() []string
	refreshFn       func() error
	mergeDevAppFn   func(app *catalog.AppManifest)
	removeDevAppFn  func(id string)
	listStacksFn    func() []*catalog.StackManifest
	getStackFn      func(id string) (*catalog.StackManifest, bool)
	mergeDevStackFn func(s *catalog.StackManifest)
	removeDevStackFn func(id string)
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

func (s catalogSvcStub) SearchApps(query string) []*catalog.AppManifest {
	if s.searchFn != nil {
		return s.searchFn(query)
	}
	return nil
}

func (s catalogSvcStub) Categories() []string {
	if s.categoriesFn != nil {
		return s.categoriesFn()
	}
	return nil
}

func (s catalogSvcStub) Refresh() error {
	if s.refreshFn != nil {
		return s.refreshFn()
	}
	return nil
}

func (s catalogSvcStub) MergeDevApp(app *catalog.AppManifest) {
	if s.mergeDevAppFn != nil {
		s.mergeDevAppFn(app)
	}
}

func (s catalogSvcStub) RemoveDevApp(id string) {
	if s.removeDevAppFn != nil {
		s.removeDevAppFn(id)
	}
}

func (s catalogSvcStub) ListStacks() []*catalog.StackManifest {
	if s.listStacksFn != nil {
		return s.listStacksFn()
	}
	return nil
}

func (s catalogSvcStub) GetStack(id string) (*catalog.StackManifest, bool) {
	if s.getStackFn != nil {
		return s.getStackFn(id)
	}
	return nil, false
}

func (s catalogSvcStub) MergeDevStack(sm *catalog.StackManifest) {
	if s.mergeDevStackFn != nil {
		s.mergeDevStackFn(sm)
	}
}

func (s catalogSvcStub) RemoveDevStack(id string) {
	if s.removeDevStackFn != nil {
		s.removeDevStackFn(id)
	}
}

type devSvcStub struct {
	DevService
	listFn          func() ([]devmode.DevAppMeta, error)
	forkFn          func(newID, sourceDir string) error
	getFn           func(id string) (*devmode.DevApp, error)
	createFn        func(id, template string) error
	saveManifestFn  func(id string, data []byte) error
	saveScriptFn    func(id string, data []byte) error
	isDeployedFn    func(id string) bool
	parseManifestFn func(id string) (*catalog.AppManifest, error)
	appDirFn        func(id string) string
	saveFileFn      func(id, relPath string, data []byte) error
	readFileFn      func(id, relPath string) ([]byte, error)
	deleteFileFn    func(id, relPath string) error
	deleteFn        func(id string) error
	setStatusFn     func(id, status string) error
	ensureIconFn    func(id string)
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

func (s devSvcStub) Create(id, template string) error {
	if s.createFn != nil {
		return s.createFn(id, template)
	}
	return nil
}

func (s devSvcStub) SaveManifest(id string, data []byte) error {
	if s.saveManifestFn != nil {
		return s.saveManifestFn(id, data)
	}
	return nil
}

func (s devSvcStub) SaveScript(id string, data []byte) error {
	if s.saveScriptFn != nil {
		return s.saveScriptFn(id, data)
	}
	return nil
}

func (s devSvcStub) IsDeployed(id string) bool {
	if s.isDeployedFn != nil {
		return s.isDeployedFn(id)
	}
	return false
}

func (s devSvcStub) ParseManifest(id string) (*catalog.AppManifest, error) {
	if s.parseManifestFn != nil {
		return s.parseManifestFn(id)
	}
	return &catalog.AppManifest{ID: id}, nil
}

func (s devSvcStub) AppDir(id string) string {
	if s.appDirFn != nil {
		return s.appDirFn(id)
	}
	return "/tmp/dev-apps/" + id
}

func (s devSvcStub) SaveFile(id, relPath string, data []byte) error {
	if s.saveFileFn != nil {
		return s.saveFileFn(id, relPath, data)
	}
	return nil
}

func (s devSvcStub) ReadFile(id, relPath string) ([]byte, error) {
	if s.readFileFn != nil {
		return s.readFileFn(id, relPath)
	}
	return nil, fmt.Errorf("not found")
}

func (s devSvcStub) DeleteFile(id, relPath string) error {
	if s.deleteFileFn != nil {
		return s.deleteFileFn(id, relPath)
	}
	return nil
}

func (s devSvcStub) Delete(id string) error {
	if s.deleteFn != nil {
		return s.deleteFn(id)
	}
	return nil
}

func (s devSvcStub) SetStatus(id, status string) error {
	if s.setStatusFn != nil {
		return s.setStatusFn(id, status)
	}
	return nil
}

func (s devSvcStub) SetGitHubMeta(id string, meta map[string]string) error {
	return nil
}

func (s devSvcStub) EnsureIcon(id string) {
	if s.ensureIconFn != nil {
		s.ensureIconFn(id)
	}
}

func (s devSvcStub) ListStacks() ([]devmode.DevStackMeta, error) { return nil, nil }
func (s devSvcStub) CreateStack(id, template string) error       { return nil }
func (s devSvcStub) GetStack(id string) (*devmode.DevStack, error) {
	return nil, fmt.Errorf("not found")
}
func (s devSvcStub) SaveStackManifest(id string, data []byte) error          { return nil }
func (s devSvcStub) ParseStackManifest(id string) (*catalog.StackManifest, error) {
	return &catalog.StackManifest{ID: id}, nil
}
func (s devSvcStub) DeleteStack(id string) error         { return nil }
func (s devSvcStub) SetStackStatus(id, status string) error { return nil }
func (s devSvcStub) IsStackDeployed(id string) bool      { return false }

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
	cancelJobFn          func(id string) error
	clearJobsFn          func() (int64, error)
	getJobFn             func(id string) (*engine.Job, error)
	startContainerFn     func(id string) error
	stopContainerFn      func(id string) error
	restartContainerFn   func(id string) error
	uninstallFn          func(id string, keepVolumes bool) (*engine.Job, error)
	purgeInstallFn       func(id string) error
	getInstallFn         func(id string) (*engine.Install, error)
	getInstallDetailFn   func(id string) (*engine.InstallDetail, error)
	startStackContFn     func(id string) error
	stopStackContFn      func(id string) error
	restartStackContFn   func(id string) error
	uninstallStackFn     func(id string) (*engine.Job, error)
	getStackFn           func(id string) (*engine.Stack, error)
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

func (s engineSvcStub) HasActiveInstallForApp(appID string) (*engine.Install, bool) {
	return nil, false
}

func (s engineSvcStub) HasActiveDevInstallForApp(appID string) (*engine.Install, bool) {
	return nil, false
}

func (s engineSvcStub) HasActiveJobForApp(appID string) (*engine.Job, bool) {
	return nil, false
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

func (s engineSvcStub) CancelJob(id string) error {
	if s.cancelJobFn != nil {
		return s.cancelJobFn(id)
	}
	return nil
}

func (s engineSvcStub) ClearJobs() (int64, error) {
	if s.clearJobsFn != nil {
		return s.clearJobsFn()
	}
	return 0, nil
}

func (s engineSvcStub) GetJob(id string) (*engine.Job, error) {
	if s.getJobFn != nil {
		return s.getJobFn(id)
	}
	return nil, fmt.Errorf("not found")
}

func (s engineSvcStub) StartContainer(id string) error {
	if s.startContainerFn != nil {
		return s.startContainerFn(id)
	}
	return nil
}

func (s engineSvcStub) StopContainer(id string) error {
	if s.stopContainerFn != nil {
		return s.stopContainerFn(id)
	}
	return nil
}

func (s engineSvcStub) RestartContainer(id string) error {
	if s.restartContainerFn != nil {
		return s.restartContainerFn(id)
	}
	return nil
}

func (s engineSvcStub) Uninstall(id string, keepVolumes bool) (*engine.Job, error) {
	if s.uninstallFn != nil {
		return s.uninstallFn(id, keepVolumes)
	}
	return nil, fmt.Errorf("not found")
}

func (s engineSvcStub) PurgeInstall(id string) error {
	if s.purgeInstallFn != nil {
		return s.purgeInstallFn(id)
	}
	return nil
}

func (s engineSvcStub) GetInstall(id string) (*engine.Install, error) {
	if s.getInstallFn != nil {
		return s.getInstallFn(id)
	}
	return nil, fmt.Errorf("not found")
}

func (s engineSvcStub) GetInstallDetail(id string) (*engine.InstallDetail, error) {
	if s.getInstallDetailFn != nil {
		return s.getInstallDetailFn(id)
	}
	return nil, fmt.Errorf("not found")
}

func (s engineSvcStub) StartStackContainer(id string) error {
	if s.startStackContFn != nil {
		return s.startStackContFn(id)
	}
	return nil
}

func (s engineSvcStub) StopStackContainer(id string) error {
	if s.stopStackContFn != nil {
		return s.stopStackContFn(id)
	}
	return nil
}

func (s engineSvcStub) RestartStackContainer(id string) error {
	if s.restartStackContFn != nil {
		return s.restartStackContFn(id)
	}
	return nil
}

func (s engineSvcStub) UninstallStack(id string) (*engine.Job, error) {
	if s.uninstallStackFn != nil {
		return s.uninstallStackFn(id)
	}
	return nil, fmt.Errorf("not found")
}

func (s engineSvcStub) GetStack(id string) (*engine.Stack, error) {
	if s.getStackFn != nil {
		return s.getStackFn(id)
	}
	return nil, fmt.Errorf("not found")
}
