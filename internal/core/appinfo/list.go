package appinfo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultScanDirs returns the standard locations doppel scans for apps.
func DefaultScanDirs() []string {
	dirs := []string{"/Applications", "/Applications/Utilities"}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, "Applications"))
	}
	return dirs
}

type ScanSkip struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type ScanResult struct {
	Reports []*InspectionReport `json:"reports"`
	Skipped []ScanSkip          `json:"skipped,omitempty"`
}

// ScanInstalled scans dirs (or DefaultScanDirs when dirs is nil) for .app
// bundles and returns both readable bundles and skipped bundles with reasons.
// Directory read failures are reported as skipped entries so interactive UIs
// can explain missing apps instead of silently hiding them.
func ScanInstalled(dirs []string) (*ScanResult, error) {
	if dirs == nil {
		dirs = DefaultScanDirs()
	}
	seen := map[string]bool{}
	out := &ScanResult{}
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			out.Skipped = append(out.Skipped, ScanSkip{Path: d, Reason: fmt.Sprintf("cannot read directory: %v", err)})
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
				out.Skipped = append(out.Skipped, ScanSkip{Path: app, Reason: err.Error()})
				continue
			}
			out.Reports = append(out.Reports, report)
		}
	}
	return out, nil
}

// ListInstalled preserves the original best-effort API for CLI callers that
// only need readable app reports.
func ListInstalled(dirs []string) ([]*InspectionReport, error) {
	result, err := ScanInstalled(dirs)
	if err != nil {
		return nil, err
	}
	return result.Reports, nil
}
