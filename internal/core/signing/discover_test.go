package signing

import (
	"os"
	"path/filepath"
	"testing"
)

func mkBundle(t *testing.T, root, name string) string {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(p, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "Contents", "Info.plist"), []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func mkDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestDiscover_EmptyBundleOnlyReturnsMain(t *testing.T) {
	app := mkBundle(t, t.TempDir(), "Empty.app")
	items, err := Discover(app)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d: %+v", len(items), items)
	}
	if items[0].Kind != KindMainBundle {
		t.Errorf("expected main bundle, got %v", items[0].Kind)
	}
	if items[0].Depth != 0 {
		t.Errorf("main bundle depth = %d, want 0", items[0].Depth)
	}
}

func TestDiscover_FrameworksSortBeforeMain(t *testing.T) {
	app := mkBundle(t, t.TempDir(), "App.app")
	mkDir(t, filepath.Join(app, "Contents", "Frameworks", "A.framework"))
	mkDir(t, filepath.Join(app, "Contents", "Frameworks", "B.framework"))

	items, err := Discover(app)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	// Frameworks first (depth 1), main bundle last (depth 0).
	if items[0].Kind != KindFramework || !hasSuffix(items[0].Path, "A.framework") {
		t.Errorf("items[0] = %+v, expected A.framework", items[0])
	}
	if items[1].Kind != KindFramework || !hasSuffix(items[1].Path, "B.framework") {
		t.Errorf("items[1] = %+v, expected B.framework", items[1])
	}
	if items[2].Kind != KindMainBundle {
		t.Errorf("items[2] = %+v, expected main bundle", items[2])
	}
}

func TestDiscover_AllKinds(t *testing.T) {
	app := mkBundle(t, t.TempDir(), "Many.app")
	mkDir(t, filepath.Join(app, "Contents", "Frameworks", "Core.framework"))
	mkDir(t, filepath.Join(app, "Contents", "XPCServices", "Updater.xpc"))
	mkDir(t, filepath.Join(app, "Contents", "PlugIns", "Plug.bundle"))
	mkDir(t, filepath.Join(app, "Contents", "Helpers"))
	mkBundle(t, filepath.Join(app, "Contents", "Helpers"), "Helper.app")
	mkDir(t, filepath.Join(app, "Contents", "Library", "LoginItems"))
	mkBundle(t, filepath.Join(app, "Contents", "Library", "LoginItems"), "LoginItem.app")

	items, err := Discover(app)
	if err != nil {
		t.Fatal(err)
	}

	kinds := map[SignableKind]int{}
	for _, it := range items {
		kinds[it.Kind]++
	}
	expected := map[SignableKind]int{
		KindFramework:  1,
		KindXPCService: 1,
		KindPlugin:     1,
		KindHelperApp:  1,
		KindLoginItem:  1,
		KindMainBundle: 1,
	}
	for k, want := range expected {
		if kinds[k] != want {
			t.Errorf("kind %v count = %d, want %d", k, kinds[k], want)
		}
	}
	if items[len(items)-1].Kind != KindMainBundle {
		t.Errorf("last item should be main bundle, got %v", items[len(items)-1].Kind)
	}
}

func TestDiscover_NestedHelperFrameworkSignedFirst(t *testing.T) {
	app := mkBundle(t, t.TempDir(), "Outer.app")
	mkDir(t, filepath.Join(app, "Contents", "Helpers"))
	helper := mkBundle(t, filepath.Join(app, "Contents", "Helpers"), "Inner.app")
	mkDir(t, filepath.Join(helper, "Contents", "Frameworks", "Deep.framework"))

	items, err := Discover(app)
	if err != nil {
		t.Fatal(err)
	}

	// Expected depth ordering: Deep.framework (2) → Inner.app (1) → Outer.app (0)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d: %+v", len(items), items)
	}
	if items[0].Depth != 2 || items[0].Kind != KindFramework {
		t.Errorf("items[0] = %+v, expected depth=2 framework", items[0])
	}
	if items[1].Depth != 1 || items[1].Kind != KindHelperApp {
		t.Errorf("items[1] = %+v, expected depth=1 helper app", items[1])
	}
	if items[2].Depth != 0 || items[2].Kind != KindMainBundle {
		t.Errorf("items[2] = %+v, expected depth=0 main bundle", items[2])
	}
}

func TestDiscover_FrameworkHelperAppSignedFirst(t *testing.T) {
	app := mkBundle(t, t.TempDir(), "Outer.app")
	helper := mkBundle(t, filepath.Join(app, "Contents", "Frameworks"), "Helper.app")
	mkDir(t, filepath.Join(helper, "Contents", "Frameworks", "Deep.framework"))

	items, err := Discover(app)
	if err != nil {
		t.Fatal(err)
	}

	// Electron-style helpers live in Contents/Frameworks/*.app.
	// They must be discovered and recursed so bundle IDs get rewritten
	// and the helper gets re-signed when the parent bundle ID changes.
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d: %+v", len(items), items)
	}
	if items[0].Depth != 2 || items[0].Kind != KindFramework {
		t.Errorf("items[0] = %+v, expected depth=2 framework", items[0])
	}
	if items[1].Depth != 1 || items[1].Kind != KindHelperApp {
		t.Errorf("items[1] = %+v, expected depth=1 helper app", items[1])
	}
	if items[2].Depth != 0 || items[2].Kind != KindMainBundle {
		t.Errorf("items[2] = %+v, expected depth=0 main bundle", items[2])
	}
}

func hasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
