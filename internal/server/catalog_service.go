package server

import (
	"time"

	"github.com/battlewithbytes/pve-appstore/internal/catalog"
)

// CatalogService isolates catalog access from HTTP handlers.
type CatalogService interface {
	AppCount() int
	StackCount() int
	RepoURL() string
	Branch() string
	ListApps() []*catalog.AppManifest
	SearchApps(query string) []*catalog.AppManifest
	GetApp(id string) (*catalog.AppManifest, bool)
	Categories() []string
	Refresh() error
	LastRefresh() time.Time
	MergeDevApp(app *catalog.AppManifest)
	RemoveDevApp(id string)
	GetShadowed(id string) (*catalog.AppManifest, bool)
	ListStacks() []*catalog.StackManifest
	GetStack(id string) (*catalog.StackManifest, bool)
	MergeDevStack(s *catalog.StackManifest)
	RemoveDevStack(id string)
}

type defaultCatalogService struct {
	cat *catalog.Catalog
}

func NewCatalogService(cat *catalog.Catalog) CatalogService {
	if cat == nil {
		return nil
	}
	return &defaultCatalogService{cat: cat}
}

func (s *defaultCatalogService) AppCount() int    { return s.cat.AppCount() }
func (s *defaultCatalogService) StackCount() int  { return s.cat.StackCount() }
func (s *defaultCatalogService) RepoURL() string  { return s.cat.RepoURL() }
func (s *defaultCatalogService) Branch() string   { return s.cat.Branch() }
func (s *defaultCatalogService) ListApps() []*catalog.AppManifest {
	return s.cat.List()
}
func (s *defaultCatalogService) SearchApps(query string) []*catalog.AppManifest {
	return s.cat.Search(query)
}
func (s *defaultCatalogService) GetApp(id string) (*catalog.AppManifest, bool) {
	return s.cat.Get(id)
}
func (s *defaultCatalogService) Categories() []string  { return s.cat.Categories() }
func (s *defaultCatalogService) Refresh() error        { return s.cat.Refresh() }
func (s *defaultCatalogService) LastRefresh() time.Time { return s.cat.LastRefresh() }
func (s *defaultCatalogService) MergeDevApp(app *catalog.AppManifest) {
	s.cat.MergeDevApp(app)
}
func (s *defaultCatalogService) RemoveDevApp(id string) {
	s.cat.RemoveDevApp(id)
}
func (s *defaultCatalogService) GetShadowed(id string) (*catalog.AppManifest, bool) {
	return s.cat.GetShadowed(id)
}
func (s *defaultCatalogService) ListStacks() []*catalog.StackManifest {
	return s.cat.ListStacks()
}
func (s *defaultCatalogService) GetStack(id string) (*catalog.StackManifest, bool) {
	return s.cat.GetStack(id)
}
func (s *defaultCatalogService) MergeDevStack(sm *catalog.StackManifest) {
	s.cat.MergeDevStack(sm)
}
func (s *defaultCatalogService) RemoveDevStack(id string) {
	s.cat.RemoveDevStack(id)
}
