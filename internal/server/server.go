package server

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
	"github.com/battlewithbytes/pve-appstore/internal/config"
	"github.com/battlewithbytes/pve-appstore/internal/engine"
)

// Server is the main HTTP server for the PVE App Store.
type Server struct {
	cfg     *config.Config
	catalog *catalog.Catalog
	engine  *engine.Engine
	http    *http.Server
	spa     fs.FS // embedded or disk-based SPA assets
}

// New creates a new Server.
func New(cfg *config.Config, cat *catalog.Catalog, eng *engine.Engine, spaFS fs.FS) *Server {
	s := &Server{
		cfg:     cfg,
		catalog: cat,
		engine:  eng,
		spa:     spaFS,
	}

	mux := http.NewServeMux()

	// API routes — catalog
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/apps", s.handleListApps)
	mux.HandleFunc("GET /api/apps/{id}", s.handleGetApp)
	mux.HandleFunc("GET /api/apps/{id}/readme", s.handleGetAppReadme)
	mux.HandleFunc("GET /api/apps/{id}/icon", s.handleGetAppIcon)
	mux.HandleFunc("GET /api/categories", s.handleCategories)
	mux.HandleFunc("POST /api/catalog/refresh", s.withAuth(s.handleCatalogRefresh))

	// API routes — config
	mux.HandleFunc("GET /api/config/defaults", s.handleConfigDefaults)

	// API routes — install engine
	mux.HandleFunc("POST /api/apps/{id}/install", s.withAuth(s.handleInstallApp))
	mux.HandleFunc("GET /api/jobs", s.handleListJobs)
	mux.HandleFunc("GET /api/jobs/{id}", s.handleGetJob)
	mux.HandleFunc("GET /api/jobs/{id}/logs", s.handleGetJobLogs)
	mux.HandleFunc("GET /api/installs", s.handleListInstalls)
	mux.HandleFunc("POST /api/installs/{id}/uninstall", s.withAuth(s.handleUninstall))

	// Auth
	if cfg.Auth.Mode == config.AuthModePassword {
		mux.HandleFunc("POST /api/auth/login", s.handleLogin)
		mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
		mux.HandleFunc("GET /api/auth/check", s.handleAuthCheck)
	}

	// SPA fallback — serve index.html for all non-API routes
	if spaFS != nil {
		mux.Handle("/", s.spaHandler())
	}

	var handler http.Handler = mux
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

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		fmt.Printf("[%s] %s %s %s\n", time.Now().Format("15:04:05"), r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
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
