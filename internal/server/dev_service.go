package server

import (
	"github.com/battlewithbytes/pve-appstore/internal/catalog"
	"github.com/battlewithbytes/pve-appstore/internal/devmode"
)

// DevService isolates developer-mode app store operations.
type DevService interface {
	List() ([]devmode.DevAppMeta, error)
	Create(id, template string) error
	Get(id string) (*devmode.DevApp, error)
	Fork(newID, sourceDir string) error
	SaveManifest(id string, manifest []byte) error
	SaveScript(id string, script []byte) error
	IsDeployed(id string) bool
	ParseManifest(id string) (*catalog.AppManifest, error)
	AppDir(id string) string
	SaveFile(id, relPath string, data []byte) error
	ReadFile(id, relPath string) ([]byte, error)
	Delete(id string) error
	SetStatus(id, status string) error
	SetGitHubMeta(id string, meta map[string]string) error
	EnsureIcon(id string)
}

type defaultDevService struct {
	store *devmode.DevStore
}

func NewDevService(store *devmode.DevStore) DevService {
	if store == nil {
		return nil
	}
	return &defaultDevService{store: store}
}

func (s *defaultDevService) List() ([]devmode.DevAppMeta, error) { return s.store.List() }
func (s *defaultDevService) Create(id, template string) error    { return s.store.Create(id, template) }
func (s *defaultDevService) Get(id string) (*devmode.DevApp, error) {
	return s.store.Get(id)
}
func (s *defaultDevService) Fork(newID, sourceDir string) error {
	return s.store.Fork(newID, sourceDir)
}
func (s *defaultDevService) SaveManifest(id string, manifest []byte) error {
	return s.store.SaveManifest(id, manifest)
}
func (s *defaultDevService) SaveScript(id string, script []byte) error {
	return s.store.SaveScript(id, script)
}
func (s *defaultDevService) IsDeployed(id string) bool { return s.store.IsDeployed(id) }
func (s *defaultDevService) ParseManifest(id string) (*catalog.AppManifest, error) {
	return s.store.ParseManifest(id)
}
func (s *defaultDevService) AppDir(id string) string { return s.store.AppDir(id) }
func (s *defaultDevService) SaveFile(id, relPath string, data []byte) error {
	return s.store.SaveFile(id, relPath, data)
}
func (s *defaultDevService) ReadFile(id, relPath string) ([]byte, error) {
	return s.store.ReadFile(id, relPath)
}
func (s *defaultDevService) Delete(id string) error            { return s.store.Delete(id) }
func (s *defaultDevService) SetStatus(id, status string) error { return s.store.SetStatus(id, status) }
func (s *defaultDevService) SetGitHubMeta(id string, meta map[string]string) error {
	return s.store.SetGitHubMeta(id, meta)
}
func (s *defaultDevService) EnsureIcon(id string) { s.store.EnsureIcon(id) }
