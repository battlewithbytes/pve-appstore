package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
)

// installSteps defines the ordered state machine for an install job.
// Each step returns the next state or an error (which moves to "failed").
var installSteps = []struct {
	state string
	fn    func(ctx *installContext) error
}{
	{StateValidateRequest, stepValidateRequest},
	{StateValidateManifest, stepValidateManifest},
	{StateValidatePlacement, stepValidatePlacement},
	{StateAllocateCTID, stepAllocateCTID},
	{StateCreateContainer, stepCreateContainer},
	{StateConfigureContainer, stepConfigureContainer},
	{StateStartContainer, stepStartContainer},
	{StateWaitForNetwork, stepWaitForNetwork},
	{StatePushAssets, stepPushAssets},
	{StateProvision, stepProvision},
	{StateHealthcheck, stepHealthcheck},
	{StateCollectOutputs, stepCollectOutputs},
}

// installContext carries state through the install pipeline.
type installContext struct {
	engine   *Engine
	job      *Job
	manifest *catalog.AppManifest
}

func (ctx *installContext) log(level, msg string, args ...interface{}) {
	message := fmt.Sprintf(msg, args...)
	ctx.engine.store.AppendLog(&LogEntry{
		JobID:     ctx.job.ID,
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
	})
}

func (ctx *installContext) info(msg string, args ...interface{}) {
	ctx.log("info", msg, args...)
}

func (ctx *installContext) warn(msg string, args ...interface{}) {
	ctx.log("warn", msg, args...)
}

func (ctx *installContext) transition(state string) {
	ctx.job.State = state
	ctx.job.UpdatedAt = time.Now()
	ctx.engine.store.UpdateJob(ctx.job)
	ctx.info("State: %s", state)
}

// runInstall executes the full install pipeline for a job.
func (e *Engine) runInstall(job *Job) {
	app, ok := e.catalog.Get(job.AppID)
	if !ok {
		job.State = StateFailed
		job.Error = fmt.Sprintf("app %q not found in catalog", job.AppID)
		now := time.Now()
		job.UpdatedAt = now
		job.CompletedAt = &now
		e.store.UpdateJob(job)
		return
	}

	ctx := &installContext{
		engine:   e,
		job:      job,
		manifest: app,
	}

	ctx.info("Starting install of %s (%s)", app.Name, app.ID)

	for _, step := range installSteps {
		ctx.transition(step.state)

		if err := step.fn(ctx); err != nil {
			ctx.log("error", "Failed at %s: %v", step.state, err)
			ctx.job.State = StateFailed
			ctx.job.Error = fmt.Sprintf("%s: %v", step.state, err)
			now := time.Now()
			ctx.job.UpdatedAt = now
			ctx.job.CompletedAt = &now
			e.store.UpdateJob(ctx.job)
			return
		}
	}

	// Success
	ctx.transition(StateCompleted)
	now := time.Now()
	ctx.job.CompletedAt = &now
	e.store.UpdateJob(ctx.job)

	// Record the installation
	e.store.CreateInstall(&Install{
		ID:        ctx.job.ID,
		AppID:     ctx.job.AppID,
		AppName:   ctx.job.AppName,
		CTID:      ctx.job.CTID,
		Node:      ctx.job.Node,
		Pool:      ctx.job.Pool,
		Status:    "running",
		CreatedAt: now,
	})

	ctx.info("Install complete! Container %d is running.", ctx.job.CTID)
}

func stepValidateRequest(ctx *installContext) error {
	if ctx.job.AppID == "" {
		return fmt.Errorf("app_id is required")
	}
	ctx.info("Request validated for app %s", ctx.job.AppID)
	return nil
}

func stepValidateManifest(ctx *installContext) error {
	if err := ctx.manifest.Validate(); err != nil {
		return fmt.Errorf("manifest validation: %w", err)
	}
	ctx.info("Manifest validated: %s v%s", ctx.manifest.Name, ctx.manifest.Version)
	return nil
}

func stepValidatePlacement(ctx *installContext) error {
	// Validate that storage and bridge are set
	if ctx.job.Storage == "" {
		return fmt.Errorf("storage is required")
	}
	if ctx.job.Bridge == "" {
		return fmt.Errorf("bridge is required")
	}
	ctx.info("Placement: storage=%s bridge=%s pool=%s", ctx.job.Storage, ctx.job.Bridge, ctx.job.Pool)
	return nil
}

func stepAllocateCTID(ctx *installContext) error {
	ctid, err := ctx.engine.cm.AllocateCTID(context.Background())
	if err != nil {
		return fmt.Errorf("allocating CTID: %w", err)
	}
	ctx.job.CTID = ctid
	ctx.job.UpdatedAt = time.Now()
	ctx.engine.store.UpdateJob(ctx.job)
	ctx.info("Allocated CTID: %d", ctid)
	return nil
}

func stepCreateContainer(ctx *installContext) error {
	// Resolve OS template
	template := ctx.manifest.LXC.OSTemplate
	if !strings.Contains(template, ":") {
		// Shorthand like "debian-12" — resolve to a full template path
		ctx.info("Resolving template %q (will download if needed)...", template)
		template = ctx.engine.cm.ResolveTemplate(context.Background(), template, ctx.job.Storage)
	}

	opts := CreateOptions{
		CTID:         ctx.job.CTID,
		OSTemplate:   template,
		Storage:      ctx.job.Storage,
		RootFSSize:   ctx.job.DiskGB,
		Cores:        ctx.job.Cores,
		MemoryMB:     ctx.job.MemoryMB,
		Bridge:       ctx.job.Bridge,
		Hostname:     ctx.job.AppID,
		Unprivileged: ctx.manifest.LXC.Defaults.Unprivileged,
		Pool:         ctx.job.Pool,
		Features:     ctx.manifest.LXC.Defaults.Features,
		OnBoot:       ctx.manifest.LXC.Defaults.OnBoot,
		Tags:         "appstore;managed",
	}

	ctx.info("Creating container %d (template=%s, %d cores, %d MB, %d GB)",
		opts.CTID, template, opts.Cores, opts.MemoryMB, opts.RootFSSize)

	if err := ctx.engine.cm.Create(context.Background(), opts); err != nil {
		return err
	}

	ctx.info("Container %d created", ctx.job.CTID)
	return nil
}

func stepConfigureContainer(ctx *installContext) error {
	// Container is configured during creation. Additional config could be done here.
	ctx.info("Container %d configured", ctx.job.CTID)
	return nil
}

func stepStartContainer(ctx *installContext) error {
	ctx.info("Starting container %d...", ctx.job.CTID)
	if err := ctx.engine.cm.Start(context.Background(), ctx.job.CTID); err != nil {
		return err
	}
	ctx.info("Container %d started", ctx.job.CTID)
	return nil
}

func stepWaitForNetwork(ctx *installContext) error {
	ctx.info("Waiting for network in container %d...", ctx.job.CTID)

	for i := 0; i < 30; i++ {
		ip, err := ctx.engine.cm.GetIP(ctx.job.CTID)
		if err == nil && ip != "" && ip != "127.0.0.1" {
			ctx.info("Container %d has IP: %s", ctx.job.CTID, ip)
			return nil
		}
		time.Sleep(2 * time.Second)
	}

	ctx.warn("Network wait timed out — continuing anyway")
	return nil
}

func stepPushAssets(ctx *installContext) error {
	if ctx.manifest.DirPath == "" {
		ctx.warn("No app directory path set — skipping asset push")
		return nil
	}

	provisionDir := filepath.Join(ctx.manifest.DirPath, "provision")
	if _, err := os.Stat(provisionDir); os.IsNotExist(err) {
		return fmt.Errorf("provision directory not found: %s", provisionDir)
	}

	// 1. Ensure python3 is available in the container
	ctx.info("Verifying python3 is available...")
	if err := ensurePython(ctx.job.CTID, ctx.engine.cm); err != nil {
		return fmt.Errorf("ensuring python3: %w", err)
	}

	// 2. Push the Python SDK into the container
	ctx.info("Pushing Python SDK...")
	if err := pushSDK(ctx.job.CTID, ctx.engine.cm); err != nil {
		return fmt.Errorf("pushing SDK: %w", err)
	}

	// 3. Push inputs and permissions JSON files
	ctx.info("Pushing inputs and permissions...")
	if err := pushInputsJSON(ctx.job.CTID, ctx.engine.cm, ctx.job.Inputs); err != nil {
		return fmt.Errorf("pushing inputs: %w", err)
	}
	if err := pushPermissionsJSON(ctx.job.CTID, ctx.engine.cm, ctx.manifest.Permissions); err != nil {
		return fmt.Errorf("pushing permissions: %w", err)
	}

	// 4. Push provision scripts and assets
	ctx.engine.cm.Exec(ctx.job.CTID, []string{"mkdir", "-p", provisionTargetDir})

	entries, err := os.ReadDir(provisionDir)
	if err != nil {
		return fmt.Errorf("reading provision dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		src := filepath.Join(provisionDir, entry.Name())
		dst := provisionTargetDir + "/" + entry.Name()
		ctx.info("Pushing %s -> %s", entry.Name(), dst)
		if err := ctx.engine.cm.Push(ctx.job.CTID, src, dst, "0755"); err != nil {
			return fmt.Errorf("pushing %s: %w", entry.Name(), err)
		}
	}

	// 5. Push templates if they exist
	templatesDir := filepath.Join(ctx.manifest.DirPath, "templates")
	if _, err := os.Stat(templatesDir); err == nil {
		ctx.engine.cm.Exec(ctx.job.CTID, []string{"mkdir", "-p", "/opt/appstore/templates"})
		entries, _ := os.ReadDir(templatesDir)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			src := filepath.Join(templatesDir, entry.Name())
			dst := "/opt/appstore/templates/" + entry.Name()
			ctx.info("Pushing template %s -> %s", entry.Name(), dst)
			ctx.engine.cm.Push(ctx.job.CTID, src, dst, "0644")
		}
	}

	ctx.info("Assets pushed to container %d", ctx.job.CTID)
	return nil
}

func stepProvision(ctx *installContext) error {
	cmd := buildProvisionCommand(ctx.manifest.Provisioning.Script, "install")

	ctx.info("Running provisioning: python3 -m appstore.runner ... install %s",
		filepath.Base(ctx.manifest.Provisioning.Script))

	// Stream output line-by-line for real-time log feedback
	result, err := ctx.engine.cm.ExecStream(ctx.job.CTID, cmd, func(line string) {
		parseProvisionLine(ctx, line)
	})
	if err != nil {
		return fmt.Errorf("executing provision script: %w", err)
	}

	if result.ExitCode != 0 {
		if result.ExitCode == 2 {
			return fmt.Errorf("provision failed: permission denied (exit 2)")
		}
		return fmt.Errorf("provision script exited with code %d", result.ExitCode)
	}

	ctx.info("Provisioning completed successfully")
	return nil
}

// parseProvisionLine handles a single line of provisioning output in real-time.
func parseProvisionLine(ctx *installContext, line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if strings.HasPrefix(line, "@@APPLOG@@") {
		jsonStr := strings.TrimPrefix(line, "@@APPLOG@@")
		level := extractJSONField(jsonStr, "level")
		msg := extractJSONField(jsonStr, "msg")
		if level == "output" {
			key := extractJSONField(jsonStr, "key")
			value := extractJSONField(jsonStr, "value")
			if key != "" {
				if ctx.job.Outputs == nil {
					ctx.job.Outputs = make(map[string]string)
				}
				ctx.job.Outputs[key] = value
			}
		} else if msg != "" {
			ctx.log(level, "[provision] %s", msg)
		}
	} else {
		ctx.info("[provision] %s", line)
	}
}

// parseProvisionOutput extracts structured @@APPLOG@@ lines from script output
// and logs them. Non-structured lines are logged as plain info.
func parseProvisionOutput(ctx *installContext, output string) {
	for _, line := range strings.Split(output, "\n") {
		parseProvisionLine(ctx, line)
	}
}

// extractJSONField is a simple field extractor for JSON strings.
// It avoids importing encoding/json in the hot path for simple key lookups.
func extractJSONField(jsonStr, key string) string {
	// Look for "key":"value" pattern
	search := `"` + key + `":"`
	idx := strings.Index(jsonStr, search)
	if idx < 0 {
		return ""
	}
	start := idx + len(search)
	end := strings.Index(jsonStr[start:], `"`)
	if end < 0 {
		return ""
	}
	return jsonStr[start : start+end]
}

func stepHealthcheck(ctx *installContext) error {
	// Check if the app's install.py implements healthcheck (always try via runner)
	// The runner will call the healthcheck method — if not overridden, it returns True
	healthcheckPy := filepath.Join(ctx.manifest.DirPath, "provision", filepath.Base(ctx.manifest.Provisioning.Script))
	if _, err := os.Stat(healthcheckPy); os.IsNotExist(err) {
		ctx.info("No provisioning script found — skipping healthcheck")
		return nil
	}

	ctx.info("Running healthcheck...")
	cmd := buildProvisionCommand(ctx.manifest.Provisioning.Script, "healthcheck")
	result, err := ctx.engine.cm.Exec(ctx.job.CTID, cmd)
	if err != nil {
		ctx.warn("Healthcheck error: %v", err)
		return nil // Non-fatal
	}

	parseProvisionOutput(ctx, result.Output)

	if result.ExitCode != 0 {
		ctx.warn("Healthcheck failed (exit %d)", result.ExitCode)
		return nil // Non-fatal
	}

	ctx.info("Healthcheck passed")
	return nil
}

func stepCollectOutputs(ctx *installContext) error {
	if len(ctx.manifest.Outputs) == 0 {
		ctx.info("No outputs defined")
		return nil
	}

	// Try to get container IP for template resolution
	ip, _ := ctx.engine.cm.GetIP(ctx.job.CTID)

	outputs := make(map[string]string)
	for _, out := range ctx.manifest.Outputs {
		value := out.Value
		// Replace template variables
		value = strings.ReplaceAll(value, "{{ip}}", ip)
		for k, v := range ctx.job.Inputs {
			value = strings.ReplaceAll(value, "{{"+k+"}}", v)
		}
		outputs[out.Key] = value
		ctx.info("Output: %s = %s", out.Label, value)
	}

	ctx.job.Outputs = outputs
	ctx.job.UpdatedAt = time.Now()
	ctx.engine.store.UpdateJob(ctx.job)

	return nil
}
