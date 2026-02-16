package catalog

import "testing"

func TestMergeDevApp_ShadowsOriginal(t *testing.T) {
	cat := &Catalog{
		apps:     make(map[string]*AppManifest),
		shadowed: make(map[string]*AppManifest),
	}

	original := &AppManifest{ID: "plex", Version: "1.40.0", Source: "official"}
	cat.apps["plex"] = original

	dev := &AppManifest{ID: "plex", Version: "1.41.0"}
	cat.MergeDevApp(dev)

	// Active app should be the dev version
	active, ok := cat.Get("plex")
	if !ok || active.Version != "1.41.0" {
		t.Errorf("expected active version 1.41.0, got %s", active.Version)
	}
	if active.Source != "developer" {
		t.Errorf("expected source=developer, got %s", active.Source)
	}

	// Shadowed should be the original
	shadowed, ok := cat.GetShadowed("plex")
	if !ok {
		t.Fatal("expected shadowed version to exist")
	}
	if shadowed.Version != "1.40.0" {
		t.Errorf("expected shadowed version 1.40.0, got %s", shadowed.Version)
	}
}

func TestRemoveDevApp_RestoresShadowed(t *testing.T) {
	cat := &Catalog{
		apps:     make(map[string]*AppManifest),
		shadowed: make(map[string]*AppManifest),
	}

	original := &AppManifest{ID: "plex", Version: "1.40.0", Source: "official"}
	cat.apps["plex"] = original

	dev := &AppManifest{ID: "plex", Version: "1.41.0"}
	cat.MergeDevApp(dev)
	cat.RemoveDevApp("plex")

	// Should restore the original
	active, ok := cat.Get("plex")
	if !ok {
		t.Fatal("expected app to still exist after undeploy")
	}
	if active.Version != "1.40.0" {
		t.Errorf("expected restored version 1.40.0, got %s", active.Version)
	}

	// No shadow anymore
	_, ok = cat.GetShadowed("plex")
	if ok {
		t.Error("expected no shadowed version after undeploy")
	}
}

func TestGetShadowed_NoShadow(t *testing.T) {
	cat := &Catalog{
		apps:     make(map[string]*AppManifest),
		shadowed: make(map[string]*AppManifest),
	}

	_, ok := cat.GetShadowed("nonexistent")
	if ok {
		t.Error("expected false for non-existent shadowed app")
	}
}

func TestGetShadowed_NewDevApp_NoShadow(t *testing.T) {
	cat := &Catalog{
		apps:     make(map[string]*AppManifest),
		shadowed: make(map[string]*AppManifest),
	}

	// Brand new dev app, no original in catalog
	dev := &AppManifest{ID: "my-new-app", Version: "1.0.0"}
	cat.MergeDevApp(dev)

	_, ok := cat.GetShadowed("my-new-app")
	if ok {
		t.Error("expected no shadowed version for brand new dev app")
	}
}
