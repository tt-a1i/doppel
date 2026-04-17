package clone

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tt-a1i/appclone/internal/core/macos"
	"github.com/tt-a1i/appclone/internal/core/plistops"
	"github.com/tt-a1i/appclone/internal/core/signing"
	"github.com/tt-a1i/appclone/internal/core/verify"
)

type StageStatus string

const (
	StageStart StageStatus = "start"
	StageOK    StageStatus = "ok"
	StageWarn  StageStatus = "warn"
	StageError StageStatus = "error"
	StageSkip  StageStatus = "skip"
)

type StageEvent struct {
	Stage   string
	Status  StageStatus
	Message string
}

type RunResult struct {
	Plan       *ClonePlan
	Items      []signing.SignableItem
	EntChanges []string
	Verify     *verify.VerifyReport
	Warnings   []string
}

// Run executes the full clone pipeline. When events is non-nil, each stage
// emits start + terminal events on it. The channel is NOT closed by Run;
// caller owns its lifecycle. Callers should use a buffered channel or drain
// concurrently to avoid blocking the pipeline.
func Run(ctx context.Context, plan *ClonePlan, ex macos.Execer, events chan<- StageEvent) (*RunResult, error) {
	result := &RunResult{Plan: plan}
	emit := func(stage string, status StageStatus, msg string) {
		if events != nil {
			events <- StageEvent{Stage: stage, Status: status, Message: msg}
		}
	}

	// 1. Copy
	emit("copy", StageStart, fmt.Sprintf("%s → %s", plan.SourceApp, plan.TargetApp))
	if plan.DryRun {
		emit("copy", StageSkip, "dry-run")
	} else {
		if err := CopyBundle(ctx, plan, ex); err != nil {
			emit("copy", StageError, err.Error())
			return result, err
		}
		emit("copy", StageOK, "bundle copied")
	}

	// 2. Mutate plists (identity + helpers + strip integrity keys)
	emit("plist", StageStart, fmt.Sprintf("%s → %s", plan.BundleIDBefore, plan.BundleIDAfter))
	if plan.DryRun {
		emit("plist", StageSkip, "dry-run")
	} else {
		stripped, err := MutatePlists(plan)
		if err != nil {
			emit("plist", StageError, err.Error())
			return result, err
		}
		parts := []string{"identity written"}
		if n := len(plan.HelperRewrites); n > 0 {
			parts[0] = fmt.Sprintf("identity + %d helper rewrite(s)", n)
		}
		if len(stripped) > 0 {
			parts = append(parts, fmt.Sprintf("stripped %s", strings.Join(stripped, ", ")))
			for _, k := range stripped {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("stripped %s from clone Info.plist — required for clone to launch", k))
			}
		}
		emit("plist", StageOK, strings.Join(parts, " · "))
	}

	// 3. Entitlements (extract from source, filter for new identity)
	emit("entitlements", StageStart, "extracting from source")
	ent, changes, err := signing.ExtractAndFilterEntitlements(ctx, ex, plan.SourceApp, plan.BundleIDBefore, plan.BundleIDAfter)
	if err != nil {
		emit("entitlements", StageWarn, "extraction failed: "+err.Error())
		result.Warnings = append(result.Warnings, "entitlements extraction failed: "+err.Error())
	}
	// On hardened-runtime sources, ad-hoc re-sign triggers library
	// validation that refuses all dylibs (since ad-hoc has no Team ID).
	// Add disable-library-validation so the clone can actually launch.
	meta, _ := macos.GetSigningMeta(ctx, ex, plan.SourceApp)
	if meta != nil && meta.HardenedRuntime {
		if ent == nil {
			ent = plistops.Plist{}
		}
		ent["com.apple.security.cs.disable-library-validation"] = true
		changes = append(changes, "added:disable-library-validation")
	}
	if ent == nil {
		emit("entitlements", StageSkip, "source has no entitlements")
	} else {
		result.EntChanges = changes
		emit("entitlements", StageOK, fmt.Sprintf("filtered (%d changes)", len(changes)))
	}

	// 4. Discover: enumerate all signables (for reporting)
	emit("discover", StageStart, "walking bundle")
	scanRoot := plan.TargetApp
	if plan.DryRun {
		scanRoot = plan.SourceApp
	}
	allItems, _ := signing.Discover(scanRoot)
	result.Items = allItems

	// Build the *re-sign set*: only bundles whose Info.plist we mutated.
	// Nested frameworks / XPC / plugins we didn't touch keep their vendor
	// signatures — this is what preserves Library Validation compatibility.
	// Re-signing them ad-hoc drops the Team ID and breaks dylib loading at
	// launch on modern macOS.
	toSign := buildResignSet(plan, allItems)
	emit("discover", StageOK, fmt.Sprintf("%d found · %d need re-signing", len(allItems), len(toSign)))

	// 5. Re-sign only mutated bundles
	emit("resign", StageStart, "deepest-first, ad-hoc")
	if plan.DryRun {
		emit("resign", StageSkip, "dry-run")
	} else {
		err := signing.DeepResign(ctx, ex, toSign, signing.ResignOptions{
			Entitlements:  ent,
			Force:         true,
			TimestampNone: true,
		})
		if err != nil {
			emit("resign", StageError, err.Error())
			return result, err
		}
		emit("resign", StageOK, fmt.Sprintf("%d item(s) signed", len(toSign)))
	}

	// 6. Verify
	emit("verify", StageStart, "codesign --deep --strict")
	if plan.DryRun {
		emit("verify", StageSkip, "dry-run")
	} else {
		vr, _ := verify.Verify(ctx, plan.TargetApp, verify.VerifyOptions{
			RunSPCTL:      true,
			RunLaunchTest: plan.LaunchTest,
		}, ex)
		result.Verify = vr
		switch {
		case vr != nil && len(vr.Errors) > 0:
			emit("verify", StageError, vr.Errors[0])
			return result, fmt.Errorf("verify failed: %s", vr.Errors[0])
		case vr != nil && vr.LaunchTest != nil && vr.LaunchTest.Survived:
			emit("verify", StageOK, fmt.Sprintf("signature valid · launch test survived %.1fs",
				float64(vr.LaunchTest.SurvivedMs)/1000))
		case vr != nil && len(vr.Warnings) > 0:
			emit("verify", StageWarn, vr.Warnings[0])
		default:
			emit("verify", StageOK, "signature valid")
		}
	}

	return result, nil
}

// MutatePlists rewrites the target bundle's Info.plist to the clone identity
// and applies any helper rewrites listed in the plan. Also strips known
// anti-clone integrity keys (ElectronAsarIntegrity, etc.) whose checks
// would abort the clone on startup. Returns any stripped keys for reporting.
// Helper Info.plists that cannot be read are skipped silently (best-effort —
// helpers sometimes ship broken symlinks or non-standard layouts).
func MutatePlists(plan *ClonePlan) ([]string, error) {
	mainPlist := filepath.Join(plan.TargetApp, "Contents", "Info.plist")
	p, format, err := plistops.Read(mainPlist)
	if err != nil {
		return nil, fmt.Errorf("read target plist: %w", err)
	}
	plistops.SetIdentity(p, plan.BundleIDAfter, plan.NameAfter, plan.DisplayNameAfter)
	stripped := plistops.StripIntegrityKeys(p)
	if err := plistops.Write(mainPlist, p, format); err != nil {
		return nil, fmt.Errorf("write target plist: %w", err)
	}

	for _, r := range plan.HelperRewrites {
		hp := filepath.Join(plan.TargetApp, r.RelativePath, "Contents", "Info.plist")
		pp, pf, err := plistops.Read(hp)
		if err != nil {
			continue
		}
		pp["CFBundleIdentifier"] = r.NewBundleID
		plistops.StripIntegrityKeys(pp) // helpers rarely have these but be safe
		if err := plistops.Write(hp, pp, pf); err != nil {
			return nil, fmt.Errorf("write helper plist %s: %w", hp, err)
		}
	}
	return stripped, nil
}

// buildResignSet returns only the signables whose on-disk contents we
// actually mutated — the main bundle (plist changed) plus any helper app
// whose bundle ID we rewrote. Untouched frameworks keep their vendor
// signatures so Library Validation / hardened-runtime loads still work.
func buildResignSet(plan *ClonePlan, all []signing.SignableItem) []signing.SignableItem {
	// Collect absolute paths we rewrote.
	rewritten := map[string]bool{}
	for _, r := range plan.HelperRewrites {
		rewritten[filepath.Join(plan.TargetApp, r.RelativePath)] = true
	}
	var out []signing.SignableItem
	for _, it := range all {
		if it.Kind == signing.KindMainBundle || rewritten[it.Path] {
			out = append(out, it)
		}
	}
	// Preserve deepest-first order from Discover.
	return out
}
