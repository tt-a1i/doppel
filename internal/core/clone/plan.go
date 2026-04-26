package clone

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/tt-a1i/doppel/internal/core/apperr"
	"github.com/tt-a1i/doppel/internal/core/appinfo"
	"github.com/tt-a1i/doppel/internal/core/plistops"
	"github.com/tt-a1i/doppel/internal/core/signing"
)

type PlanOptions struct {
	SourceApp   string
	Name        string
	TargetApp   string // optional; default <DefaultTargetDir>/<Name>.app
	BundleID    string
	DisplayName string // optional; default Name
	DryRun      bool
	Force       bool // if true, pre-existing target is deleted before copy
	LaunchTest  bool // if true, verify stage briefly launches the clone
}

// HelperRewrite describes a nested bundle whose CFBundleIdentifier should be
// rewritten so it still matches the new parent ID (Electron and similar
// frameworks embed the parent ID as a prefix into helper IDs).
type HelperRewrite struct {
	// RelativePath is relative to the bundle root, e.g.
	// "Contents/Helpers/My Helper.app". Apply to TargetApp to get the real
	// file path.
	RelativePath string `json:"relative_path"`
	OldBundleID  string `json:"old_bundle_id"`
	NewBundleID  string `json:"new_bundle_id"`
}

type ClonePlan struct {
	SourceApp        string
	TargetApp        string
	BundleIDBefore   string
	BundleIDAfter    string
	NameAfter        string
	DisplayNameAfter string
	DryRun           bool
	Force            bool
	LaunchTest       bool
	HelperRewrites   []HelperRewrite
}

// DefaultTargetDir is where clones go when --target is omitted. Prefer the
// per-user Applications folder so ordinary users do not need admin privileges.
// Overridable for tests.
var DefaultTargetDir = defaultTargetDir()

func defaultTargetDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, "Applications")
	}
	return "/Applications"
}

func DerivePlan(opts PlanOptions) (*ClonePlan, error) {
	if opts.SourceApp == "" {
		return nil, fmt.Errorf("%w: source app path is required", apperr.ErrInvalidInput)
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: --name is required", apperr.ErrInvalidInput)
	}
	if err := appinfo.ValidateAppPath(opts.SourceApp); err != nil {
		return nil, err
	}

	report, err := appinfo.Inspect(opts.SourceApp)
	if err != nil {
		return nil, err
	}

	bundleID := strings.TrimSpace(opts.BundleID)
	if bundleID == "" {
		bundleID = DefaultBundleID(report.Identity.BundleID, name)
	}
	if err := appinfo.ValidateBundleID(bundleID); err != nil {
		return nil, err
	}
	if bundleID == report.Identity.BundleID {
		return nil, fmt.Errorf("%w: bundle ID must differ from source: %s", apperr.ErrInvalidInput, bundleID)
	}

	target := strings.TrimSpace(opts.TargetApp)
	if target == "" {
		target = filepath.Join(DefaultTargetDir, name+".app")
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
	if !strings.HasSuffix(dst, ".app") {
		return nil, fmt.Errorf("%w: target path must end in .app: %s", apperr.ErrInvalidInput, dst)
	}
	if _, err := os.Stat(dst); err == nil {
		if !opts.Force {
			return nil, fmt.Errorf("%w: %s (use --force to overwrite)", apperr.ErrTargetExists, dst)
		}
		if err := checkForceTarget(dst); err != nil {
			return nil, err
		}
	}

	displayName := strings.TrimSpace(opts.DisplayName)
	if displayName == "" {
		displayName = name
	}

	plan := &ClonePlan{
		SourceApp:        src,
		TargetApp:        dst,
		BundleIDBefore:   report.Identity.BundleID,
		BundleIDAfter:    bundleID,
		NameAfter:        name,
		DisplayNameAfter: displayName,
		DryRun:           opts.DryRun,
		Force:            opts.Force,
		LaunchTest:       opts.LaunchTest,
		HelperRewrites:   computeHelperRewrites(src, report.Identity.BundleID, bundleID),
	}
	return plan, nil
}

func DefaultBundleID(sourceBundleID, cloneName string) string {
	suffix := bundleIDComponent(cloneName)
	if suffix == "" {
		suffix = "clone"
	}
	sourceBundleID = strings.TrimSpace(sourceBundleID)
	if sourceBundleID == "" || appinfo.ValidateBundleID(sourceBundleID) != nil {
		return "com.example." + suffix
	}
	return sourceBundleID + "." + suffix
}

func bundleIDComponent(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || unicode.IsSpace(r) || r == '.':
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// checkForceTarget refuses to --force-delete paths that don't look like an
// app bundle. This guards against catastrophic typos (e.g. --target /, /etc).
func checkForceTarget(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(abs, ".app") {
		return fmt.Errorf("%w: --force refuses to remove non-.app path %s", apperr.ErrInvalidInput, abs)
	}
	// Path must have at least 2 separators (e.g. /Applications/X.app) to
	// avoid removing something like /X.app sitting at the root.
	if strings.Count(abs, string(filepath.Separator)) < 2 {
		return fmt.Errorf("%w: --force refuses to remove shallow path %s", apperr.ErrInvalidInput, abs)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: --force target is not a directory: %s", apperr.ErrInvalidInput, abs)
	}
	return nil
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
