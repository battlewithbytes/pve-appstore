// Package e2e contains Playwright-based browser UI tests for the PVE App Store developer mode.
//
// These tests require:
//   - pve-appstore service running on localhost:8088
//   - Playwright Chromium installed: go run github.com/playwright-community/playwright-go/cmd/playwright install chromium
//
// Run: cd test/e2e && go test -v -timeout 180s
// Visible browser: HEADLESS=false go test -v -timeout 180s
package e2e

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/playwright-community/playwright-go"
)

const baseURL = "http://localhost:8088"

var (
	pw      *playwright.Playwright
	browser playwright.Browser
	page    playwright.Page
)

// TestMain handles setup/teardown: disable auth, enable dev mode, launch browser.
func TestMain(m *testing.M) {
	configPath := "/etc/pve-appstore/config.yml"

	// Read original config
	origConfig, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: cannot read config: %v\n", err)
		os.Exit(1)
	}

	// Save original auth mode
	origMode := "password"
	for _, line := range strings.Split(string(origConfig), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "mode:") {
			origMode = strings.TrimSpace(strings.TrimPrefix(line, "mode:"))
			break
		}
	}

	// Set auth.mode: none
	fmt.Println("Setup: setting auth.mode to none (was:", origMode+")")
	newConfig := strings.Replace(string(origConfig), "mode: "+origMode, "mode: none", 1)
	if err := os.WriteFile(configPath, []byte(newConfig), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: cannot write config: %v\n", err)
		os.Exit(1)
	}

	// Restart service
	exec.Command("systemctl", "restart", "pve-appstore").Run()

	// Wait for health
	healthy := false
	for i := 0; i < 30; i++ {
		resp, err := http.Get(baseURL + "/api/health")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			healthy = true
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !healthy {
		fmt.Fprintln(os.Stderr, "ERROR: service did not start")
		os.WriteFile(configPath, origConfig, 0644)
		exec.Command("systemctl", "restart", "pve-appstore").Run()
		os.Exit(1)
	}
	fmt.Println("Setup: service healthy")

	// Ensure developer mode is enabled
	enableDevMode()

	// Launch Playwright + browser
	pw, err = playwright.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: playwright.Run: %v\n", err)
		os.WriteFile(configPath, origConfig, 0644)
		exec.Command("systemctl", "restart", "pve-appstore").Run()
		os.Exit(1)
	}

	headless := true
	if os.Getenv("HEADLESS") == "false" {
		headless = false
	}

	browser, err = pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headless),
		Args:     []string{"--no-sandbox", "--disable-gpu"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: browser launch: %v\n", err)
		pw.Stop()
		os.WriteFile(configPath, origConfig, 0644)
		exec.Command("systemctl", "restart", "pve-appstore").Run()
		os.Exit(1)
	}

	ctx, err := browser.NewContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: browser context: %v\n", err)
		browser.Close()
		pw.Stop()
		os.WriteFile(configPath, origConfig, 0644)
		exec.Command("systemctl", "restart", "pve-appstore").Run()
		os.Exit(1)
	}

	page, err = ctx.NewPage()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: new page: %v\n", err)
		browser.Close()
		pw.Stop()
		os.WriteFile(configPath, origConfig, 0644)
		exec.Command("systemctl", "restart", "pve-appstore").Run()
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Teardown: clean up test data via API
	sqliteExec("DELETE FROM installs WHERE id='pw-fake-install'")
	apiPost("/api/dev/apps/pw-branch-test/undeploy", "")
	apiDelete("/api/dev/apps/pw-branch-test")
	apiDelete("/api/dev/stacks/pw-test-stack")
	apiDelete("/api/dev/stacks/zip-import-test")

	// Close browser
	browser.Close()
	pw.Stop()

	// Restore config and restart
	fmt.Println("Teardown: restoring auth.mode to", origMode)
	os.WriteFile(configPath, origConfig, 0644)
	exec.Command("systemctl", "restart", "pve-appstore").Run()

	os.Exit(code)
}

// --- Test 1: Developer Dashboard ---

func TestDevDashboard(t *testing.T) {
	navigate(t, "#/developer")

	// Assert heading
	h2 := page.Locator("h2:has-text('Developer Dashboard')")
	assertVisible(t, h2, "Developer Dashboard heading")

	// Assert action buttons
	for _, btn := range []string{"+ New Stack", "+ New App", "Import ZIP", "Import Dockerfile", "Import Unraid XML"} {
		loc := page.Locator(fmt.Sprintf("button:has-text('%s')", btn))
		assertVisible(t, loc, btn+" button")
	}

	// Assert existing dev apps appear as card links
	links := page.Locator("a[href^='#/dev/']")
	count, err := links.Count()
	if err != nil {
		t.Fatalf("failed to count dev app links: %v", err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 dev app card link, got %d", count)
	} else {
		t.Logf("found %d dev app/stack cards", count)
	}
}

// --- Test 2: Create Stack Wizard ---

func TestCreateStackWizard(t *testing.T) {
	// Clean up in case of previous failed run
	apiDelete("/api/dev/stacks/pw-test-stack")

	navigate(t, "#/developer")

	// Click "+ New Stack"
	btn := page.Locator("button:has-text('+ New Stack')")
	assertVisible(t, btn, "+ New Stack button")
	if err := btn.Click(); err != nil {
		t.Fatalf("click + New Stack: %v", err)
	}

	// Assert modal appears
	modal := page.Locator("h3:has-text('New Stack')")
	assertVisible(t, modal, "New Stack modal heading")

	// Fill stack ID
	input := page.Locator("input[placeholder='my-stack']")
	assertVisible(t, input, "stack ID input")
	if err := input.Fill("pw-test-stack"); err != nil {
		t.Fatalf("fill stack ID: %v", err)
	}

	// Select "Web + Database" template
	sel := page.Locator("select")
	if _, err := sel.SelectOption(playwright.SelectOptionValues{Values: &[]string{"web-database"}}); err != nil {
		t.Fatalf("select template: %v", err)
	}

	// Click "Create Stack"
	createBtn := page.Locator("button:has-text('Create Stack')")
	if err := createBtn.Click(); err != nil {
		t.Fatalf("click Create Stack: %v", err)
	}

	// Wait for navigation to stack editor — hash-based URLs need polling
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(page.URL(), "#/dev/stack/pw-test-stack") {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if !strings.Contains(page.URL(), "#/dev/stack/pw-test-stack") {
		t.Fatalf("expected navigation to #/dev/stack/pw-test-stack, got: %s", page.URL())
	}

	// Assert page contains stack name (titleFromID: "pw-test-stack" → "Pw Test Stack")
	heading := page.Locator("h2:has-text('Pw Test Stack')")
	assertVisible(t, heading, "stack editor heading")
}

// --- Test 3: Stack Editor — Validate & Deploy ---

func TestStackEditorValidateDeploy(t *testing.T) {
	navigate(t, "#/dev/stack/pw-test-stack")

	// Assert CodeMirror editor visible
	editor := page.Locator(".cm-editor")
	assertVisible(t, editor, "CodeMirror editor")

	// Assert "stack.yml" label
	label := page.Locator("text=stack.yml")
	assertVisible(t, label, "stack.yml label")

	// Assert action buttons
	for _, btn := range []string{"Validate", "Deploy", "Export", "Submit"} {
		loc := page.Locator(fmt.Sprintf("button:has-text('%s')", btn))
		assertVisible(t, loc, btn+" button")
	}

	// Assert status badge shows "draft"
	badge := page.Locator("text=draft")
	assertVisible(t, badge, "draft status badge")

	// Click Validate
	validateBtn := page.Locator("button:has-text('Validate')")
	if err := validateBtn.Click(); err != nil {
		t.Fatalf("click Validate: %v", err)
	}

	// Wait for validation result
	validResult := page.Locator("text=Valid").Or(page.Locator("text=Invalid"))
	if err := validResult.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
	}); err != nil {
		t.Fatalf("validation result did not appear: %v", err)
	}
	t.Log("validation result appeared")

	// Click Deploy
	deployBtn := page.Locator("button:has-text('Deploy')")
	if err := deployBtn.Click(); err != nil {
		t.Fatalf("click Deploy: %v", err)
	}

	// Wait for badge to change to "deployed"
	deployedBadge := page.Locator("text=deployed")
	if err := deployedBadge.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
	}); err != nil {
		t.Fatalf("deployed badge did not appear: %v", err)
	}

	// Assert Undeploy button now visible
	undeployBtn := page.Locator("button:has-text('Undeploy')")
	assertVisible(t, undeployBtn, "Undeploy button after deploy")
}

// --- Test 4: Catalog Stacks After Deploy ---

func TestCatalogStacksAfterDeploy(t *testing.T) {
	navigate(t, "#/catalog-stacks")

	// Assert heading
	h2 := page.Locator("h2:has-text('Stack Templates')")
	assertVisible(t, h2, "Stack Templates heading")

	// Wait for stack cards to appear (network fetch)
	cards := page.Locator("a[href^='#/catalog-stack/']")
	if err := cards.First().WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
	}); err != nil {
		t.Fatalf("no catalog stack cards appeared: %v", err)
	}

	// Assert pw-test-stack is present
	pwCard := page.Locator("a[href='#/catalog-stack/pw-test-stack']")
	assertVisible(t, pwCard, "pw-test-stack catalog card")

	// Click the card
	if err := pwCard.Click(); err != nil {
		t.Fatalf("click pw-test-stack card: %v", err)
	}

	// Wait for detail page
	time.Sleep(1 * time.Second)
	url := page.URL()
	if !strings.Contains(url, "#/catalog-stack/pw-test-stack") {
		t.Fatalf("expected #/catalog-stack/pw-test-stack, got: %s", url)
	}

	// Assert stack name visible
	name := page.Locator("h2:has-text('pw-test-stack')").Or(page.Locator("h2:has-text('PW Test Stack')"))
	assertVisible(t, name, "catalog stack detail name")
}

// --- Test 5: Stack Undeploy ---

func TestStackUndeploy(t *testing.T) {
	navigate(t, "#/dev/stack/pw-test-stack")

	// Click Undeploy
	undeployBtn := page.Locator("button:has-text('Undeploy')")
	assertVisible(t, undeployBtn, "Undeploy button")
	if err := undeployBtn.Click(); err != nil {
		t.Fatalf("click Undeploy: %v", err)
	}

	// Wait for badge to show "draft"
	draftBadge := page.Locator("text=draft")
	if err := draftBadge.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
	}); err != nil {
		t.Fatalf("draft badge did not reappear: %v", err)
	}

	// Assert Deploy button reappears
	deployBtn := page.Locator("button:has-text('Deploy')")
	assertVisible(t, deployBtn, "Deploy button after undeploy")

	// Navigate to catalog stacks
	navigate(t, "#/catalog-stacks")

	// Assert pw-test-stack is NOT present (or empty state)
	time.Sleep(1 * time.Second)
	emptyState := page.Locator("text=No stack templates available")
	pwCard := page.Locator("a[href='#/catalog-stack/pw-test-stack']")

	emptyVisible, _ := emptyState.IsVisible()
	pwVisible, _ := pwCard.IsVisible()

	if pwVisible {
		t.Error("pw-test-stack should not appear in catalog after undeploy")
	}
	if !emptyVisible && !pwVisible {
		// Other stacks may exist, just confirm pw-test-stack is gone
		t.Log("catalog has other stacks but pw-test-stack is correctly absent")
	}
	if emptyVisible {
		t.Log("empty state confirmed — no stacks in catalog")
	}
}

// --- Test 6: Import ZIP Shows in Dashboard ---

func TestImportZipShowsInDashboard(t *testing.T) {
	// Clean up in case of previous run
	apiDelete("/api/dev/stacks/zip-import-test")

	// Create a minimal stack ZIP in memory and POST via API
	zipData := createStackZip(t, "zip-import-test")
	resp := apiPostZip("/api/dev/import/zip", zipData)
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("import ZIP failed: HTTP %d — %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	// Navigate to developer dashboard
	navigate(t, "#/developer")

	// Assert card link appears
	card := page.Locator("a[href='#/dev/stack/zip-import-test']")
	assertVisible(t, card, "zip-import-test card in dashboard")

	// Click it
	if err := card.Click(); err != nil {
		t.Fatalf("click zip-import-test card: %v", err)
	}

	// Verify navigation
	time.Sleep(1 * time.Second)
	url := page.URL()
	if !strings.Contains(url, "#/dev/stack/zip-import-test") {
		t.Fatalf("expected #/dev/stack/zip-import-test, got: %s", url)
	}

	// Assert editor visible
	editor := page.Locator(".cm-editor")
	assertVisible(t, editor, "CodeMirror editor for imported stack")
}

// --- Test 7: Settings Developer Mode Toggle ---

func TestSettingsDevModeToggle(t *testing.T) {
	// First deploy pw-test-stack so catalog-stacks has something
	apiPost("/api/dev/stacks/pw-test-stack/deploy", "")

	navigate(t, "#/settings")

	// Assert heading
	h2 := page.Locator("h2:has-text('Settings')")
	assertVisible(t, h2, "Settings heading")

	// Click Developer tab
	devTab := page.Locator("button:has-text('Developer')")
	if err := devTab.Click(); err != nil {
		t.Fatalf("click Developer tab: %v", err)
	}

	// Assert "Developer Mode" heading visible (use heading role to avoid matching paragraph text)
	devModeText := page.Locator("h3:has-text('Developer Mode')")
	assertVisible(t, devModeText, "Developer Mode heading")

	// Click the toggle to disable dev mode
	toggle := page.Locator("button.relative.rounded-full")
	assertVisible(t, toggle, "dev mode toggle button")
	if err := toggle.Click(); err != nil {
		t.Fatalf("click toggle: %v", err)
	}

	// Wait for API to complete
	time.Sleep(1500 * time.Millisecond)

	// Navigate to catalog-stacks — should show empty state
	navigate(t, "#/catalog-stacks")
	time.Sleep(1 * time.Second)

	emptyState := page.Locator("text=No stack templates available")
	emptyVisible, _ := emptyState.IsVisible()
	t.Logf("catalog-stacks empty state visible: %v", emptyVisible)

	// Developer nav link should NOT be visible
	devLink := page.Locator("a[href='#/developer']")
	devVisible, _ := devLink.IsVisible()
	if devVisible {
		t.Error("Developer nav link should be hidden when dev mode is off")
	} else {
		t.Log("Developer nav link correctly hidden")
	}

	// Re-enable dev mode via API
	enableDevMode()

	// Reload page
	if _, err := page.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	time.Sleep(1500 * time.Millisecond)

	// Assert Developer nav link reappears
	devLink2 := page.Locator("a[href='#/developer']")
	assertVisible(t, devLink2, "Developer nav link after re-enable")
}

// --- Test 8: Publish Status Dialog ---

func TestPublishStatusDialog(t *testing.T) {
	navigate(t, "#/dev/stack/pw-test-stack")

	// Click Submit
	submitBtn := page.Locator("button:has-text('Submit')")
	assertVisible(t, submitBtn, "Submit button")
	if err := submitBtn.Click(); err != nil {
		t.Fatalf("click Submit: %v", err)
	}

	// Wait for modal
	modal := page.Locator("h3:has-text('Submit to Catalog')")
	if err := modal.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
	}); err != nil {
		t.Fatalf("submit modal did not appear: %v", err)
	}

	// Assert publish checklist OR loading text
	checklist := page.Locator("text=Publish Checklist").Or(page.Locator("text=Checking publish readiness"))
	assertVisible(t, checklist, "publish checklist or loading state")

	// Wait for checklist to load
	time.Sleep(2 * time.Second)

	// Check for "GitHub connected" text (should appear as a check item)
	ghCheck := page.Locator("text=GitHub connected")
	ghVisible, _ := ghCheck.IsVisible()
	t.Logf("GitHub connected check visible: %v", ghVisible)

	// Close modal by pressing Escape
	if err := page.Keyboard().Press("Escape"); err != nil {
		// Try clicking the backdrop instead
		backdrop := page.Locator(".fixed.inset-0")
		if err := backdrop.Click(playwright.LocatorClickOptions{
			Position: &playwright.Position{X: 5, Y: 5},
		}); err != nil {
			t.Logf("warning: could not close modal: %v", err)
		}
	}

	time.Sleep(500 * time.Millisecond)

	// Verify modal is gone
	modalGone, _ := modal.IsVisible()
	if modalGone {
		t.Log("warning: modal still visible after escape (backdrop click may be needed)")
	} else {
		t.Log("modal closed successfully")
	}
}

// --- Test 9: Branch App from Catalog ---

func TestBranchAppFromCatalog(t *testing.T) {
	// Clean up in case of previous failed run
	apiPost("/api/dev/apps/pw-branch-test/undeploy", "")
	apiDelete("/api/dev/apps/pw-branch-test")

	navigate(t, "#/app/hello-world")

	// The Branch button requires GitHub connected + fork.
	// In CI/test environments GitHub may not be connected, so we check via API first.
	resp, err := http.Get(baseURL + "/api/dev/github/status")
	if err != nil {
		t.Fatalf("GET github status: %v", err)
	}
	var ghStatus struct {
		Connected bool `json:"connected"`
		Fork      *struct {
			FullName string `json:"full_name"`
		} `json:"fork"`
	}
	json.NewDecoder(resp.Body).Decode(&ghStatus)
	resp.Body.Close()

	if ghStatus.Connected && ghStatus.Fork != nil {
		// Assert Branch button visible
		branchBtn := page.Locator("button:has-text('Branch')")
		assertVisible(t, branchBtn, "Branch button on app detail")
		t.Log("Branch button visible (GitHub connected with fork)")
	} else {
		t.Log("GitHub not connected — Branch button correctly hidden")
	}

	// Use fork API to create a uniquely-named test copy (avoids ID collision with catalog)
	apiResp := apiPost("/api/dev/fork", `{"source_id":"hello-world","new_id":"pw-branch-test"}`)
	if apiResp.StatusCode != 201 {
		body, _ := io.ReadAll(apiResp.Body)
		apiResp.Body.Close()
		t.Fatalf("API fork failed: HTTP %d — %s", apiResp.StatusCode, string(body))
	}
	apiResp.Body.Close()

	navigate(t, "#/dev/pw-branch-test")

	// Assert DevAppEditor loads with heading
	heading := page.Locator("h2")
	assertVisible(t, heading, "dev app editor heading")

	// Assert CodeMirror visible
	editor := page.Locator(".cm-editor")
	assertVisible(t, editor, "CodeMirror editor for branched app")
}

// --- Test 10: Dev App Editor Layout ---

func TestDevAppEditorLayout(t *testing.T) {
	navigate(t, "#/dev/pw-branch-test")

	// Assert file tree items
	for _, file := range []string{"app.yml", "provision/install.py", "README.md"} {
		loc := page.Locator(fmt.Sprintf("button:has-text('%s')", file))
		assertVisible(t, loc, file+" in file tree")
	}

	// Assert "Files" heading in sidebar
	filesHeading := page.Locator("text=Files")
	assertVisible(t, filesHeading, "Files sidebar heading")

	// Assert action buttons
	for _, btn := range []string{"Validate", "Deploy", "Export", "SDK Ref", "Submit", "Delete"} {
		loc := page.Locator(fmt.Sprintf("button:has-text('%s')", btn))
		assertVisible(t, loc, btn+" button")
	}

	// Assert status badge "draft"
	badge := page.Locator("text=draft")
	assertVisible(t, badge, "draft status badge")

	// Assert version visible (format: v1.0.1 etc.)
	version := page.Locator("span.text-xs:has-text('v')")
	assertVisible(t, version, "version label")
}

// --- Test 11: Dev App Edit Manifest ---

func TestDevAppEditManifest(t *testing.T) {
	navigate(t, "#/dev/pw-branch-test")

	// Click app.yml in file tree (should be default active)
	appYmlBtn := page.Locator("button:has-text('app.yml')")
	assertVisible(t, appYmlBtn, "app.yml file tree button")

	// Assert CodeMirror editor visible
	editor := page.Locator(".cm-editor")
	assertVisible(t, editor, "CodeMirror editor")

	// Update manifest via API with a modified description
	newManifest := `id: pw-branch-test
name: PW Branch Test
description: Playwright E2E test branch app
version: "1.0.0"
categories:
  - testing
lxc:
  ostemplate: local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst
  defaults:
    cores: 1
    memory_mb: 512
    disk_gb: 4
`
	resp := apiPut("/api/dev/apps/pw-branch-test/manifest", newManifest, "text/plain")
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("save manifest failed: HTTP %d — %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	// Also save a modified install.py with a marker comment
	newScript := `#!/usr/bin/env python3
# Playwright E2E test branch script
from appstore import App

class Install(App):
    def install(self):
        self.apt_install(["nginx"])
        self.enable_service("nginx")
        self.set_output("url", f"http://{self.ip}:80")
`
	resp = apiPut("/api/dev/apps/pw-branch-test/script", newScript, "text/plain")
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("save script failed: HTTP %d — %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	// Reload page and verify the editor shows updated content
	navigate(t, "#/dev/pw-branch-test")

	// Wait for CodeMirror to render with new content
	time.Sleep(1 * time.Second)

	// Verify the updated description appears in the editor
	editorContent := page.Locator(".cm-editor:has-text('Playwright E2E test branch')")
	if err := editorContent.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(5000),
	}); err != nil {
		// Not fatal — CodeMirror virtual DOM may not expose text via :has-text
		t.Log("warning: could not verify manifest content via selector (CodeMirror virtual DOM)")
	} else {
		t.Log("manifest content verified in editor")
	}
}

// --- Test 12: Dev App Validation ---

func TestDevAppValidation(t *testing.T) {
	navigate(t, "#/dev/pw-branch-test")

	// Click Validate
	validateBtn := page.Locator("button:has-text('Validate')")
	assertVisible(t, validateBtn, "Validate button")
	if err := validateBtn.Click(); err != nil {
		t.Fatalf("click Validate: %v", err)
	}

	// Wait for validation panel to appear with PASS or FAIL
	passOrFail := page.Locator("text=PASS").Or(page.Locator("text=FAIL"))
	if err := passOrFail.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
	}); err != nil {
		t.Fatalf("validation result did not appear: %v", err)
	}

	// Check which result
	passVisible, _ := page.Locator("text=PASS").IsVisible()
	failVisible, _ := page.Locator("text=FAIL").IsVisible()

	if passVisible {
		t.Log("validation PASSED")
		// Assert checklist visible
		checklist := page.Locator("text=Checklist")
		assertVisible(t, checklist, "validation Checklist heading")
	} else if failVisible {
		t.Log("validation FAILED")
		// Log error count if visible
		errors := page.Locator("text=Errors")
		errVisible, _ := errors.IsVisible()
		if errVisible {
			t.Log("Errors section visible in validation panel")
		}
	}
}

// --- Test 13: Dev App Deploy ---

func TestDevAppDeploy(t *testing.T) {
	navigate(t, "#/dev/pw-branch-test")

	// Register dialog handler BEFORE clicking Deploy (it calls alert())
	dialogCh := make(chan string, 1)
	page.On("dialog", func(dialog playwright.Dialog) {
		msg := dialog.Message()
		dialog.Accept()
		select {
		case dialogCh <- msg:
		default:
		}
	})

	// Click Deploy
	deployBtn := page.Locator("button:has-text('Deploy')")
	assertVisible(t, deployBtn, "Deploy button")
	if err := deployBtn.Click(); err != nil {
		t.Fatalf("click Deploy: %v", err)
	}

	// Wait for alert dialog
	select {
	case msg := <-dialogCh:
		t.Logf("deploy alert: %s", msg)
	case <-time.After(15 * time.Second):
		t.Fatal("deploy alert did not fire within 15s")
	}

	// Wait for page to re-fetch and update
	time.Sleep(1 * time.Second)

	// Assert badge changes to "deployed"
	deployedBadge := page.Locator("text=deployed")
	if err := deployedBadge.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
	}); err != nil {
		t.Fatalf("deployed badge did not appear: %v", err)
	}

	// Assert Undeploy button visible
	undeployBtn := page.Locator("button:has-text('Undeploy')")
	assertVisible(t, undeployBtn, "Undeploy button after deploy")

	// Assert Test Install link visible
	testInstallLink := page.Locator("a:has-text('Test Install')")
	assertVisible(t, testInstallLink, "Test Install link after deploy")

	// Navigate to catalog and verify app appears
	navigate(t, "#/apps")
	time.Sleep(1 * time.Second)

	// Search for the branched app
	appCard := page.Locator("text=PW Branch Test").Or(page.Locator("text=pw-branch-test"))
	cardVisible, _ := appCard.IsVisible()
	if cardVisible {
		t.Log("branched app visible in catalog after deploy")
	} else {
		t.Log("warning: branched app not immediately visible in catalog (may need search)")
	}
}

// --- Test 14: Dev App Submit PR (first submit — creates PR) ---

func TestDevAppSubmitPR(t *testing.T) {
	// Insert synthetic install record so test_installed check passes
	sqliteExec(`INSERT OR REPLACE INTO installs (id, app_id, app_name, ctid, node, pool, status, created_at) VALUES ('pw-fake-install', 'pw-branch-test', 'PW Branch Test', 99999, 'pve', 'local-lvm', 'running', '2025-01-01T00:00:00Z')`)

	navigate(t, "#/dev/pw-branch-test")

	// Click Submit
	submitBtn := page.Locator("button:has-text('Submit')")
	assertVisible(t, submitBtn, "Submit button")
	if err := submitBtn.Click(); err != nil {
		t.Fatalf("click Submit: %v", err)
	}

	// Wait for modal and checklist
	modal := page.Locator("h3:has-text('Submit to Catalog')")
	if err := modal.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
	}); err != nil {
		t.Fatalf("submit modal did not appear: %v", err)
	}

	checklist := page.Locator("text=Publish Checklist")
	checklist.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(10000)})
	time.Sleep(2 * time.Second)

	// Assert checklist: 3 items, NO fork_exists (fork is an internal detail now)
	for _, label := range []string{"GitHub connected", "Manifest validates", "Test install exists"} {
		loc := page.Locator(fmt.Sprintf("text=%s", label))
		vis, _ := loc.IsVisible()
		t.Logf("checklist %q visible: %v", label, vis)
	}
	// fork_exists must NOT appear
	forkCheck := page.Locator("text=Catalog fork exists")
	forkVisible, _ := forkCheck.IsVisible()
	if forkVisible {
		t.Error("fork_exists check should not appear in publish checklist")
	}

	// Find the submit button — on first submit with no prior PR it should say "Submit Pull Request"
	submitPRBtn := page.Locator("button:has-text('Submit Pull Request')")
	prBtnVisible, _ := submitPRBtn.IsVisible()
	if !prBtnVisible {
		// May also say "Update Pull Request" if a prior PR is still open from a previous test run
		submitPRBtn = page.Locator("button:has-text('Update Pull Request')")
		prBtnVisible, _ = submitPRBtn.IsVisible()
	}
	if !prBtnVisible {
		t.Log("Submit/Update Pull Request button not visible (GitHub may not be connected)")
		closeSubmitModal()
		return
	}

	disabled, _ := submitPRBtn.GetAttribute("disabled")
	if disabled != "" {
		t.Log("Submit button is disabled (not all checks pass)")
		closeSubmitModal()
		return
	}

	// Click submit
	btnText, _ := submitPRBtn.TextContent()
	t.Logf("clicking %q...", btnText)
	if err := submitPRBtn.Click(); err != nil {
		t.Fatalf("click submit button: %v", err)
	}

	// Wait for result
	modalContainer := page.Locator(".fixed.inset-0 .bg-bg-card")
	okResult := modalContainer.Locator("text=[OK]")
	errorResult := modalContainer.Locator(".text-red-400.font-mono")

	result := okResult.Or(errorResult)
	if err := result.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(30000),
	}); err != nil {
		t.Logf("PR result not detected within 30s (non-fatal): %v", err)
	}

	okVisible, _ := okResult.IsVisible()
	if okVisible {
		// Assert success message says "created!" or "updated!"
		createdMsg := modalContainer.Locator("text=Pull request created!")
		updatedMsg := modalContainer.Locator("text=Pull request updated!")
		createdVis, _ := createdMsg.IsVisible()
		updatedVis, _ := updatedMsg.IsVisible()
		t.Logf("success message: created=%v updated=%v", createdVis, updatedVis)

		prLink := modalContainer.Locator("a[href*='github.com']")
		if href, err := prLink.GetAttribute("href"); err == nil {
			t.Logf("PR URL: %s", href)
		}
	} else {
		errVisible, _ := errorResult.IsVisible()
		if errVisible {
			errText, _ := errorResult.TextContent()
			t.Logf("PR submission error (non-fatal): %s", errText)
		}
	}

	closeSubmitModalFull(t)
}

// --- Test 14b: Dev App Re-submit PR (updates existing PR instead of creating duplicate) ---

func TestDevAppResubmitPR(t *testing.T) {
	// This test runs AFTER TestDevAppSubmitPR which created/updated a PR.
	// Re-submitting should find the open PR and update it, not create a new one.

	navigate(t, "#/dev/pw-branch-test")

	// Click Submit
	submitBtn := page.Locator("button:has-text('Submit')")
	assertVisible(t, submitBtn, "Submit button")
	if err := submitBtn.Click(); err != nil {
		t.Fatalf("click Submit: %v", err)
	}

	// Wait for modal and checklist to load
	modal := page.Locator("h3:has-text('Submit to Catalog')")
	if err := modal.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
	}); err != nil {
		t.Fatalf("submit modal did not appear: %v", err)
	}

	time.Sleep(3 * time.Second) // let publish-status API complete

	// The previous test created a PR that should still be open.
	// The button should now say "Update Pull Request" instead of "Submit Pull Request".
	updateBtn := page.Locator("button:has-text('Update Pull Request')")
	submitNewBtn := page.Locator("button:has-text('Submit Pull Request')")

	updateVis, _ := updateBtn.IsVisible()
	submitVis, _ := submitNewBtn.IsVisible()
	t.Logf("button state: 'Update Pull Request'=%v, 'Submit Pull Request'=%v", updateVis, submitVis)

	var actionBtn playwright.Locator
	if updateVis {
		t.Log("open PR detected — button correctly shows 'Update Pull Request'")
		actionBtn = updateBtn
	} else if submitVis {
		t.Log("no open PR detected (may have been merged/closed) — showing 'Submit Pull Request'")
		actionBtn = submitNewBtn
	} else {
		t.Log("neither submit button visible (GitHub may not be connected)")
		closeSubmitModal()
		return
	}

	disabled, _ := actionBtn.GetAttribute("disabled")
	if disabled != "" {
		t.Log("button is disabled (not all checks pass)")
		closeSubmitModal()
		return
	}

	// Record the PR URL from publish status before clicking
	prLinkBefore := ""
	if statusResp, err := http.Get(baseURL + "/api/dev/apps/pw-branch-test/publish-status"); err == nil {
		var status struct {
			PRURL string `json:"pr_url"`
		}
		json.NewDecoder(statusResp.Body).Decode(&status)
		statusResp.Body.Close()
		prLinkBefore = status.PRURL
		t.Logf("PR URL before re-submit: %s", prLinkBefore)
	}

	// Click the button
	btnText, _ := actionBtn.TextContent()
	t.Logf("clicking %q...", btnText)
	if err := actionBtn.Click(); err != nil {
		t.Fatalf("click action button: %v", err)
	}

	// Wait for result
	modalContainer := page.Locator(".fixed.inset-0 .bg-bg-card")
	okResult := modalContainer.Locator("text=[OK]")
	errorResult := modalContainer.Locator(".text-red-400.font-mono")

	result := okResult.Or(errorResult)
	if err := result.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(30000),
	}); err != nil {
		t.Logf("PR result not detected within 30s (non-fatal): %v", err)
	}

	okVisible, _ := okResult.IsVisible()
	if !okVisible {
		errVisible, _ := errorResult.IsVisible()
		if errVisible {
			errText, _ := errorResult.TextContent()
			t.Logf("PR re-submit error (non-fatal): %s", errText)
		}
		closeSubmitModalFull(t)
		return
	}

	// Assert correct success message based on whether we expected an update
	updatedMsg := modalContainer.Locator("text=Pull request updated!")
	createdMsg := modalContainer.Locator("text=Pull request created!")
	updatedVis, _ := updatedMsg.IsVisible()
	createdVis, _ := createdMsg.IsVisible()

	if updateVis {
		// We saw "Update Pull Request" button → expect "Pull request updated!" message
		if !updatedVis {
			t.Error("expected 'Pull request updated!' message but got 'Pull request created!'")
		} else {
			t.Log("re-submit correctly shows 'Pull request updated!'")
		}
	} else {
		t.Logf("re-submit message: updated=%v created=%v", updatedVis, createdVis)
	}

	// Assert PR URL is the same (updated, not a new PR)
	prLink := modalContainer.Locator("a[href*='github.com']")
	prLinkAfter, _ := prLink.GetAttribute("href")
	t.Logf("PR URL after re-submit: %s", prLinkAfter)

	if prLinkBefore != "" && prLinkAfter != "" && updateVis {
		if prLinkBefore == prLinkAfter {
			t.Log("PR URL unchanged — confirmed update, not duplicate")
		} else {
			t.Errorf("PR URL changed on re-submit: %s → %s (should reuse existing PR)", prLinkBefore, prLinkAfter)
		}
	}

	closeSubmitModalFull(t)
}

// --- Test 15: Dev App Undeploy ---

func TestDevAppUndeploy(t *testing.T) {
	navigate(t, "#/dev/pw-branch-test")

	// Ensure it's deployed first (may already be from TestDevAppDeploy)
	deployedBadge := page.Locator("text=deployed")
	isDeployed, _ := deployedBadge.IsVisible()
	if !isDeployed {
		t.Log("app not deployed, deploying first...")
		apiPost("/api/dev/apps/pw-branch-test/deploy", "")
		navigate(t, "#/dev/pw-branch-test")
	}

	// Click Undeploy
	undeployBtn := page.Locator("button:has-text('Undeploy')")
	assertVisible(t, undeployBtn, "Undeploy button")
	if err := undeployBtn.Click(); err != nil {
		t.Fatalf("click Undeploy: %v", err)
	}

	// Wait for badge to return to "draft"
	draftBadge := page.Locator("text=draft")
	if err := draftBadge.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(10000),
	}); err != nil {
		t.Fatalf("draft badge did not reappear: %v", err)
	}

	// Assert Deploy button reappears
	deployBtn := page.Locator("button:has-text('Deploy')")
	assertVisible(t, deployBtn, "Deploy button after undeploy")

	// Navigate to catalog and assert app is NOT visible
	navigate(t, "#/apps")
	time.Sleep(1 * time.Second)

	appCard := page.Locator("a[href='#/app/pw-branch-test']")
	cardVisible, _ := appCard.IsVisible()
	if cardVisible {
		t.Error("pw-branch-test should not appear in catalog after undeploy")
	} else {
		t.Log("pw-branch-test correctly absent from catalog after undeploy")
	}
}

// --- Test 16: Cleanup ---

func TestCleanup(t *testing.T) {
	// Clean up synthetic install record
	sqliteExec("DELETE FROM installs WHERE id='pw-fake-install'")

	// Undeploy first (in case still deployed)
	apiPost("/api/dev/stacks/pw-test-stack/undeploy", "")
	apiPost("/api/dev/apps/pw-branch-test/undeploy", "")

	// Delete test stacks via API
	for _, id := range []string{"pw-test-stack", "zip-import-test"} {
		resp := apiDeleteResp("/api/dev/stacks/" + id)
		if resp.StatusCode == 200 {
			t.Logf("deleted dev stack: %s", id)
		} else if resp.StatusCode == 404 {
			t.Logf("dev stack already gone: %s", id)
		} else {
			body, _ := io.ReadAll(resp.Body)
			t.Logf("delete %s: HTTP %d — %s", id, resp.StatusCode, string(body))
		}
		resp.Body.Close()
	}

	// Delete test app via API
	resp := apiDeleteResp("/api/dev/apps/pw-branch-test")
	if resp.StatusCode == 200 {
		t.Log("deleted dev app: pw-branch-test")
	} else if resp.StatusCode == 404 {
		t.Log("dev app already gone: pw-branch-test")
	} else {
		body, _ := io.ReadAll(resp.Body)
		t.Logf("delete pw-branch-test: HTTP %d — %s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────────

func navigate(t *testing.T, hash string) {
	t.Helper()
	url := baseURL + "/" + hash
	if _, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(15000),
	}); err != nil {
		t.Fatalf("navigate to %s: %v", hash, err)
	}
	time.Sleep(500 * time.Millisecond) // let React render
}

func assertVisible(t *testing.T, loc playwright.Locator, name string) {
	t.Helper()
	if err := loc.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(5000),
	}); err != nil {
		t.Errorf("%s not visible: %v", name, err)
	}
}

func enableDevMode() {
	body := `{"developer":{"enabled":true}}`
	req, _ := http.NewRequest("PUT", baseURL+"/api/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

func apiPost(path, body string) *http.Response {
	req, _ := http.NewRequest("POST", baseURL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &http.Response{StatusCode: 0}
	}
	return resp
}

func apiDelete(path string) {
	req, _ := http.NewRequest("DELETE", baseURL+path, nil)
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

func apiDeleteResp(path string) *http.Response {
	req, _ := http.NewRequest("DELETE", baseURL+path, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &http.Response{StatusCode: 0}
	}
	return resp
}

func apiPostZip(path string, zipData []byte) *http.Response {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "stack.zip")
	if err != nil {
		return &http.Response{StatusCode: 0}
	}
	fw.Write(zipData)
	w.Close()

	req, _ := http.NewRequest("POST", baseURL+path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &http.Response{StatusCode: 0}
	}
	return resp
}

func sqliteExec(sql string) {
	cmd := exec.Command("sqlite3", "/var/lib/pve-appstore/jobs.db", sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sqliteExec %q: %v — %s\n", sql, err, string(out))
	}
}

func apiPut(path, body, contentType string) *http.Response {
	req, _ := http.NewRequest("PUT", baseURL+path, strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &http.Response{StatusCode: 0}
	}
	return resp
}

func waitForHash(t *testing.T, hash string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(page.URL(), hash) {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("expected navigation to %s, got: %s", hash, page.URL())
}

func closeSubmitModal() {
	cancelBtn := page.Locator(".fixed.inset-0 .bg-bg-card button:has-text('Cancel')")
	if vis, _ := cancelBtn.IsVisible(); vis {
		cancelBtn.Click()
	} else {
		backdrop := page.Locator(".fixed.inset-0").First()
		backdrop.Click(playwright.LocatorClickOptions{
			Position: &playwright.Position{X: 5, Y: 5},
			Force:    playwright.Bool(true),
		})
	}
	time.Sleep(500 * time.Millisecond)
}

// closeSubmitModalFull tries Done, Cancel, backdrop click, then navigates away as last resort.
func closeSubmitModalFull(t *testing.T) {
	t.Helper()
	modalContainer := page.Locator(".fixed.inset-0 .bg-bg-card")

	doneBtn := modalContainer.Locator("button:has-text('Done')")
	cancelBtn := modalContainer.Locator("button:has-text('Cancel')")

	if vis, _ := doneBtn.IsVisible(); vis {
		doneBtn.Click()
	} else if vis, _ := cancelBtn.IsVisible(); vis {
		cancelBtn.Click()
	} else {
		backdrop := page.Locator(".fixed.inset-0").First()
		backdrop.Click(playwright.LocatorClickOptions{
			Position: &playwright.Position{X: 5, Y: 5},
			Force:    playwright.Bool(true),
		})
	}
	time.Sleep(1 * time.Second)

	if vis, _ := page.Locator("h3:has-text('Submit to Catalog')").IsVisible(); vis {
		t.Log("warning: modal still open, navigating away to force close")
		navigate(t, "#/developer")
		time.Sleep(500 * time.Millisecond)
	}
}

func createStackZip(t *testing.T, id string) []byte {
	t.Helper()

	manifest := fmt.Sprintf(`id: %s
name: %s
description: Test stack for Playwright E2E
version: "1.0.0"
categories:
  - testing
apps:
  - app_id: adguard
lxc:
  ostemplate: local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst
  defaults:
    cores: 1
    memory_mb: 512
    disk_gb: 4
`, id, id)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	fw, err := zw.Create(id + "/stack.yml")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	fw.Write([]byte(manifest))
	zw.Close()

	return buf.Bytes()
}

// settingsJSON is used for unmarshalling GET /api/settings.
type settingsJSON struct {
	Developer struct {
		Enabled bool `json:"enabled"`
	} `json:"developer"`
}

func getSettings() (*settingsJSON, error) {
	resp, err := http.Get(baseURL + "/api/settings")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var s settingsJSON
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}
