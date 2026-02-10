package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
	"github.com/battlewithbytes/pve-appstore/internal/config"
	"github.com/battlewithbytes/pve-appstore/internal/engine"
	"github.com/battlewithbytes/pve-appstore/internal/proxmox"
	"github.com/battlewithbytes/pve-appstore/internal/ui"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	taConfigPath  string
	taCatalogDir  string
	taDataDir     string
	taTimeout     int
	taConcurrency int
	taKeep        bool
	taApp         string
	taVerbose     bool
)

func init() {
	testAppsCmd.Flags().StringVar(&taConfigPath, "config", config.DefaultConfigPath, "path to config file")
	testAppsCmd.Flags().StringVar(&taCatalogDir, "catalog-dir", "", "load catalog from local directory")
	testAppsCmd.Flags().StringVar(&taDataDir, "data-dir", "", "data directory (default: auto temp dir)")
	testAppsCmd.Flags().IntVar(&taTimeout, "timeout", 15, "per-app timeout in minutes")
	testAppsCmd.Flags().IntVar(&taConcurrency, "concurrency", 0, "max parallel installs (0 = unlimited)")
	testAppsCmd.Flags().BoolVar(&taKeep, "keep", false, "skip cleanup, keep containers for debugging")
	testAppsCmd.Flags().StringVar(&taApp, "app", "", "test a single app by ID")
	testAppsCmd.Flags().BoolVar(&taVerbose, "verbose", false, "stream job logs to stdout")
	rootCmd.AddCommand(testAppsCmd)
}

type testResult struct {
	AppID    string
	AppName  string
	Status   string // "PASS" or "FAIL"
	Duration time.Duration
	CTID     int
	Error    string
	JobID    string
	Index    int
}

// testAppConfig holds per-app test overrides loaded from test.yml.
// Place a test.yml in the app directory (alongside app.yml) to configure
// how the app should be tested without changing code.
type testAppConfig struct {
	Skip           bool              `yaml:"skip"`             // skip this app entirely
	SkipBindMounts bool              `yaml:"skip_bind_mounts"` // don't mount bind volumes
	Inputs         map[string]string `yaml:"inputs"`           // override input values
	MemoryMB       int               `yaml:"memory_mb"`        // override memory
	DiskGB         int               `yaml:"disk_gb"`          // override disk
	Cores          int               `yaml:"cores"`            // override cores
}

// loadTestConfig reads test.yml from the app directory if it exists.
func loadTestConfig(appDirPath string) testAppConfig {
	var cfg testAppConfig
	data, err := os.ReadFile(filepath.Join(appDirPath, "test.yml"))
	if err != nil {
		return cfg // no test config, use defaults
	}
	yaml.Unmarshal(data, &cfg)
	return cfg
}

var testAppsCmd = &cobra.Command{
	Use:   "test-apps",
	Short: "Provision, verify, and clean up all catalog apps in parallel",
	Long: `Runs a full integration test of every app in the catalog by actually
provisioning real LXC containers on this Proxmox host, verifying they
reach running state, then cleaning up. Uses an isolated temp database
so production data is never touched.`,
	RunE: runTestApps,
}

func runTestApps(cmd *cobra.Command, args []string) error {
	totalStart := time.Now()

	// 1. Load config
	cfg, err := config.Load(taConfigPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// 2. Initialize catalog
	cat := catalog.New(cfg.Catalog.URL, cfg.Catalog.Branch, config.DefaultDataDir)
	if taCatalogDir != "" {
		if err := cat.LoadLocal(taCatalogDir); err != nil {
			return fmt.Errorf("loading local catalog: %w", err)
		}
	} else {
		if err := cat.Refresh(); err != nil {
			return fmt.Errorf("refreshing catalog: %w", err)
		}
	}

	// 3. Resolve app list
	allApps := cat.List()
	var apps []*catalog.AppManifest
	if taApp != "" {
		for _, a := range allApps {
			if a.ID == taApp {
				apps = append(apps, a)
				break
			}
		}
		if len(apps) == 0 {
			return fmt.Errorf("app %q not found in catalog", taApp)
		}
	} else {
		apps = allApps
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i].ID < apps[j].ID })

	if len(apps) == 0 {
		return fmt.Errorf("no apps found in catalog")
	}

	// 4. Create isolated temp data dir
	dataDir := taDataDir
	if dataDir == "" {
		d, err := os.MkdirTemp("/tmp", "pve-appstore-test-")
		if err != nil {
			return fmt.Errorf("creating temp dir: %w", err)
		}
		dataDir = d
	} else {
		os.MkdirAll(dataDir, 0750)
	}

	// 5. Initialize Proxmox client & engine
	pmClient, err := proxmox.NewClient(proxmox.ClientConfig{
		BaseURL:       cfg.Proxmox.BaseURL,
		Node:          cfg.NodeName,
		TokenID:       cfg.Proxmox.TokenID,
		TokenSecret:   cfg.Proxmox.TokenSecret,
		TLSSkipVerify: cfg.Proxmox.TLSSkipVerify,
		TLSCACertPath: cfg.Proxmox.TLSCACertPath,
	})
	if err != nil {
		return fmt.Errorf("creating proxmox client: %w", err)
	}
	cm := proxmox.NewManager(pmClient)

	// Disable GPU passthrough for tests — device passthrough requires root@pam
	// which the API token may not have.
	cfg.GPU.Policy = config.GPUPolicyNone

	eng, err := engine.New(cfg, cat, dataDir, cm)
	if err != nil {
		return fmt.Errorf("initializing engine: %w", err)
	}
	defer eng.Close()

	// 6. Concurrency setup
	concurrency := taConcurrency
	if concurrency <= 0 {
		concurrency = len(apps)
	}
	timeout := time.Duration(taTimeout) * time.Minute

	// Print banner
	concStr := "unlimited"
	if taConcurrency > 0 {
		concStr = fmt.Sprintf("%d", taConcurrency)
	}
	fmt.Println(ui.Cyan.Render("============================================================="))
	fmt.Println(ui.White.Render("  PVE App Store — Integration Test"))
	fmt.Printf("  %d app(s) | concurrency: %s | timeout: %s\n", len(apps), concStr, timeout)
	fmt.Printf("  node: %s | data: %s\n", cfg.NodeName, dataDir)
	fmt.Println(ui.Cyan.Render("============================================================="))
	fmt.Println()

	// 7. Signal handling
	var interrupted int32
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	var activeJobsMu sync.Mutex
	activeJobs := make(map[string]string) // jobID -> appID

	go func() {
		<-sigCh
		atomic.StoreInt32(&interrupted, 1)
		fmt.Println(ui.Red.Render("\nInterrupted! Cancelling jobs..."))
		activeJobsMu.Lock()
		for jobID := range activeJobs {
			eng.CancelJob(jobID)
		}
		activeJobsMu.Unlock()
	}()

	// 8. Start installs sequentially (to avoid CTID allocation race),
	//    then poll all in parallel. Proxmox's /cluster/nextid returns the
	//    same ID if called before the previous container is created.
	sem := make(chan struct{}, concurrency)
	results := make([]testResult, len(apps))
	type activeJob struct {
		jobID string
		appID string
		name  string
		idx   int
		start time.Time
	}
	var startedJobs []activeJob

	fmt.Println(ui.Dim.Render("Starting installs..."))
	for i, app := range apps {
		if atomic.LoadInt32(&interrupted) != 0 {
			break
		}

		prefix := fmt.Sprintf("[%d/%d] %s", i+1, len(apps), app.ID)

		// Load per-app test config
		tc := loadTestConfig(app.DirPath)
		if tc.Skip {
			fmt.Printf("%s: %s\n", prefix, ui.Dim.Render("skipped (test.yml)"))
			results[i] = testResult{
				AppID:   app.ID,
				AppName: app.Name,
				Status:  "PASS",
				Index:   i,
			}
			continue
		}

		fmt.Printf("%s: %s\n", prefix, ui.Dim.Render("starting..."))

		onboot := false
		req := engine.InstallRequest{
			AppID:     app.ID,
			OnBoot:    &onboot,
			ExtraTags: "testing",
			Inputs:    tc.Inputs,
		}
		if tc.MemoryMB > 0 {
			req.MemoryMB = tc.MemoryMB
		}
		if tc.DiskGB > 0 {
			req.DiskGB = tc.DiskGB
		}
		if tc.Cores > 0 {
			req.Cores = tc.Cores
		}
		if tc.SkipBindMounts {
			// Set empty host paths for all bind volumes to prevent engine from using defaults
			req.BindMounts = make(map[string]string)
			for _, vol := range app.Volumes {
				if vol.Type == "bind" {
					req.BindMounts[vol.Name] = "" // explicit empty clears default
				}
			}
		}

		job, err := eng.StartInstall(req)
		if err != nil {
			results[i] = testResult{
				AppID:   app.ID,
				AppName: app.Name,
				Status:  "FAIL",
				Error:   err.Error(),
				Index:   i,
			}
			fmt.Printf("%s: %s — %s\n", prefix, ui.Red.Render("FAIL"), err.Error())
			continue
		}

		activeJobsMu.Lock()
		activeJobs[job.ID] = app.ID
		activeJobsMu.Unlock()

		startedJobs = append(startedJobs, activeJob{
			jobID: job.ID,
			appID: app.ID,
			name:  app.Name,
			idx:   i,
			start: time.Now(),
		})

		// Wait for CTID allocation before starting next install.
		// Poll until the job has a CTID assigned (typically < 3s).
		ctidDeadline := time.After(15 * time.Second)
		for {
			select {
			case <-ctidDeadline:
				goto nextApp
			case <-time.After(500 * time.Millisecond):
			}
			j, err := eng.GetJob(job.ID)
			if err != nil {
				continue
			}
			if j.CTID > 0 || j.CompletedAt != nil {
				break
			}
		}
	nextApp:
	}

	fmt.Println()
	fmt.Println(ui.Dim.Render("All installs started. Waiting for completion..."))
	fmt.Println()

	// Poll all jobs in parallel
	var wg sync.WaitGroup
	for _, aj := range startedJobs {
		wg.Add(1)
		go func(aj activeJob) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			result := waitAndVerify(eng, cm, aj.jobID, aj.appID, aj.name, aj.idx, len(apps), timeout, aj.start, &activeJobsMu, activeJobs, taVerbose)
			results[aj.idx] = result
		}(aj)
	}

	wg.Wait()

	// 9. Cleanup phase
	if !taKeep {
		fmt.Println()
		fmt.Println(ui.Dim.Render("Cleaning up test containers..."))
		cleanupResults(eng, cm, results, timeout)
	} else {
		fmt.Println()
		fmt.Println(ui.Dim.Render("--keep set: skipping cleanup. Containers are still running."))
	}

	// 10. Clean up temp data dir
	if !taKeep && taDataDir == "" {
		os.RemoveAll(dataDir)
	}

	// 11. Summary
	totalDuration := time.Since(totalStart)
	printTestSummary(results, totalDuration)

	// 12. Exit code
	for _, r := range results {
		if r.Status == "FAIL" {
			return fmt.Errorf("some apps failed")
		}
	}
	return nil
}

// waitAndVerify polls a started job until completion, verifies the container is running, and returns the result.
func waitAndVerify(
	eng *engine.Engine,
	cm engine.ContainerManager,
	jobID, appID, appName string,
	index, total int,
	timeout time.Duration,
	start time.Time,
	activeJobsMu *sync.Mutex,
	activeJobs map[string]string,
	verbose bool,
) testResult {
	prefix := fmt.Sprintf("[%d/%d] %s", index+1, total, appID)
	result := testResult{
		AppID:   appID,
		AppName: appName,
		JobID:   jobID,
		Index:   index,
	}

	fmt.Printf("%s: %s\n", prefix, ui.Dim.Render("installing..."))

	defer func() {
		activeJobsMu.Lock()
		delete(activeJobs, jobID)
		activeJobsMu.Unlock()
	}()

	// Poll for completion
	deadline := time.After(timeout)
	var lastLogID int

	for {
		select {
		case <-deadline:
			eng.CancelJob(jobID)
			result.Status = "FAIL"
			result.Duration = time.Since(start)
			result.Error = "timeout"
			fmt.Printf("%s: %s (%s) — %s\n", prefix, ui.Red.Render("FAIL"), fmtDur(result.Duration), "timeout")
			return result
		case <-time.After(3 * time.Second):
		}

		current, err := eng.GetJob(jobID)
		if err != nil {
			continue
		}

		// Stream logs in verbose mode
		if verbose {
			logs, newLastID, err := eng.GetLogsSince(jobID, lastLogID)
			if err == nil {
				lastLogID = newLastID
				for _, log := range logs {
					fmt.Printf("  %s %s %s\n", ui.Dim.Render(appID), logLevelColor(log.Level), log.Message)
				}
			}
		}

		if current.CTID > 0 {
			result.CTID = current.CTID
		}

		if current.CompletedAt == nil {
			continue
		}

		// Job finished
		result.Duration = time.Since(start)

		if current.State == engine.StateFailed {
			result.Status = "FAIL"
			result.Error = current.Error
			ctidStr := ""
			if result.CTID > 0 {
				ctidStr = fmt.Sprintf(", CTID %d", result.CTID)
			}
			fmt.Printf("%s: %s (%s%s) — %s\n", prefix, ui.Red.Render("FAIL"), fmtDur(result.Duration), ctidStr, result.Error)
			return result
		}

		if current.State == engine.StateCancelled {
			result.Status = "FAIL"
			result.Error = "cancelled"
			fmt.Printf("%s: %s (%s) — %s\n", prefix, ui.Red.Render("FAIL"), fmtDur(result.Duration), "cancelled")
			return result
		}

		// Verify container is running
		if result.CTID > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			sd, err := cm.StatusDetail(ctx, result.CTID)
			cancel()
			if err != nil {
				result.Status = "FAIL"
				result.Error = fmt.Sprintf("status check: %v", err)
				fmt.Printf("%s: %s (%s, CTID %d) — %s\n", prefix, ui.Red.Render("FAIL"), fmtDur(result.Duration), result.CTID, result.Error)
				return result
			}
			if sd.Status != "running" {
				result.Status = "FAIL"
				result.Error = fmt.Sprintf("container status: %s", sd.Status)
				fmt.Printf("%s: %s (%s, CTID %d) — %s\n", prefix, ui.Red.Render("FAIL"), fmtDur(result.Duration), result.CTID, result.Error)
				return result
			}
		}

		result.Status = "PASS"
		fmt.Printf("%s: %s (%s, CTID %d)\n", prefix, ui.Green.Render("PASS"), fmtDur(result.Duration), result.CTID)
		return result
	}
}

func cleanupResults(eng *engine.Engine, cm engine.ContainerManager, results []testResult, timeout time.Duration) {
	var wg sync.WaitGroup
	for _, r := range results {
		if r.CTID == 0 && r.JobID == "" {
			continue // nothing to clean up
		}
		wg.Add(1)
		go func(r testResult) {
			defer wg.Done()

			// Try engine uninstall first (handles stop + destroy + DB cleanup)
			if r.JobID != "" {
				unJob, err := eng.Uninstall(r.JobID, false)
				if err == nil {
					// Wait for uninstall to complete
					deadline := time.After(5 * time.Minute)
					for {
						select {
						case <-deadline:
							goto directCleanup
						case <-time.After(2 * time.Second):
						}
						current, err := eng.GetJob(unJob.ID)
						if err != nil {
							break
						}
						if current.CompletedAt != nil {
							return // successfully uninstalled
						}
					}
				}
			}

		directCleanup:
			// Fallback: directly stop + destroy the container via Proxmox API.
			// This handles cases where the job failed before creating an install record.
			if r.CTID > 0 {
				ctx := context.Background()
				_ = cm.Stop(ctx, r.CTID)
				time.Sleep(2 * time.Second)
				_ = cm.Destroy(ctx, r.CTID)
			}
		}(r)
	}
	wg.Wait()
	fmt.Println(ui.Dim.Render("Cleanup complete."))
}

func printTestSummary(results []testResult, totalDuration time.Duration) {
	fmt.Println()
	fmt.Println(ui.Cyan.Render("============================================================="))
	fmt.Println(ui.White.Render("  RESULTS"))
	fmt.Println(ui.Dim.Render("-------------------------------------------------------------"))
	fmt.Printf("  %-22s %-8s %-8s %-12s %s\n",
		ui.Dim.Render("APP"), ui.Dim.Render("STATUS"), ui.Dim.Render("CTID"), ui.Dim.Render("DURATION"), ui.Dim.Render("ERROR"))

	passed, failed := 0, 0
	for _, r := range results {
		status := ui.Green.Render("PASS")
		if r.Status == "FAIL" {
			status = ui.Red.Render("FAIL")
			failed++
		} else {
			passed++
		}

		ctidStr := "-"
		if r.CTID > 0 {
			ctidStr = fmt.Sprintf("%d", r.CTID)
		}

		errStr := ""
		if r.Error != "" {
			errStr = ui.Red.Render(truncate(r.Error, 40))
		}

		fmt.Printf("  %-22s %-8s %-8s %-12s %s\n",
			ui.White.Render(truncate(r.AppName, 20)),
			status,
			ctidStr,
			fmtDur(r.Duration),
			errStr,
		)
	}

	fmt.Println(ui.Dim.Render("-------------------------------------------------------------"))
	summary := fmt.Sprintf("  %d/%d passed", passed, passed+failed)
	if failed > 0 {
		summary += fmt.Sprintf(" | %s", ui.Red.Render(fmt.Sprintf("%d failed", failed)))
	}
	summary += fmt.Sprintf(" | %s total", fmtDur(totalDuration))
	fmt.Println(summary)
	fmt.Println(ui.Cyan.Render("============================================================="))
}

func fmtDur(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	d = d.Round(time.Second)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func logLevelColor(level string) string {
	switch strings.ToLower(level) {
	case "error":
		return ui.Red.Render("[ERR]")
	case "warn":
		return ui.Red.Render("[WRN]")
	default:
		return ui.Dim.Render("[INF]")
	}
}
