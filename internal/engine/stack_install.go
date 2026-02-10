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

// StartStack creates a new stack job and runs it asynchronously.
func (e *Engine) StartStack(req StackCreateRequest) (*Job, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("stack name is required")
	}
	if len(req.Apps) == 0 {
		return nil, fmt.Errorf("at least one app is required")
	}

	// Validate all apps exist and collect manifests
	manifests := make([]*catalog.AppManifest, 0, len(req.Apps))
	for _, appReq := range req.Apps {
		app, ok := e.catalog.Get(appReq.AppID)
		if !ok {
			return nil, fmt.Errorf("app %q not found in catalog", appReq.AppID)
		}
		manifests = append(manifests, app)
	}

	// Validate request inputs
	if err := ValidateHostname(req.Hostname); err != nil {
		return nil, err
	}
	if err := ValidateBridge(req.Bridge); err != nil {
		return nil, err
	}
	if err := ValidateIPAddress(req.IPAddress); err != nil {
		return nil, err
	}
	if err := ValidateEnvVars(req.EnvVars); err != nil {
		return nil, err
	}
	if err := ValidateDevices(req.Devices); err != nil {
		return nil, err
	}
	for _, hp := range req.BindMounts {
		if err := ValidateBindMountPath(hp); err != nil {
			return nil, err
		}
	}
	for _, em := range req.ExtraMounts {
		if err := ValidateBindMountPath(em.HostPath); err != nil {
			return nil, err
		}
	}

	// Validate OS template compatibility — all apps must use the same template
	osTemplate := manifests[0].LXC.OSTemplate
	for _, m := range manifests[1:] {
		if m.LXC.OSTemplate != osTemplate {
			return nil, fmt.Errorf("OS template conflict: %s uses %q, %s uses %q",
				manifests[0].ID, osTemplate, m.ID, m.LXC.OSTemplate)
		}
	}

	// Apply defaults
	storage := e.cfg.Storages[0]
	if req.Storage != "" {
		storage = req.Storage
	}
	bridge := e.cfg.Bridges[0]
	if req.Bridge != "" {
		bridge = req.Bridge
	}

	// Compute recommended resources: max cores, sum memory/disk across apps
	cores := e.cfg.Defaults.Cores
	memoryMB := 0
	diskGB := 0
	for _, m := range manifests {
		if m.LXC.Defaults.Cores > cores {
			cores = m.LXC.Defaults.Cores
		}
		memoryMB += m.LXC.Defaults.MemoryMB
		diskGB += m.LXC.Defaults.DiskGB
	}
	if req.Cores > 0 {
		cores = req.Cores
	}
	if req.MemoryMB > 0 {
		memoryMB = req.MemoryMB
	}
	if req.DiskGB > 0 {
		diskGB = req.DiskGB
	}

	hostname := req.Hostname
	if hostname == "" {
		hostname = strings.ReplaceAll(req.Name, " ", "-")
	}

	onboot := true
	if req.OnBoot != nil {
		onboot = *req.OnBoot
	}
	unprivileged := true
	if req.Unprivileged != nil {
		unprivileged = *req.Unprivileged
	}

	// Build StackApp entries
	stackApps := make([]StackApp, len(req.Apps))
	for i, appReq := range req.Apps {
		app := manifests[i]
		inputs := appReq.Inputs
		if inputs == nil {
			inputs = make(map[string]string)
		}
		// Apply input defaults from manifest
		for _, input := range app.Inputs {
			if _, exists := inputs[input.Key]; !exists && input.Default != nil {
				inputs[input.Key] = fmt.Sprintf("%v", input.Default)
			}
		}
		stackApps[i] = StackApp{
			AppID:      app.ID,
			AppName:    app.Name,
			AppVersion: app.Version,
			Order:      i,
			Inputs:     inputs,
			Outputs:    make(map[string]string),
			Status:     "pending",
		}
	}

	// Build mount points from all apps' volumes (first app wins on duplicate mount_path)
	var mountPoints []MountPoint
	seenMountPaths := make(map[string]bool)
	mpIndex := 0
	for _, m := range manifests {
		for _, vol := range m.Volumes {
			if seenMountPaths[vol.MountPath] {
				continue // first app wins
			}
			seenMountPaths[vol.MountPath] = true
			volType := vol.Type
			if volType == "" {
				volType = "volume"
			}
			mp := MountPoint{
				Index:     mpIndex,
				Name:      m.ID + "-" + vol.Name,
				Type:      volType,
				MountPath: vol.MountPath,
				SizeGB:    vol.SizeGB,
				ReadOnly:  vol.ReadOnly,
			}
			if volType == "volume" {
				if hp, ok := req.BindMounts[m.ID+"-"+vol.Name]; ok && hp != "" {
					mp.Type = "bind"
					mp.HostPath = hp
					mp.SizeGB = 0
				} else if vs, ok := req.VolumeStorages[m.ID+"-"+vol.Name]; ok && vs != "" {
					mp.Storage = vs
				}
			}
			if volType == "bind" {
				if hp, ok := req.BindMounts[m.ID+"-"+vol.Name]; ok && hp != "" {
					mp.HostPath = hp
				} else if vol.DefaultHostPath != "" {
					mp.HostPath = vol.DefaultHostPath
				}
				if !vol.Required && mp.HostPath == "" {
					continue
				}
			}
			mountPoints = append(mountPoints, mp)
			mpIndex++
		}
	}
	// Add extra user-defined mounts
	for _, em := range req.ExtraMounts {
		if em.HostPath == "" || em.MountPath == "" {
			continue
		}
		mountPoints = append(mountPoints, MountPoint{
			Index:     mpIndex,
			Name:      fmt.Sprintf("extra-%d", mpIndex),
			Type:      "bind",
			MountPath: em.MountPath,
			HostPath:  em.HostPath,
			ReadOnly:  em.ReadOnly,
		})
		mpIndex++
	}

	stackID := generateID()
	now := time.Now()

	// Create the job
	job := &Job{
		ID:           generateID(),
		Type:         JobTypeStack,
		State:        StateQueued,
		AppID:        "stack:" + req.Name,
		AppName:      req.Name,
		Node:         e.cfg.NodeName,
		Pool:         e.cfg.Pool,
		Storage:      storage,
		Bridge:       bridge,
		Cores:        cores,
		MemoryMB:     memoryMB,
		DiskGB:       diskGB,
		Hostname:     hostname,
		IPAddress:    req.IPAddress,
		OnBoot:       onboot,
		Unprivileged: unprivileged,
		Inputs:       make(map[string]string),
		Outputs:      make(map[string]string),
		MountPoints:  mountPoints,
		Devices:      req.Devices,
		EnvVars:      req.EnvVars,
		StackID:      stackID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := e.store.CreateJob(job); err != nil {
		return nil, fmt.Errorf("creating stack job: %w", err)
	}

	// Run asynchronously with cancel support
	ctx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	e.cancels[job.ID] = cancel
	e.mu.Unlock()

	go func() {
		defer func() {
			e.mu.Lock()
			delete(e.cancels, job.ID)
			e.mu.Unlock()
			cancel()
		}()
		e.runStackInstall(ctx, job, stackID, osTemplate, stackApps, manifests)
	}()

	return job, nil
}

// ValidateStack checks OS template compatibility and resource recommendations.
func (e *Engine) ValidateStack(req StackCreateRequest) map[string]interface{} {
	result := map[string]interface{}{
		"valid":    true,
		"errors":   []string{},
		"warnings": []string{},
	}

	if len(req.Apps) == 0 {
		result["valid"] = false
		result["errors"] = []string{"at least one app is required"}
		return result
	}

	var errors []string
	var warnings []string

	manifests := make([]*catalog.AppManifest, 0)
	for _, appReq := range req.Apps {
		app, ok := e.catalog.Get(appReq.AppID)
		if !ok {
			errors = append(errors, fmt.Sprintf("app %q not found", appReq.AppID))
			continue
		}
		manifests = append(manifests, app)
	}

	if len(errors) > 0 {
		result["valid"] = false
		result["errors"] = errors
		return result
	}

	// Check OS template compatibility
	osTemplate := manifests[0].LXC.OSTemplate
	for _, m := range manifests[1:] {
		if m.LXC.OSTemplate != osTemplate {
			errors = append(errors, fmt.Sprintf("OS template conflict: %s uses %q, %s uses %q",
				manifests[0].ID, osTemplate, m.ID, m.LXC.OSTemplate))
		}
	}

	// Check for mount path conflicts
	mountPaths := make(map[string]string)
	for _, m := range manifests {
		for _, vol := range m.Volumes {
			if existing, ok := mountPaths[vol.MountPath]; ok {
				warnings = append(warnings, fmt.Sprintf("mount_path %s used by both %s and %s (first wins)", vol.MountPath, existing, m.ID))
			} else {
				mountPaths[vol.MountPath] = m.ID
			}
		}
	}

	// Compute recommended resources
	recCores := 0
	recMemory := 0
	recDisk := 0
	for _, m := range manifests {
		if m.LXC.Defaults.Cores > recCores {
			recCores = m.LXC.Defaults.Cores
		}
		recMemory += m.LXC.Defaults.MemoryMB
		recDisk += m.LXC.Defaults.DiskGB
	}

	if len(errors) > 0 {
		result["valid"] = false
	}
	result["errors"] = errors
	result["warnings"] = warnings
	result["recommended"] = map[string]int{
		"cores":     recCores,
		"memory_mb": recMemory,
		"disk_gb":   recDisk,
	}
	result["ostemplate"] = osTemplate

	return result
}

// runStackInstall executes the full stack provisioning pipeline.
func (e *Engine) runStackInstall(bgCtx context.Context, job *Job, stackID, osTemplate string, apps []StackApp, manifests []*catalog.AppManifest) {
	ctx := &installContext{
		ctx:    bgCtx,
		engine: e,
		job:    job,
	}

	ctx.info("Starting stack install: %s (%d apps)", job.AppName, len(apps))

	// Step 1: Validate all manifests
	ctx.transition(StateValidateManifest)
	for _, m := range manifests {
		if err := m.Validate(); err != nil {
			ctx.log("error", "Manifest validation failed for %s: %v", m.ID, err)
			ctx.job.State = StateFailed
			ctx.job.Error = fmt.Sprintf("validate_manifest: %v", err)
			now := time.Now()
			ctx.job.UpdatedAt = now
			ctx.job.CompletedAt = &now
			e.store.UpdateJob(ctx.job)
			return
		}
	}
	ctx.info("All %d manifests validated", len(manifests))

	// Step 2: Allocate CTID (under lock to prevent races with concurrent installs)
	ctx.transition(StateAllocateCTID)
	e.ctidMu.Lock()
	ctid, err := e.cm.AllocateCTID(bgCtx)
	if err != nil {
		e.ctidMu.Unlock()
		ctx.failJob("allocate_ctid: %v", err)
		return
	}
	ctx.job.CTID = ctid
	ctx.job.UpdatedAt = time.Now()
	e.store.UpdateJob(ctx.job)
	ctx.info("Allocated CTID: %d", ctid)

	// Step 3: Resolve template and create container (ctidMu released after Create)
	ctx.transition(StateCreateContainer)
	template := osTemplate
	if !strings.Contains(template, ":") {
		ctx.info("Resolving template %q...", template)
		template = e.cm.ResolveTemplate(bgCtx, template, job.Storage)
	}

	// Merge features from all manifests
	featureSet := make(map[string]bool)
	for _, m := range manifests {
		for _, f := range m.LXC.Defaults.Features {
			featureSet[f] = true
		}
	}
	var features []string
	for f := range featureSet {
		features = append(features, f)
	}

	opts := CreateOptions{
		CTID:         ctid,
		OSTemplate:   template,
		Storage:      job.Storage,
		RootFSSize:   job.DiskGB,
		Cores:        job.Cores,
		MemoryMB:     job.MemoryMB,
		Bridge:       job.Bridge,
		Hostname:     job.Hostname,
		IPAddress:    job.IPAddress,
		Unprivileged: job.Unprivileged,
		Pool:         job.Pool,
		Features:     features,
		OnBoot:       job.OnBoot,
		Tags:         buildTags("appstore;stack;managed", job.ExtraTags),
	}

	// Add mount points
	for _, mp := range job.MountPoints {
		mpStorage := mp.Storage
		if mpStorage == "" {
			mpStorage = job.Storage
		}
		opts.MountPoints = append(opts.MountPoints, MountPointOption{
			Index:     mp.Index,
			Type:      mp.Type,
			MountPath: mp.MountPath,
			Storage:   mpStorage,
			SizeGB:    mp.SizeGB,
			VolumeID:  mp.VolumeID,
			HostPath:  mp.HostPath,
			ReadOnly:  mp.ReadOnly,
		})
	}

	ctx.info("Creating container %d (template=%s, %d cores, %d MB, %d GB)", ctid, template, opts.Cores, opts.MemoryMB, opts.RootFSSize)
	if err := e.cm.Create(bgCtx, opts); err != nil {
		e.ctidMu.Unlock()
		ctx.failJob("create_container: %v", err)
		return
	}
	e.ctidMu.Unlock()
	ctx.info("Container %d created", ctid)

	// Configure device passthrough via pct set (bypasses API root@pam restriction)
	if len(job.Devices) > 0 {
		ctx.info("Configuring %d device passthrough(s)...", len(job.Devices))
		if err := e.cm.ConfigureDevices(ctid, job.Devices); err != nil {
			ctx.failJob("configure_devices: %v", err)
			return
		}

		// Bind-mount NVIDIA libraries if NVIDIA devices are present
		if hasNvidiaDevices(job.Devices) {
			libPath, err := resolveNvidiaLibPath()
			if err != nil {
				ctx.warn("Could not resolve NVIDIA libraries: %v", err)
			} else if libPath != "" {
				nextMP := len(job.MountPoints)
				ctx.info("Bind-mounting NVIDIA libraries from %s (mp%d)", libPath, nextMP)
				if err := e.cm.MountHostPath(ctid, nextMP, libPath, nvidiaContainerLibPath, true); err != nil {
					ctx.warn("Failed to mount NVIDIA libraries: %v", err)
				}
			}
		}
	}

	// Step 4: Start container
	ctx.transition(StateStartContainer)
	if err := e.cm.Start(bgCtx, ctid); err != nil {
		ctx.failJob("start_container: %v", err)
		return
	}
	ctx.info("Container %d started", ctid)

	// Step 5: Wait for network
	ctx.transition(StateWaitForNetwork)
	ctx.info("Waiting for network...")
	for i := 0; i < 30; i++ {
		ip, err := e.cm.GetIP(ctid)
		if err == nil && ip != "" && ip != "127.0.0.1" {
			ctx.info("Container %d has IP: %s", ctid, ip)
			break
		}
		time.Sleep(2 * time.Second)
	}

	// Step 5.5: Setup GPU runtime if NVIDIA devices are present
	if hasNvidiaDevices(job.Devices) {
		ctx.info("Setting up NVIDIA runtime inside container...")
		ldconfContent := nvidiaContainerLibPath + "\n"
		for _, cmd := range [][]string{
			{"mkdir", "-p", "/etc/ld.so.conf.d"},
			{"sh", "-c", fmt.Sprintf("echo '%s' > %s", ldconfContent, nvidiaLdconfPath)},
			{"ldconfig"},
		} {
			if result, err := e.cm.Exec(ctid, cmd); err != nil {
				ctx.warn("GPU setup command failed: %v", err)
			} else if result.ExitCode != 0 {
				ctx.warn("GPU setup command %v exited %d", cmd, result.ExitCode)
			}
		}
		ctx.info("NVIDIA runtime configured")
	}

	// Step 5.6: Install base packages
	ctx.transition(StateInstallBasePkgs)
	if err := stepInstallBasePackages(ctx); err != nil {
		ctx.failJob("install_base_packages: %v", err)
		return
	}

	// Step 6: Push SDK once
	ctx.transition(StatePushAssets)
	ctx.info("Verifying python3...")
	if err := ensurePython(ctid, e.cm); err != nil {
		ctx.failJob("push_assets: ensuring python3: %v", err)
		return
	}
	ctx.info("Pushing Python SDK...")
	if err := pushSDK(ctid, e.cm); err != nil {
		ctx.failJob("push_assets: pushing SDK: %v", err)
		return
	}

	// Merge permissions from all apps (union)
	mergedPerms := mergeAllPermissions(manifests)
	if err := pushPermissionsJSON(ctid, e.cm, mergedPerms); err != nil {
		ctx.failJob("push_assets: pushing permissions: %v", err)
		return
	}
	ctx.info("SDK and permissions pushed")

	// Step 7: Provision each app in order
	ctx.transition(StateProvision)
	allOutputs := make(map[string]string)

	for i, app := range apps {
		select {
		case <-bgCtx.Done():
			ctx.warn("Job cancelled by user")
			e.cleanupCancelledJob(ctx)
			return
		default:
		}

		manifest := manifests[i]
		apps[i].Status = "provisioning"
		ctx.info("[%d/%d] Provisioning: %s", i+1, len(apps), app.AppName)

		// Push per-app provision directory
		appProvisionDir := filepath.Join(manifest.DirPath, "provision")
		if _, err := os.Stat(appProvisionDir); os.IsNotExist(err) {
			apps[i].Status = "failed"
			apps[i].Error = "provision directory not found"
			ctx.warn("[%d/%d] %s: provision directory not found — skipping", i+1, len(apps), app.AppName)
			continue
		}

		targetProvisionDir := "/opt/appstore/provision/" + app.AppID
		e.cm.Exec(ctid, []string{"mkdir", "-p", targetProvisionDir})

		entries, err := os.ReadDir(appProvisionDir)
		if err != nil {
			apps[i].Status = "failed"
			apps[i].Error = fmt.Sprintf("reading provision dir: %v", err)
			ctx.warn("[%d/%d] %s: %v — continuing", i+1, len(apps), app.AppName, err)
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			src := filepath.Join(appProvisionDir, entry.Name())
			dst := targetProvisionDir + "/" + entry.Name()
			if err := e.cm.Push(ctid, src, dst, "0755"); err != nil {
				ctx.warn("Failed to push %s: %v", entry.Name(), err)
			}
		}

		// Push templates if they exist
		templatesDir := filepath.Join(manifest.DirPath, "templates")
		if _, err := os.Stat(templatesDir); err == nil {
			e.cm.Exec(ctid, []string{"mkdir", "-p", "/opt/appstore/templates"})
			tEntries, _ := os.ReadDir(templatesDir)
			for _, entry := range tEntries {
				if entry.IsDir() {
					continue
				}
				src := filepath.Join(templatesDir, entry.Name())
				dst := "/opt/appstore/templates/" + entry.Name()
				e.cm.Push(ctid, src, dst, "0644")
			}
		}

		// Push per-app inputs
		appInputsPath := "/opt/appstore/" + app.AppID + "/inputs.json"
		e.cm.Exec(ctid, []string{"mkdir", "-p", "/opt/appstore/" + app.AppID})
		if err := pushInputsJSONToPath(ctid, e.cm, app.Inputs, appInputsPath); err != nil {
			apps[i].Status = "failed"
			apps[i].Error = fmt.Sprintf("pushing inputs: %v", err)
			ctx.warn("[%d/%d] %s: failed to push inputs — continuing", i+1, len(apps), app.AppName)
			continue
		}

		// Run provisioning
		envVars := mergeEnvVars(manifest.Provisioning.Env, job.EnvVars)
		cmd := buildStackProvisionCommand(app.AppID, manifest.Provisioning.Script, "install", envVars)

		appOutputs := make(map[string]string)
		result, err := e.cm.ExecStream(ctid, cmd, func(line string) {
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
						appOutputs[key] = value
					}
				} else if msg != "" {
					ctx.log(level, "[%s] %s", app.AppID, msg)
				}
			} else {
				ctx.info("[%s] %s", app.AppID, line)
			}
		})

		if err != nil {
			apps[i].Status = "failed"
			apps[i].Error = fmt.Sprintf("exec: %v", err)
			ctx.warn("[%d/%d] %s: provisioning error: %v — continuing", i+1, len(apps), app.AppName, err)
			continue
		}

		if result.ExitCode != 0 {
			apps[i].Status = "failed"
			apps[i].Error = fmt.Sprintf("exit code %d", result.ExitCode)
			ctx.warn("[%d/%d] %s: provisioning failed (exit %d) — continuing", i+1, len(apps), app.AppName, result.ExitCode)
			continue
		}

		apps[i].Status = "completed"
		apps[i].Outputs = appOutputs
		// Namespace outputs: {app-id}.{output-key}
		for k, v := range appOutputs {
			allOutputs[app.AppID+"."+k] = v
		}
		ctx.info("[%d/%d] %s: provisioning completed", i+1, len(apps), app.AppName)
	}

	// Collect manifest outputs with template resolution
	ip, _ := e.cm.GetIP(ctid)
	for i, app := range apps {
		manifest := manifests[i]
		for _, out := range manifest.Outputs {
			value := out.Value
			value = strings.ReplaceAll(value, "{{ip}}", ip)
			for k, v := range app.Inputs {
				value = strings.ReplaceAll(value, "{{"+k+"}}", v)
			}
			allOutputs[app.AppID+"."+out.Key] = value
			if apps[i].Outputs == nil {
				apps[i].Outputs = make(map[string]string)
			}
			apps[i].Outputs[out.Key] = value
		}
	}

	// Update job outputs
	job.Outputs = allOutputs
	job.UpdatedAt = time.Now()
	e.store.UpdateJob(job)

	// Success — create stack record
	ctx.transition(StateCompleted)
	now := time.Now()
	ctx.job.CompletedAt = &now
	e.store.UpdateJob(ctx.job)

	stack := &Stack{
		ID:           stackID,
		Name:         job.AppName,
		CTID:         ctid,
		Node:         job.Node,
		Pool:         job.Pool,
		Storage:      job.Storage,
		Bridge:       job.Bridge,
		Cores:        job.Cores,
		MemoryMB:     job.MemoryMB,
		DiskGB:       job.DiskGB,
		Hostname:     job.Hostname,
		IPAddress:    job.IPAddress,
		OnBoot:       job.OnBoot,
		Unprivileged: job.Unprivileged,
		OSTemplate:   osTemplate,
		Apps:         apps,
		MountPoints:  job.MountPoints,
		Devices:      job.Devices,
		EnvVars:      job.EnvVars,
		Status:       "running",
		CreatedAt:    now,
	}
	e.store.CreateStack(stack)

	succeededCount := 0
	for _, app := range apps {
		if app.Status == "completed" {
			succeededCount++
		}
	}
	ctx.info("Stack install complete! %d/%d apps provisioned. Container %d is running.", succeededCount, len(apps), ctid)
}

// failJob is a helper to mark a job as failed.
func (ctx *installContext) failJob(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	ctx.log("error", "Failed: %s", msg)
	ctx.job.State = StateFailed
	ctx.job.Error = msg
	now := time.Now()
	ctx.job.UpdatedAt = now
	ctx.job.CompletedAt = &now
	ctx.engine.store.UpdateJob(ctx.job)
}

// mergeAllPermissions computes the union of all apps' permission specs.
func mergeAllPermissions(manifests []*catalog.AppManifest) catalog.PermissionsSpec {
	merged := catalog.PermissionsSpec{}
	seen := map[string]map[string]bool{
		"packages": {}, "pip": {}, "urls": {}, "paths": {},
		"services": {}, "users": {}, "commands": {},
		"installer_scripts": {}, "apt_repos": {},
	}

	addUnique := func(field string, items []string, target *[]string) {
		for _, item := range items {
			if !seen[field][item] {
				seen[field][item] = true
				*target = append(*target, item)
			}
		}
	}

	for _, m := range manifests {
		addUnique("packages", m.Permissions.Packages, &merged.Packages)
		addUnique("pip", m.Permissions.Pip, &merged.Pip)
		addUnique("urls", m.Permissions.URLs, &merged.URLs)
		addUnique("paths", m.Permissions.Paths, &merged.Paths)
		addUnique("services", m.Permissions.Services, &merged.Services)
		addUnique("users", m.Permissions.Users, &merged.Users)
		addUnique("commands", m.Permissions.Commands, &merged.Commands)
		addUnique("installer_scripts", m.Permissions.InstallerScripts, &merged.InstallerScripts)
		addUnique("apt_repos", m.Permissions.AptRepos, &merged.AptRepos)
	}

	return merged
}
