package appinfo

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultScanDirs returns the standard locations appclone scans for apps.
func DefaultScanDirs() []string {
	dirs := []string{"/Applications", "/Applications/Utilities"}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, "Applications"))
	}
	return dirs
}

// ListInstalled scans dirs (or DefaultScanDirs when dirs is nil) for .app
// bundles and returns one InspectionReport per readable bundle. Unreadable
// directories and unparseable bundles are skipped silently — this is a
// best-effort listing, not a strict check.
func ListInstalled(dirs []string) ([]*InspectionReport, error) {
	if dirs == nil {
		dirs = DefaultScanDirs()
	}
	seen := map[string]bool{}
	var out []*InspectionReport
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".app") {
				continue
			}
			app := filepath.Join(d, e.Name())
			if seen[app] {
				continue
			}
			seen[app] = true
			report, err := Inspect(app)
			if err != nil {
				continue
			}
			out = append(out, report)
		}
	}
	return out, nil
}
