package server

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
	"github.com/battlewithbytes/pve-appstore/internal/config"
	"github.com/battlewithbytes/pve-appstore/internal/devmode"
	"github.com/battlewithbytes/pve-appstore/internal/engine"
)

// Server is the main HTTP server for the PVE App Store.
type Server struct {
	cfg              *config.Config
	configPath       string
	catalog          *catalog.Catalog
	catalogSvc       CatalogService
	engine           *engine.Engine
	engineInstallSvc EngineInstallService
	engineStackSvc   EngineStackService
	engineConfigSvc  EngineConfigService
	devStore    *devmode.DevStore
	devSvc      DevService
	githubStore GitHubStore
	http        *http.Server
	spa               fs.FS // embedded or disk-based SPA assets
	storageMetas      []engine.StorageInfo
	allowedPaths      []string // browsable filesystem roots from configured storages
}

// Option configures the server.
type Option func(*Server)

// WithConfigPath sets the config file path for settings persistence.
func WithConfigPath(path string) Option {
	return func(s *Server) { s.configPath = path }
}

// isPathAllowed checks whether a filesystem path is under a configured storage root.
func (s *Server) isPathAllowed(requested string) bool {
	if len(s.allowedPaths) == 0 {
		return false // deny all if no storage paths resolved
	}
	cleaned := filepath.Clean(requested)
	for _, allowed := range s.allowedPaths {
		if cleaned == allowed || strings.HasPrefix(cleaned, allowed+"/") {
			return true
		}
	}
	return false
}

// New creates a new Server.
func New(cfg *config.Config, cat *catalog.Catalog, eng *engine.Engine, spaFS fs.FS, opts ...Option) *Server {
	s := &Server{
		cfg:               cfg,
		configPath:        config.DefaultConfigPath,
		catalog:           cat,
		catalogSvc:        NewCatalogService(cat),
		engine:            eng,
		spa:               spaFS,
	}
	engineSvc := NewEngineService(eng)
	s.engineInstallSvc = engineSvc
	s.engineStackSvc = engineSvc
	s.engineConfigSvc = engineSvc
	if eng != nil {
		s.githubStore = eng
	}
	for _, opt := range opts {
		opt(s)
	}

	// Initialize dev store and restore deployed dev apps into catalog
	s.devStore = devmode.NewDevStore(filepath.Join(config.DefaultDataDir, "dev-apps"))
	s.devSvc = NewDevService(s.devStore)
	if cfg.Developer.Enabled {
		if apps, err := s.devSvc.List(); err == nil {
			for _, meta := range apps {
				if meta.Status != "deployed" {
					continue
				}
				m, err := s.devSvc.ParseManifest(meta.ID)
				if err != nil {
					log.Printf("[dev] warning: could not restore deployed app %q: %v", meta.ID, err)
					continue
				}
				appDir := s.devSvc.AppDir(meta.ID)
				m.DirPath = appDir
				if _, err := os.Stat(filepath.Join(appDir, "icon.png")); err == nil {
					m.IconPath = filepath.Join(appDir, "icon.png")
				}
				if _, err := os.Stat(filepath.Join(appDir, "README.md")); err == nil {
					m.ReadmePath = filepath.Join(appDir, "README.md")
				}
				if s.catalogSvc != nil {
					s.catalogSvc.MergeDevApp(m)
				}
				log.Printf("[dev] restored deployed app %q into catalog", meta.ID)
			}
		}
	}

	// Resolve configured storages to filesystem paths via Proxmox API
	if eng != nil {
		ctx := context.Background()
		for _, storageID := range cfg.Storages {
			si, err := eng.GetStorageInfo(ctx, storageID)
			if err != nil {
				log.Printf("[server] warning: could not resolve storage %q: %v", storageID, err)
				continue
			}
			s.storageMetas = append(s.storageMetas, *si)
			if si.Browsable && si.Path != "" {
				s.allowedPaths = append(s.allowedPaths, filepath.Clean(si.Path))
			}
		}
		if len(s.storageMetas) > 0 {
			log.Printf("[server] resolved %d storage(s), %d browsable path(s)", len(s.storageMetas), len(s.allowedPaths))
		}
	}

	mux := http.NewServeMux()

	// API routes — catalog
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/apps", s.handleListApps)
	mux.HandleFunc("GET /api/apps/{id}", s.handleGetApp)
	mux.HandleFunc("GET /api/apps/{id}/readme", s.handleGetAppReadme)
	mux.HandleFunc("GET /api/apps/{id}/icon", s.handleGetAppIcon)
	mux.HandleFunc("GET /api/apps/{id}/status", s.handleAppStatus)
	mux.HandleFunc("GET /api/categories", s.handleCategories)
	mux.HandleFunc("POST /api/catalog/refresh", s.withAuth(s.handleCatalogRefresh))

	// API routes — config
	mux.HandleFunc("GET /api/config/defaults", s.handleConfigDefaults)
	mux.HandleFunc("GET /api/config/export", s.withAuth(s.handleConfigExport))
	mux.HandleFunc("GET /api/config/export/download", s.withAuth(s.handleConfigExportDownload))
	mux.HandleFunc("POST /api/config/apply", s.withAuth(s.handleConfigApply))
	mux.HandleFunc("POST /api/config/apply/preview", s.withAuth(s.handleConfigApplyPreview))

	// API routes — settings
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", s.withAuth(s.handleUpdateSettings))

	// API routes — developer mode
	mux.HandleFunc("GET /api/dev/apps", s.withDevMode(s.withAuth(s.handleDevListApps)))
	mux.HandleFunc("POST /api/dev/apps", s.withDevMode(s.withAuth(s.handleDevCreateApp)))
	mux.HandleFunc("POST /api/dev/fork", s.withDevMode(s.withAuth(s.handleDevForkApp)))
	mux.HandleFunc("GET /api/dev/apps/{id}", s.withDevMode(s.withAuth(s.handleDevGetApp)))
	mux.HandleFunc("PUT /api/dev/apps/{id}/manifest", s.withDevMode(s.withAuth(s.handleDevSaveManifest)))
	mux.HandleFunc("PUT /api/dev/apps/{id}/script", s.withDevMode(s.withAuth(s.handleDevSaveScript)))
	mux.HandleFunc("GET /api/dev/apps/{id}/file", s.withDevMode(s.withAuth(s.handleDevGetFile)))
	mux.HandleFunc("PUT /api/dev/apps/{id}/file", s.withDevMode(s.withAuth(s.handleDevSaveFile)))
	mux.HandleFunc("DELETE /api/dev/apps/{id}", s.withDevMode(s.withAuth(s.handleDevDeleteApp)))
	mux.HandleFunc("POST /api/dev/apps/{id}/validate", s.withDevMode(s.withAuth(s.handleDevValidate)))
	mux.HandleFunc("POST /api/dev/apps/{id}/deploy", s.withDevMode(s.withAuth(s.handleDevDeploy)))
	mux.HandleFunc("POST /api/dev/apps/{id}/undeploy", s.withDevMode(s.withAuth(s.handleDevUndeploy)))
	mux.HandleFunc("GET /api/dev/apps/{id}/icon", s.withDevMode(s.handleDevGetIcon))
	mux.HandleFunc("PUT /api/dev/apps/{id}/icon", s.withDevMode(s.withAuth(s.handleDevSetIcon)))
	mux.HandleFunc("POST /api/dev/apps/{id}/export", s.withDevMode(s.withAuth(s.handleDevExport)))
	mux.HandleFunc("POST /api/dev/import/unraid", s.withDevMode(s.withAuth(s.handleDevImportUnraid)))
	mux.HandleFunc("POST /api/dev/import/dockerfile", s.withDevMode(s.withAuth(s.handleDevImportDockerfile)))
	mux.HandleFunc("POST /api/dev/import/dockerfile/stream", s.withDevMode(s.withAuth(s.handleDevImportDockerfileStream)))
	mux.HandleFunc("GET /api/dev/templates", s.withDevMode(s.handleDevListTemplates))

	// API routes — developer GitHub integration
	mux.HandleFunc("GET /api/dev/github/status", s.withDevMode(s.withAuth(s.handleDevGitHubStatus)))
	mux.HandleFunc("POST /api/dev/github/connect", s.withDevMode(s.withAuth(s.handleDevGitHubConnect)))
	mux.HandleFunc("POST /api/dev/github/disconnect", s.withDevMode(s.withAuth(s.handleDevGitHubDisconnect)))
	mux.HandleFunc("GET /api/dev/apps/{id}/publish-status", s.withDevMode(s.withAuth(s.handleDevPublishStatus)))
	mux.HandleFunc("POST /api/dev/apps/{id}/publish", s.withDevMode(s.withAuth(s.handleDevPublish)))

	// API routes — filesystem browser
	mux.HandleFunc("GET /api/browse/paths", s.withAuth(s.handleBrowsePaths))
	mux.HandleFunc("GET /api/browse/storages", s.withAuth(s.handleBrowseStorages))
	mux.HandleFunc("GET /api/browse/mounts", s.withAuth(s.handleBrowseMounts))
	mux.HandleFunc("POST /api/browse/mkdir", s.withAuth(s.handleBrowseMkdir))

	// API routes — install engine
	mux.HandleFunc("POST /api/apps/{id}/install", s.withAuth(s.handleInstallApp))
	mux.HandleFunc("GET /api/jobs", s.handleListJobs)
	mux.HandleFunc("DELETE /api/jobs", s.withAuth(s.handleClearJobs))
	mux.HandleFunc("GET /api/jobs/{id}", s.handleGetJob)
	mux.HandleFunc("GET /api/jobs/{id}/logs", s.handleGetJobLogs)
	mux.HandleFunc("POST /api/jobs/{id}/cancel", s.withAuth(s.handleCancelJob))
	mux.HandleFunc("GET /api/installs", s.handleListInstalls)
	mux.HandleFunc("GET /api/installs/{id}", s.handleGetInstall)
	mux.HandleFunc("GET /api/installs/{id}/terminal", s.withAuth(s.handleTerminal))
	mux.HandleFunc("GET /api/installs/{id}/logs", s.withAuth(s.handleJournalLogs))
	mux.HandleFunc("POST /api/installs/{id}/start", s.withAuth(s.handleStartContainer))
	mux.HandleFunc("POST /api/installs/{id}/stop", s.withAuth(s.handleStopContainer))
	mux.HandleFunc("POST /api/installs/{id}/restart", s.withAuth(s.handleRestartContainer))
	mux.HandleFunc("POST /api/installs/{id}/uninstall", s.withAuth(s.handleUninstall))
	mux.HandleFunc("DELETE /api/installs/{id}", s.withAuth(s.handlePurgeInstall))
	mux.HandleFunc("POST /api/installs/{id}/reinstall", s.withAuth(s.handleReinstall))
	mux.HandleFunc("POST /api/installs/{id}/update", s.withAuth(s.handleUpdate))
	mux.HandleFunc("POST /api/installs/{id}/edit", s.withAuth(s.handleEditInstall))
	mux.HandleFunc("POST /api/installs/{id}/reconfigure", s.withAuth(s.handleReconfigure))

	// API routes — stacks
	mux.HandleFunc("POST /api/stacks", s.withAuth(s.handleCreateStack))
	mux.HandleFunc("GET /api/stacks", s.handleListStacks)
	mux.HandleFunc("GET /api/stacks/{id}", s.handleGetStack)
	mux.HandleFunc("POST /api/stacks/{id}/start", s.withAuth(s.handleStartStack))
	mux.HandleFunc("POST /api/stacks/{id}/stop", s.withAuth(s.handleStopStack))
	mux.HandleFunc("POST /api/stacks/{id}/restart", s.withAuth(s.handleRestartStack))
	mux.HandleFunc("POST /api/stacks/{id}/uninstall", s.withAuth(s.handleUninstallStack))
	mux.HandleFunc("POST /api/stacks/{id}/edit", s.withAuth(s.handleEditStack))
	mux.HandleFunc("POST /api/stacks/validate", s.withAuth(s.handleValidateStack))
	mux.HandleFunc("GET /api/stacks/{id}/terminal", s.withAuth(s.handleStackTerminal))
	mux.HandleFunc("GET /api/stacks/{id}/logs", s.withAuth(s.handleStackJournalLogs))

	// Auth
	if cfg.Auth.Mode == config.AuthModePassword {
		mux.HandleFunc("POST /api/auth/login", s.handleLogin)
		mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
		mux.HandleFunc("GET /api/auth/check", s.handleAuthCheck)
		mux.HandleFunc("POST /api/auth/terminal-token", s.withAuth(s.handleTerminalToken))
	}

	// SPA fallback — serve index.html for all non-API routes
	if spaFS != nil {
		mux.Handle("/", s.spaHandler())
	}

	var handler http.Handler = mux
	handler = maxBodyMiddleware(handler, 1<<20) // 1 MB limit for API requests
	handler = corsMiddleware(handler)
	handler = logMiddleware(handler)

	s.http = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Service.BindAddress, cfg.Service.Port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return s.http.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

// Addr returns the configured listen address.
func (s *Server) Addr() string {
	return s.http.Addr
}

func maxBodyMiddleware(next http.Handler, maxBytes int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only limit request body for API POST/PUT/DELETE, not WebSocket upgrades or static assets
		if r.Body != nil && strings.HasPrefix(r.URL.Path, "/api/") && r.Method != "GET" &&
			!strings.Contains(r.Header.Get("Upgrade"), "websocket") {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		fmt.Printf("[%s] %s %s %s\n", time.Now().Format("15:04:05"), r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			// Reflect the request origin only if it matches this server's host,
			// or if no Origin header is present (same-origin requests).
			host := r.Host
			if strings.HasPrefix(origin, "http://"+host) || strings.HasPrefix(origin, "https://"+host) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			// For dev: also allow localhost origins connecting to any host
			if strings.Contains(origin, "://localhost:") || strings.Contains(origin, "://127.0.0.1:") {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Upgrade, Connection")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// allowedOriginPatterns returns WebSocket origin patterns matching the server's host.
func (s *Server) allowedOriginPatterns(r *http.Request) []string {
	patterns := []string{"localhost:*", "127.0.0.1:*"}
	if host := r.Host; host != "" {
		// Strip port if present for the pattern
		h := host
		if idx := strings.LastIndex(h, ":"); idx > 0 {
			h = h[:idx]
		}
		patterns = append(patterns, h+":*", host)
	}
	return patterns
}

// withDevMode gates a handler on developer mode being enabled.
func (s *Server) withDevMode(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.Developer.Enabled {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "developer mode is not enabled"})
			return
		}
		next(w, r)
	}
}

// spaHandler serves static files from the SPA filesystem, falling back to index.html.
func (s *Server) spaHandler() http.Handler {
	fileServer := http.FileServerFS(s.spa)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the file directly
		path := r.URL.Path
		if path == "/" {
			path = "index.html"
		}

		// fs.FS.Open expects paths without leading slash
		cleanPath := strings.TrimPrefix(path, "/")

		// Check if file exists in SPA FS
		if f, err := s.spa.Open(cleanPath); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// Fallback to index.html for SPA client-side routing
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
