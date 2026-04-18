package signing

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type SignableKind int

const (
	KindFramework SignableKind = iota
	KindHelperApp
	KindXPCService
	KindPlugin
	KindLoginItem
	KindMainBundle
)

func (k SignableKind) String() string {
	switch k {
	case KindFramework:
		return "framework"
	case KindHelperApp:
		return "helper-app"
	case KindXPCService:
		return "xpc-service"
	case KindPlugin:
		return "plugin"
	case KindLoginItem:
		return "login-item"
	case KindMainBundle:
		return "main-bundle"
	default:
		return "unknown"
	}
}

type SignableItem struct {
	Path  string
	Kind  SignableKind
	Depth int // 0 = outermost bundle; deeper = sign first
}

// Discover walks the .app bundle and returns every path that needs to be
// re-signed, sorted deepest-first (post-order) so signing can proceed from
// leaves to root. The main bundle itself is always the last element.
func Discover(appPath string) ([]SignableItem, error) {
	items := discoverAt(appPath, 0)
	items = append(items, SignableItem{Path: appPath, Kind: KindMainBundle, Depth: 0})
	sort.Slice(items, func(i, j int) bool {
		if items[i].Depth != items[j].Depth {
			return items[i].Depth > items[j].Depth
		}
		return items[i].Path < items[j].Path
	})
	return items, nil
}

func discoverAt(bundlePath string, depth int) []SignableItem {
	var items []SignableItem
	contents := filepath.Join(bundlePath, "Contents")

	items = append(items, scanDir(filepath.Join(contents, "Frameworks"), ".framework", KindFramework, depth+1)...)
	items = append(items, scanDir(filepath.Join(contents, "XPCServices"), ".xpc", KindXPCService, depth+1)...)

	// Plugins: .bundle, .appex, .plugin, or .app containers
	for _, suffix := range []string{".bundle", ".appex", ".plugin", ".app"} {
		items = append(items, scanDir(filepath.Join(contents, "PlugIns"), suffix, KindPlugin, depth+1)...)
	}

	// Helpers live in a few known locations; recurse into each because
	// helpers commonly ship their own nested Frameworks.
	helperDirs := []string{
		filepath.Join(contents, "Frameworks"),
		filepath.Join(contents, "Helpers"),
		filepath.Join(contents, "Library", "Helpers"),
	}
	for _, dir := range helperDirs {
		for _, item := range scanDir(dir, ".app", KindHelperApp, depth+1) {
			items = append(items, item)
			items = append(items, discoverAt(item.Path, depth+1)...)
		}
	}

	// Login items
	for _, item := range scanDir(filepath.Join(contents, "Library", "LoginItems"), ".app", KindLoginItem, depth+1) {
		items = append(items, item)
		items = append(items, discoverAt(item.Path, depth+1)...)
	}

	return items
}

func scanDir(dir, suffix string, kind SignableKind, depth int) []SignableItem {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var items []SignableItem
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), suffix) {
			continue
		}
		items = append(items, SignableItem{
			Path:  filepath.Join(dir, e.Name()),
			Kind:  kind,
			Depth: depth,
		})
	}
	return items
}
