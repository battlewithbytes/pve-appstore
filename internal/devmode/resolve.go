package devmode

import (
	"fmt"
	"net/url"
	"strings"
)

// ChainEvent represents a progress event during chain resolution.
type ChainEvent struct {
	Type     string `json:"type"`              // "fetching", "parsed", "terminal", "error", "merged", "complete"
	Layer    int    `json:"layer"`             // layer index (0 = app layer)
	Image    string `json:"image,omitempty"`   // image reference
	URL      string `json:"url,omitempty"`     // URL being fetched
	Message  string `json:"message"`           // human-readable message
	Packages int    `json:"packages,omitempty"`
	Ports    int    `json:"ports,omitempty"`
	Volumes  int    `json:"volumes,omitempty"`
	AppID    string `json:"app_id,omitempty"` // only on "complete"
}

// DockerfileFetcher abstracts HTTP fetching for testability.
type DockerfileFetcher interface {
	// FetchDockerfile tries to fetch a Dockerfile from the given URL template.
	// urlTmpl contains {branch} placeholder. Returns content, the resolved URL used, and any error.
	FetchDockerfile(urlTmpl, branch string) (content, usedURL string, err error)
}

// ResolveDockerfileChain recursively resolves a Dockerfile's FROM chain,
// fetching parent Dockerfiles until a terminal base image is reached.
// It emits ChainEvent callbacks for progress reporting.
// Returns the merged DockerfileInfo with packages from all layers.
func ResolveDockerfileChain(
	fetcher DockerfileFetcher,
	initialContent string,
	maxDepth int,
	onEvent func(ChainEvent),
) *DockerfileInfo {
	if onEvent == nil {
		onEvent = func(ChainEvent) {}
	}

	// Parse the initial (app) layer
	info := ParseDockerfile(initialContent)
	onEvent(ChainEvent{
		Type:     "parsed",
		Layer:    0,
		Image:    info.BaseImage,
		Message:  fmt.Sprintf("Layer 0: %d packages, %d ports, %d volumes", len(info.Packages), len(info.Ports), len(info.Volumes)),
		Packages: len(info.Packages),
		Ports:    len(info.Ports),
		Volumes:  len(info.Volumes),
	})

	// Check if the base image is terminal
	if IsTerminalBaseImage(info.BaseImage) {
		onEvent(ChainEvent{
			Type:    "terminal",
			Layer:   0,
			Image:   info.BaseImage,
			Message: fmt.Sprintf("Base OS: %s (terminal)", friendlyOS(info)),
		})
		return info
	}

	// Try to resolve parent
	parentInfo := resolveParent(fetcher, info.BaseImage, 1, maxDepth, onEvent)
	if parentInfo == nil {
		return info
	}

	// Merge parent-first: [parent, app]
	merged := MergeDockerfileInfoChain([]*DockerfileInfo{parentInfo, info})
	onEvent(ChainEvent{
		Type:     "merged",
		Message:  fmt.Sprintf("Merged layers: %d packages total", len(merged.Packages)),
		Packages: len(merged.Packages),
		Ports:    len(merged.Ports),
		Volumes:  len(merged.Volumes),
	})

	return merged
}

// resolveParent recursively fetches and parses parent Dockerfiles.
func resolveParent(
	fetcher DockerfileFetcher,
	image string,
	layer int,
	maxDepth int,
	onEvent func(ChainEvent),
) *DockerfileInfo {
	if maxDepth <= 0 {
		onEvent(ChainEvent{
			Type:    "error",
			Layer:   layer,
			Image:   image,
			Message: "Max depth reached, stopping chain resolution",
		})
		return nil
	}

	// Infer the URL for this parent image
	urlTmpl, branch := InferDockerfileURL(image, "")
	if urlTmpl == "" {
		onEvent(ChainEvent{
			Type:    "error",
			Layer:   layer,
			Image:   image,
			Message: fmt.Sprintf("Cannot resolve Dockerfile URL for %s", shortImage(image)),
		})
		return nil
	}

	onEvent(ChainEvent{
		Type:    "fetching",
		Layer:   layer,
		Image:   shortImage(image),
		URL:     urlTmpl,
		Message: fmt.Sprintf("Fetching parent: %s", shortImage(image)),
	})

	content, _, err := fetcher.FetchDockerfile(urlTmpl, branch)
	if err != nil {
		onEvent(ChainEvent{
			Type:    "error",
			Layer:   layer,
			Image:   shortImage(image),
			Message: fmt.Sprintf("Failed to fetch %s: %v", shortImage(image), err),
		})
		return nil
	}

	// Parse this parent layer
	info := ParseDockerfile(content)
	if repo := repoURLFromTemplate(urlTmpl); repo != "" {
		info.RepoURL = repo
	}

	onEvent(ChainEvent{
		Type:     "parsed",
		Layer:    layer,
		Image:    shortImage(image),
		Message:  fmt.Sprintf("Layer %d (%s): %d packages, %d ports", layer, shortImage(image), len(info.Packages), len(info.Ports)),
		Packages: len(info.Packages),
		Ports:    len(info.Ports),
		Volumes:  len(info.Volumes),
	})

	// Check if this parent's base is terminal
	if IsTerminalBaseImage(info.BaseImage) {
		onEvent(ChainEvent{
			Type:    "terminal",
			Layer:   layer + 1,
			Image:   info.BaseImage,
			Message: fmt.Sprintf("Base OS: %s (terminal)", friendlyOS(info)),
		})
		return info
	}

	// Recurse to grandparent
	grandparent := resolveParent(fetcher, info.BaseImage, layer+1, maxDepth-1, onEvent)
	if grandparent == nil {
		return info
	}

	// Merge grandparent + this parent
	return MergeDockerfileInfoChain([]*DockerfileInfo{grandparent, info})
}

// MergeDockerfileInfoChain merges multiple DockerfileInfo layers.
// Layers are ordered parent-first: index 0 = deepest parent, last = app layer.
func MergeDockerfileInfoChain(layers []*DockerfileInfo) *DockerfileInfo {
	if len(layers) == 0 {
		return &DockerfileInfo{BaseOS: "unknown"}
	}
	if len(layers) == 1 {
		return layers[0]
	}

	merged := &DockerfileInfo{}

	// BaseOS from deepest parent (index 0)
	merged.BaseOS = layers[0].BaseOS

	// BaseImage from the app layer (last)
	merged.BaseImage = layers[len(layers)-1].BaseImage

	// ExecCmd from app layer only
	merged.ExecCmd = layers[len(layers)-1].ExecCmd

	// Collect all packages, dedup (parent first), and track per-layer.
	// If a layer already has PackageLayers (from a previous merge), preserve
	// those sub-layers instead of flattening into one.
	pkgSeen := map[string]bool{}
	for _, layer := range layers {
		if len(layer.PackageLayers) > 0 {
			// This layer is already a merged result — preserve its sub-layers
			for _, pl := range layer.PackageLayers {
				var dedupedPkgs []string
				for _, pkg := range pl.Packages {
					if !pkgSeen[pkg] {
						pkgSeen[pkg] = true
						merged.Packages = append(merged.Packages, pkg)
						dedupedPkgs = append(dedupedPkgs, pkg)
					}
				}
				if len(dedupedPkgs) > 0 {
					merged.PackageLayers = append(merged.PackageLayers, PackageLayer{
						Image:    pl.Image,
						Packages: dedupedPkgs,
					})
				}
			}
		} else {
			// Single layer — create a PackageLayer entry
			var layerPkgs []string
			for _, pkg := range layer.Packages {
				if !pkgSeen[pkg] {
					pkgSeen[pkg] = true
					merged.Packages = append(merged.Packages, pkg)
					layerPkgs = append(layerPkgs, pkg)
				}
			}
			if len(layerPkgs) > 0 {
				merged.PackageLayers = append(merged.PackageLayers, PackageLayer{
					Image:    shortImage(layer.BaseImage),
					Packages: layerPkgs,
				})
			}
		}
	}

	// Collect all pip packages, dedup
	pipSeen := map[string]bool{}
	for _, layer := range layers {
		for _, pkg := range layer.PipPackages {
			if !pipSeen[pkg] {
				pipSeen[pkg] = true
				merged.PipPackages = append(merged.PipPackages, pkg)
			}
		}
	}

	// Ports: union
	portSeen := map[string]bool{}
	for _, layer := range layers {
		for _, port := range layer.Ports {
			if !portSeen[port] {
				portSeen[port] = true
				merged.Ports = append(merged.Ports, port)
			}
		}
	}

	// Volumes: union
	volSeen := map[string]bool{}
	for _, layer := range layers {
		for _, vol := range layer.Volumes {
			if !volSeen[vol] {
				volSeen[vol] = true
				merged.Volumes = append(merged.Volumes, vol)
			}
		}
	}

	// EnvVars: child overrides parent for same key
	envMap := map[string]EnvVar{}
	var envOrder []string
	for _, layer := range layers {
		for _, ev := range layer.EnvVars {
			if _, exists := envMap[ev.Key]; !exists {
				envOrder = append(envOrder, ev.Key)
			}
			envMap[ev.Key] = ev
		}
	}
	for _, key := range envOrder {
		merged.EnvVars = append(merged.EnvVars, envMap[key])
	}

	// AptKeys: union by URL
	keySeen := map[string]bool{}
	for _, layer := range layers {
		for _, key := range layer.AptKeys {
			if !keySeen[key.URL] {
				keySeen[key.URL] = true
				merged.AptKeys = append(merged.AptKeys, key)
			}
		}
	}

	// AptRepos: union by Line
	repoSeen := map[string]bool{}
	for _, layer := range layers {
		for _, repo := range layer.AptRepos {
			if !repoSeen[repo.Line] {
				repoSeen[repo.Line] = true
				merged.AptRepos = append(merged.AptRepos, repo)
			}
		}
	}

	// RepoURL: app layer (last) wins
	for i := len(layers) - 1; i >= 0; i-- {
		if layers[i].RepoURL != "" {
			merged.RepoURL = layers[i].RepoURL
			break
		}
	}

	// StartupCmd/EntrypointCmd: app layer (last) wins
	for i := len(layers) - 1; i >= 0; i-- {
		if layers[i].StartupCmd != "" && merged.StartupCmd == "" {
			merged.StartupCmd = layers[i].StartupCmd
		}
		if layers[i].EntrypointCmd != "" && merged.EntrypointCmd == "" {
			merged.EntrypointCmd = layers[i].EntrypointCmd
		}
	}

	// Action fields from all layers, but parent layers only contribute
	// typed/actionable RunCommands (sed, mv, chmod, chown, tar, etc.).
	// Parent "unknown" and "git_clone" RunCommands are Docker build-time
	// operations (shell conditionals, variable assignments, complex scripts)
	// that produce garbage TODO comments — drop them.
	app := layers[len(layers)-1]

	// CopyInstructions: app layer only (parent COPYs deploy Docker-specific
	// s6 overlay scripts we don't need in LXC)
	merged.CopyInstructions = app.CopyInstructions

	// Users, Directories, Downloads, Symlinks: collect from all layers
	userSeen := map[string]bool{}
	for _, layer := range layers {
		for _, u := range layer.Users {
			if !userSeen[u] {
				userSeen[u] = true
				merged.Users = append(merged.Users, u)
			}
		}
	}
	dirSeen := map[string]bool{}
	for _, layer := range layers {
		for _, d := range layer.Directories {
			if !dirSeen[d] {
				dirSeen[d] = true
				merged.Directories = append(merged.Directories, d)
			}
		}
	}
	for _, layer := range layers {
		merged.Downloads = append(merged.Downloads, layer.Downloads...)
	}
	for _, layer := range layers {
		merged.Symlinks = append(merged.Symlinks, layer.Symlinks...)
	}

	// RunCommands: all from app layer, only typed/actionable from parents
	for i, layer := range layers {
		isAppLayer := i == len(layers)-1
		for _, rc := range layer.RunCommands {
			if isAppLayer {
				merged.RunCommands = append(merged.RunCommands, rc)
			} else if rc.Type != "unknown" && rc.Type != "git_clone" {
				// Parent layer: keep actionable commands (sed, mv, chmod, etc.)
				merged.RunCommands = append(merged.RunCommands, rc)
			}
		}
	}

	return merged
}

// IsTerminalBaseImage returns true if the image is a base OS image
// that has no fetchable Dockerfile (e.g. alpine:3.23, ubuntu:noble, scratch).
func IsTerminalBaseImage(image string) bool {
	img := strings.ToLower(image)

	// scratch is always terminal
	if img == "scratch" {
		return true
	}

	// docker.io/library/* is terminal
	if strings.HasPrefix(img, "docker.io/library/") {
		return true
	}

	// If it has a registry prefix (contains .), it's NOT terminal
	// (except docker.io/library which is handled above)
	name := img
	if idx := strings.Index(img, "/"); idx > 0 {
		prefix := img[:idx]
		if strings.Contains(prefix, ".") {
			// Has a registry domain → not terminal (has fetchable Dockerfile)
			return false
		}
	}

	// No registry prefix: check if it resolves to a known OS
	// Strip tag for detection
	if idx := strings.Index(name, ":"); idx > 0 {
		name = name[:idx]
	}

	// Known terminal base image names
	terminalBases := []string{
		"alpine", "ubuntu", "debian", "centos", "fedora", "archlinux",
		"busybox", "clearlinux", "oraclelinux", "rockylinux", "almalinux",
		"amazonlinux", "opensuse", "photon", "void",
		// Language runtimes from Docker Hub are also terminal
		"python", "node", "ruby", "golang", "rust", "php", "openjdk",
		"eclipse-temurin", "gradle", "maven", "dotnet",
		"perl", "elixir", "erlang", "haskell", "julia", "swift",
		// Database/service images from Docker Hub
		"postgres", "mysql", "mariadb", "mongo", "redis",
		"nginx", "httpd", "haproxy", "memcached", "rabbitmq",
		"wordpress", "ghost", "buildpack-deps",
	}

	for _, base := range terminalBases {
		if name == base {
			return true
		}
	}

	return false
}

// shortImage returns a shortened image reference for display.
func shortImage(image string) string {
	// Strip registry prefix for display
	if idx := strings.LastIndex(image, "/"); idx > 0 {
		return image[idx+1:]
	}
	return image
}

// repoURLFromTemplate extracts a GitHub repo URL from a raw.githubusercontent.com URL template.
// "https://raw.githubusercontent.com/owner/repo/{branch}/Dockerfile" → "https://github.com/owner/repo"
func repoURLFromTemplate(urlTmpl string) string {
	const rawPrefix = "https://raw.githubusercontent.com/"
	if !strings.HasPrefix(urlTmpl, rawPrefix) {
		return ""
	}
	path := strings.TrimPrefix(urlTmpl, rawPrefix)
	parts := strings.SplitN(path, "/", 3) // owner/repo/rest
	if len(parts) < 2 {
		return ""
	}
	return "https://github.com/" + parts[0] + "/" + parts[1]
}

// ExtractGitHubRepoURL extracts a GitHub repo URL from any GitHub URL.
// Handles github.com pages, raw.githubusercontent.com, etc.
// Returns empty string if not a GitHub URL.
func ExtractGitHubRepoURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	switch u.Host {
	case "github.com":
		parts := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 3)
		if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			return "https://github.com/" + parts[0] + "/" + parts[1]
		}
	case "raw.githubusercontent.com":
		return repoURLFromTemplate(rawURL)
	}
	return ""
}

// friendlyOS returns a friendly OS name for display in terminal events.
// It prefers the OS profile display name over the raw image name,
// since images like "scratch" are used in rootfs-builder patterns but the
// actual OS is detected from earlier stages.
func friendlyOS(info *DockerfileInfo) string {
	profile := ProfileFor(info.BaseOS)
	if info.BaseOS != "" && info.BaseOS != "unknown" {
		return profile.DisplayName
	}
	img := shortImage(info.BaseImage)
	if img == "scratch" || img == "" {
		return "Base OS"
	}
	if len(img) > 0 {
		return strings.ToUpper(img[:1]) + img[1:]
	}
	return "Unknown"
}
