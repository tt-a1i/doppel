package clone

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tt-a1i/appclone/internal/core/appinfo"
	"github.com/tt-a1i/appclone/internal/core/apperr"
	"github.com/tt-a1i/appclone/internal/core/plistops"
	"github.com/tt-a1i/appclone/internal/core/signing"
)

type PlanOptions struct {
	SourceApp   string
	Name        string
	TargetApp   string // optional; default /Applications/<Name>.app
	BundleID    string
	DisplayName string // optional; default Name
	DryRun      bool
}

// HelperRewrite describes a nested bundle whose CFBundleIdentifier should be
// rewritten so it still matches the new parent ID (Electron and similar
// frameworks embed the parent ID as a prefix into helper IDs).
type HelperRewrite struct {
	// RelativePath is relative to the bundle root, e.g.
	// "Contents/Helpers/My Helper.app". Apply to TargetApp to get the real
	// file path.
	RelativePath string
	OldBundleID  string
	NewBundleID  string
}

type ClonePlan struct {
	SourceApp        string
	TargetApp        string
	BundleIDBefore   string
	BundleIDAfter    string
	NameAfter        string
	DisplayNameAfter string
	DryRun           bool
	HelperRewrites   []HelperRewrite
}

// DefaultTargetDir is /Applications; overridable for tests.
var DefaultTargetDir = "/Applications"

func DerivePlan(opts PlanOptions) (*ClonePlan, error) {
	if opts.SourceApp == "" {
		return nil, fmt.Errorf("%w: source app path is required", apperr.ErrInvalidInput)
	}
	if opts.Name == "" {
		return nil, fmt.Errorf("%w: --name is required", apperr.ErrInvalidInput)
	}
	if opts.BundleID == "" {
		return nil, fmt.Errorf("%w: --bundle-id is required", apperr.ErrInvalidInput)
	}
	if err := appinfo.ValidateAppPath(opts.SourceApp); err != nil {
		return nil, err
	}

	report, err := appinfo.Inspect(opts.SourceApp)
	if err != nil {
		return nil, err
	}

	target := opts.TargetApp
	if target == "" {
		target = filepath.Join(DefaultTargetDir, opts.Name+".app")
	}

	src, err := filepath.Abs(opts.SourceApp)
	if err != nil {
		return nil, err
	}
	dst, err := filepath.Abs(target)
	if err != nil {
		return nil, err
	}
	if src == dst {
		return nil, fmt.Errorf("%w: target must differ from source", apperr.ErrInvalidInput)
	}
	if _, err := os.Stat(dst); err == nil {
		return nil, fmt.Errorf("%w: %s", apperr.ErrTargetExists, dst)
	}

	displayName := opts.DisplayName
	if displayName == "" {
		displayName = opts.Name
	}

	plan := &ClonePlan{
		SourceApp:        src,
		TargetApp:        dst,
		BundleIDBefore:   report.Identity.BundleID,
		BundleIDAfter:    opts.BundleID,
		NameAfter:        opts.Name,
		DisplayNameAfter: displayName,
		DryRun:           opts.DryRun,
		HelperRewrites:   computeHelperRewrites(src, report.Identity.BundleID, opts.BundleID),
	}
	return plan, nil
}

// computeHelperRewrites scans nested signable bundles in the source and
// returns a rewrite plan for each one whose CFBundleIdentifier is the source
// bundle ID or starts with "<sourceBundleID>." (Electron helper pattern).
func computeHelperRewrites(sourceApp, oldID, newID string) []HelperRewrite {
	if oldID == "" {
		return nil
	}
	items, _ := signing.Discover(sourceApp)
	var rewrites []HelperRewrite
	for _, item := range items {
		if item.Kind == signing.KindMainBundle {
			continue
		}
		plistPath := filepath.Join(item.Path, "Contents", "Info.plist")
		p, _, err := plistops.Read(plistPath)
		if err != nil {
			continue
		}
		id, _ := p["CFBundleIdentifier"].(string)
		if id == "" {
			continue
		}
		if id != oldID && !strings.HasPrefix(id, oldID+".") {
			continue
		}
		newHelperID := newID + strings.TrimPrefix(id, oldID)
		rel, err := filepath.Rel(sourceApp, item.Path)
		if err != nil {
			continue
		}
		rewrites = append(rewrites, HelperRewrite{
			RelativePath: rel,
			OldBundleID:  id,
			NewBundleID:  newHelperID,
		})
	}
	return rewrites
}
