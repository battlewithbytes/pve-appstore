package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
)

// installSteps defines the ordered state machine for an install job.
// Each step returns the next state or an error (which moves to "failed").
// StateReadVolumeIDs is the state for reading volume IDs after container creation.
const StateReadVolumeIDs = "read_volume_ids"

// StateSetupGPURuntime is the state for configuring GPU libraries inside the container.
const StateSetupGPURuntime = "setup_gpu_runtime"

var installSteps = []struct {
	state string
	fn    func(ctx *installContext) error
}{
	{StateValidateRequest, stepValidateRequest},
	{StateValidateManifest, stepValidateManifest},
	{StateValidatePlacement, stepValidatePlacement},
	{StateAllocateCTID, stepAllocateCTID},
	{StateCreateContainer, stepCreateContainer},
	{StateReadVolumeIDs, stepReadVolumeIDs},
	{StateConfigureContainer, stepConfigureContainer},
	{StateStartContainer, stepStartContainer},
	{StateWaitForNetwork, stepWaitForNetwork},
	{StateSetupGPURuntime, stepSetupGPURuntime},
	{StateInstallBasePkgs, stepInstallBasePackages},
	{StatePushAssets, stepPushAssets},
	{StateProvision, stepProvision},
	{StateHealthcheck, stepHealthcheck},
	{StateCollectOutputs, stepCollectOutputs},
}

// installContext carries state through the install pipeline.
type installContext struct {
	ctx        context.Context
	engine     *Engine
	job        *Job
	manifest   *catalog.AppManifest
	hwAddr     string // MAC address to preserve across recreates
	ctidLocked bool   // true when ctidMu is held between allocate→create
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
func (e *Engine) runInstall(bgCtx context.Context, job *Job) {
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
		ctx:      bgCtx,
		engine:   e,
		job:      job,
		manifest: app,
	}

	ctx.info("Starting install of %s (%s)", app.Name, app.ID)

	// releaseCTIDLock ensures the CTID mutex is released if the pipeline
	// exits between stepAllocateCTID and stepCreateContainer (cancel/panic).
	releaseCTIDLock := func() {
		if ctx.ctidLocked {
			ctx.engine.ctidMu.Unlock()
			ctx.ctidLocked = false
		}
	}

	for _, step := range installSteps {
		// Check for cancellation between steps
		select {
		case <-bgCtx.Done():
			releaseCTIDLock()
			ctx.warn("Job cancelled by user")
			e.cleanupCancelledJob(ctx)
			return
		default:
		}

		ctx.transition(step.state)

		if err := step.fn(ctx); err != nil {
			releaseCTIDLock()
			// Check if the error was due to cancellation
			if bgCtx.Err() != nil {
				ctx.warn("Job cancelled by user during %s", step.state)
				e.cleanupCancelledJob(ctx)
				return
			}
			ctx.log("error", "Failed at %s: %v", step.state, err)
			ctx.job.State = StateFailed
			ctx.job.Error = fmt.Sprintf("%s: %v", step.state, err)
			now := time.Now()
			ctx.job.UpdatedAt = now
			ctx.job.CompletedAt = &now
			e.store.UpdateJob(ctx.job)
			// Destroy the container if one was created
			if ctx.job.CTID > 0 {
				ctx.info("Cleaning up container %d after failure...", ctx.job.CTID)
				cleanCtx := context.Background()
				_ = e.cm.Stop(cleanCtx, ctx.job.CTID)
				time.Sleep(2 * time.Second)
				if dErr := e.cm.Destroy(cleanCtx, ctx.job.CTID); dErr != nil {
					ctx.warn("Failed to destroy container %d: %v", ctx.job.CTID, dErr)
				} else {
					ctx.info("Container %d destroyed", ctx.job.CTID)
				}
			}
			return
		}
	}

	// Success
	ctx.transition(StateCompleted)
	now := time.Now()
	ctx.job.CompletedAt = &now
	e.store.UpdateJob(ctx.job)

	// Record the installation with all enriched fields
	e.store.CreateInstall(&Install{
		ID:           ctx.job.ID,
		AppID:        ctx.job.AppID,
		AppName:      ctx.job.AppName,
		AppVersion:   ctx.manifest.Version,
		CTID:         ctx.job.CTID,
		Node:         ctx.job.Node,
		Pool:         ctx.job.Pool,
		Storage:      ctx.job.Storage,
		Bridge:       ctx.job.Bridge,
		Cores:        ctx.job.Cores,
		MemoryMB:     ctx.job.MemoryMB,
		DiskGB:       ctx.job.DiskGB,
		Hostname:     ctx.job.Hostname,
		IPAddress:    ctx.job.IPAddress,
		OnBoot:       ctx.job.OnBoot,
		Unprivileged: ctx.job.Unprivileged,
		Inputs:       ctx.job.Inputs,
		Outputs:      ctx.job.Outputs,
		MountPoints:  ctx.job.MountPoints,
		Devices:      ctx.job.Devices,
		EnvVars:      ctx.job.EnvVars,
		Status:       "running",
		CreatedAt:    now,
	})

	ctx.info("Install complete! Container %d is running.", ctx.job.CTID)
}

func stepValidateRequest(ctx *installContext) error {
	if ctx.job.AppID == "" {
		return fmt.Errorf("app_id is required")
	}

	// Validate user inputs against manifest rules
	if err := validateInputs(ctx.manifest, ctx.job.Inputs); err != nil {
		return err
	}

	ctx.info("Request validated for app %s", ctx.job.AppID)
	return nil
}

// validateInputs checks user-supplied input values against manifest InputSpec rules.
func validateInputs(manifest *catalog.AppManifest, inputs map[string]string) error {
	for _, inp := range manifest.Inputs {
		val, provided := inputs[inp.Key]

		// Required check
		if inp.Required && (!provided || val == "") {
			return fmt.Errorf("input %q is required", inp.Key)
		}

		// Skip further validation if not provided or empty
		if !provided || val == "" {
			continue
		}

		v := inp.Validation
		if v == nil {
			continue
		}

		switch inp.Type {
		case "number":
			num, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return fmt.Errorf("input %q: invalid number %q", inp.Key, val)
			}
			if v.Min != nil && num < *v.Min {
				return fmt.Errorf("input %q: value %g is below minimum %g", inp.Key, num, *v.Min)
			}
			if v.Max != nil && num > *v.Max {
				return fmt.Errorf("input %q: value %g exceeds maximum %g", inp.Key, num, *v.Max)
			}
		case "string", "secret":
			if v.MinLength != nil && len(val) < *v.MinLength {
				return fmt.Errorf("input %q: must be at least %d characters (got %d)", inp.Key, *v.MinLength, len(val))
			}
			if v.MaxLength != nil && len(val) > *v.MaxLength {
				return fmt.Errorf("input %q: must be at most %d characters (got %d)", inp.Key, *v.MaxLength, len(val))
			}
			if v.Regex != "" {
				re, err := regexp.Compile(v.Regex)
				if err != nil {
					return fmt.Errorf("input %q: invalid regex %q: %w", inp.Key, v.Regex, err)
				}
				if !re.MatchString(val) {
					return fmt.Errorf("input %q: value does not match pattern %q", inp.Key, v.Regex)
				}
			}
		}

		if len(v.Enum) > 0 {
			found := false
			for _, e := range v.Enum {
				if val == e {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("input %q: value %q is not one of the allowed options", inp.Key, val)
			}
		}
	}
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
	// Lock to serialize CTID allocation + container creation across concurrent jobs.
	// Proxmox /cluster/nextid returns the same ID until a container is actually created,
	// so we must hold the lock until Create() completes in the next step.
	ctx.engine.ctidMu.Lock()
	ctx.ctidLocked = true

	ctid, err := ctx.engine.cm.AllocateCTID(context.Background())
	if err != nil {
		ctx.engine.ctidMu.Unlock()
		ctx.ctidLocked = false
		return fmt.Errorf("allocating CTID: %w", err)
	}
	ctx.job.CTID = ctid
	ctx.job.UpdatedAt = time.Now()
	ctx.engine.store.UpdateJob(ctx.job)
	ctx.info("Allocated CTID: %d", ctid)
	return nil
}

func stepCreateContainer(ctx *installContext) error {
	// Unlock ctidMu when done — acquired in stepAllocateCTID.
	defer func() {
		ctx.engine.ctidMu.Unlock()
		ctx.ctidLocked = false
	}()

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
		HWAddr:       ctx.hwAddr,
		Hostname:     ctx.job.Hostname,
		IPAddress:    ctx.job.IPAddress,
		Unprivileged: ctx.job.Unprivileged,
		Pool:         ctx.job.Pool,
		Features:     ctx.manifest.LXC.Defaults.Features,
		OnBoot:       ctx.job.OnBoot,
		Tags:         buildTags("appstore;managed", ctx.job.ExtraTags),
	}

	// Mount points are pre-built on the job (by StartInstall or Reinstall)
	for _, mp := range ctx.job.MountPoints {
		mpStorage := mp.Storage
		if mpStorage == "" {
			mpStorage = ctx.job.Storage
		}
		opt := MountPointOption{
			Index:     mp.Index,
			Type:      mp.Type,
			MountPath: mp.MountPath,
			Storage:   mpStorage,
			SizeGB:    mp.SizeGB,
			VolumeID:  mp.VolumeID,
			HostPath:  mp.HostPath,
			ReadOnly:  mp.ReadOnly,
		}
		opts.MountPoints = append(opts.MountPoints, opt)
	}
	if len(ctx.job.MountPoints) > 0 {
		ctx.info("Configuring %d mount(s)", len(ctx.job.MountPoints))
	}

	ctx.info("Creating container %d (template=%s, %d cores, %d MB, %d GB)",
		opts.CTID, template, opts.Cores, opts.MemoryMB, opts.RootFSSize)

	if err := ctx.engine.cm.Create(context.Background(), opts); err != nil {
		return err
	}

	ctx.info("Container %d created", ctx.job.CTID)
	return nil
}

func stepReadVolumeIDs(ctx *installContext) error {
	// Only need to read volume IDs for managed volumes (not bind mounts)
	hasManagedVolumes := false
	for _, mp := range ctx.job.MountPoints {
		if mp.Type == "volume" {
			hasManagedVolumes = true
			break
		}
	}
	if !hasManagedVolumes {
		return nil
	}

	config, err := ctx.engine.cm.GetConfig(context.Background(), ctx.job.CTID)
	if err != nil {
		ctx.warn("Failed to read container config for volume IDs: %v", err)
		return nil // non-fatal
	}

	for i := range ctx.job.MountPoints {
		if ctx.job.MountPoints[i].Type == "bind" {
			continue
		}
		key := fmt.Sprintf("mp%d", ctx.job.MountPoints[i].Index)
		val, ok := config[key]
		if !ok {
			continue
		}
		// Value format: "local-lvm:vm-104-disk-1,mp=/mnt/media,..." — extract the volume ID
		valStr, ok := val.(string)
		if !ok {
			continue
		}
		parts := strings.SplitN(valStr, ",", 2)
		if len(parts) > 0 {
			ctx.job.MountPoints[i].VolumeID = parts[0]
			ctx.info("Volume %s (%s): %s", ctx.job.MountPoints[i].Name, key, parts[0])
		}
	}

	ctx.engine.store.UpdateJob(ctx.job)
	return nil
}

func stepConfigureContainer(ctx *installContext) error {
	// Apply device passthrough via pct set (bypasses Proxmox API root@pam restriction)
	if len(ctx.job.Devices) > 0 {
		ctx.info("Configuring %d device passthrough(s) via pct set...", len(ctx.job.Devices))
		if err := ctx.engine.cm.ConfigureDevices(ctx.job.CTID, ctx.job.Devices); err != nil {
			return fmt.Errorf("configuring devices: %w", err)
		}

		// If NVIDIA devices are present, bind-mount host NVIDIA libraries into the container
		if hasNvidiaDevices(ctx.job.Devices) {
			libPath, err := resolveNvidiaLibPath()
			if err != nil {
				ctx.warn("Could not resolve NVIDIA libraries: %v — GPU may not work inside container", err)
			} else if libPath != "" {
				// Find next free mount point index (after job's mount points)
				nextMP := len(ctx.job.MountPoints)
				ctx.info("Bind-mounting NVIDIA libraries from %s to %s (mp%d)", libPath, nvidiaContainerLibPath, nextMP)
				if err := ctx.engine.cm.MountHostPath(ctx.job.CTID, nextMP, libPath, nvidiaContainerLibPath, true); err != nil {
					ctx.warn("Failed to mount NVIDIA libraries: %v — GPU may not work inside container", err)
				}
			}
		}
	}
	// Apply extra LXC config lines from the manifest (e.g. TUN device, cgroup rules)
	if len(ctx.manifest.LXC.ExtraConfig) > 0 {
		if err := ValidateExtraConfig(ctx.manifest.LXC.ExtraConfig); err != nil {
			return fmt.Errorf("extra LXC config validation failed: %w", err)
		}

		ctx.info("Applying %d extra LXC config line(s)...", len(ctx.manifest.LXC.ExtraConfig))
		if err := ctx.engine.cm.AppendLXCConfig(ctx.job.CTID, ctx.manifest.LXC.ExtraConfig); err != nil {
			return fmt.Errorf("applying extra LXC config: %w", err)
		}
	}

	ctx.info("Container %d configured", ctx.job.CTID)
	return nil
}

// stepSetupGPURuntime configures the GPU runtime inside the container after it starts.
// For NVIDIA: creates an ldconfig entry so the container can find the mounted libraries.
func stepSetupGPURuntime(ctx *installContext) error {
	if !hasNvidiaDevices(ctx.job.Devices) {
		return nil // no NVIDIA devices, nothing to do
	}

	ctx.info("Setting up NVIDIA runtime inside container...")

	// Create ldconfig entry for the mounted library path
	ldconfContent := nvidiaContainerLibPath + "\n"
	cmds := [][]string{
		{"mkdir", "-p", "/etc/ld.so.conf.d"},
		{"sh", "-c", fmt.Sprintf("echo '%s' > %s", ldconfContent, nvidiaLdconfPath)},
		{"ldconfig"},
	}

	for _, cmd := range cmds {
		result, err := ctx.engine.cm.Exec(ctx.job.CTID, cmd)
		if err != nil {
			ctx.warn("GPU runtime setup command failed: %v", err)
			return nil // non-fatal
		}
		if result.ExitCode != 0 {
			ctx.warn("GPU runtime command %v exited %d: %s", cmd, result.ExitCode, result.Output)
		}
	}

	// Verify NVIDIA is accessible
	result, err := ctx.engine.cm.Exec(ctx.job.CTID, []string{"nvidia-smi", "--query-gpu=name", "--format=csv,noheader"})
	if err == nil && result.ExitCode == 0 {
		gpuName := strings.TrimSpace(result.Output)
		ctx.info("NVIDIA GPU accessible inside container: %s", gpuName)
	} else {
		// nvidia-smi may not be in the mounted libs; try via library check
		result2, err2 := ctx.engine.cm.Exec(ctx.job.CTID, []string{"ldconfig", "-p"})
		if err2 == nil && strings.Contains(result2.Output, "libcuda") {
			ctx.info("NVIDIA CUDA libraries available (ldconfig confirms libcuda)")
		} else {
			ctx.warn("NVIDIA GPU libraries may not be fully available inside container")
		}
	}

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

// mergeEnvVars combines manifest and job env vars; job overrides manifest.
func mergeEnvVars(manifest map[string]string, job map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range manifest {
		merged[k] = v
	}
	for k, v := range job {
		merged[k] = v
	}
	return merged
}

func stepProvision(ctx *installContext) error {
	envVars := mergeEnvVars(ctx.manifest.Provisioning.Env, ctx.job.EnvVars)
	cmd := buildProvisionCommand(ctx.manifest.Provisioning.Script, "install", envVars)

	ctx.info("Running provisioning: python3 -m appstore.runner ... install %s",
		filepath.Base(ctx.manifest.Provisioning.Script))

	// Track the last error message from the SDK for inclusion in the error
	var lastError string

	// Stream output line-by-line for real-time log feedback
	result, err := ctx.engine.cm.ExecStream(ctx.job.CTID, cmd, func(line string) {
		parseProvisionLine(ctx, line)
		// Capture error-level messages so we can include them in the Go error
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "@@APPLOG@@") {
			jsonStr := strings.TrimPrefix(trimmed, "@@APPLOG@@")
			if extractJSONField(jsonStr, "level") == "error" {
				if msg := extractJSONField(jsonStr, "msg"); msg != "" {
					lastError = msg
				}
			}
		}
	})
	if err != nil {
		return fmt.Errorf("executing provision script: %w", err)
	}

	if result.ExitCode != 0 {
		if lastError != "" {
			return fmt.Errorf("provision failed: %s", lastError)
		}
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
	envVars := mergeEnvVars(ctx.manifest.Provisioning.Env, ctx.job.EnvVars)
	cmd := buildProvisionCommand(ctx.manifest.Provisioning.Script, "healthcheck", envVars)
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
		// Replace template variables (support both {{IP}} and {{ip}})
		value = strings.ReplaceAll(value, "{{IP}}", ip)
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

// cleanupCancelledJob handles cleanup when a job is cancelled.
// If a container was allocated, attempt to stop and destroy it.
func (e *Engine) cleanupCancelledJob(ctx *installContext) {
	ctx.job.State = StateCancelled
	ctx.job.Error = "cancelled by user"
	now := time.Now()
	ctx.job.UpdatedAt = now
	ctx.job.CompletedAt = &now
	e.store.UpdateJob(ctx.job)

	// If a CTID was allocated, try to clean up the container
	if ctx.job.CTID > 0 {
		ctx.info("Cleaning up container %d...", ctx.job.CTID)
		cleanCtx := context.Background() // fresh context since the job context is cancelled
		// Try stop first, ignore errors (may not be running)
		_ = e.cm.Stop(cleanCtx, ctx.job.CTID)
		time.Sleep(2 * time.Second)
		if err := e.cm.Destroy(cleanCtx, ctx.job.CTID); err != nil {
			ctx.warn("Failed to destroy container %d during cancel cleanup: %v", ctx.job.CTID, err)
		} else {
			ctx.info("Container %d destroyed", ctx.job.CTID)
		}
	}

	ctx.info("Job cancelled and cleaned up")
}

// buildTags appends extra semicolon-separated tags to a base tag string.
func buildTags(base, extra string) string {
	if extra == "" {
		return base
	}
	return base + ";" + extra
}
