package clone

import (
	"context"
	"fmt"
	"os"

	"github.com/tt-a1i/appclone/internal/core/apperr"
	"github.com/tt-a1i/appclone/internal/core/macos"
)

// CopyBundle copies plan.SourceApp → plan.TargetApp using /usr/bin/ditto so
// extended attributes, ACLs, and resource forks survive. When plan.Force is
// true and the target already exists, it's removed first (the path has
// already been validated as an app-shaped path during plan derivation).
// No-ops when plan.DryRun is true.
func CopyBundle(ctx context.Context, plan *ClonePlan, ex macos.Execer) error {
	if plan == nil {
		return fmt.Errorf("%w: nil plan", apperr.ErrInvalidInput)
	}
	if plan.DryRun {
		return nil
	}
	if plan.Force {
		if err := removeExistingTarget(plan.TargetApp); err != nil {
			return err
		}
	}
	return macos.Copy(ctx, ex, plan.SourceApp, plan.TargetApp)
}

// removeExistingTarget deletes path if it exists. Assumes checkForceTarget
// has already guarded against shallow / non-.app paths.
func removeExistingTarget(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.RemoveAll(path)
}
