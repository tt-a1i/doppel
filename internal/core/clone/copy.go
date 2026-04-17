package clone

import (
	"context"
	"fmt"

	"github.com/tt-a1i/appclone/internal/core/apperr"
	"github.com/tt-a1i/appclone/internal/core/macos"
)

// CopyBundle copies plan.SourceApp → plan.TargetApp using /usr/bin/ditto so
// extended attributes, ACLs, and resource forks survive.
// No-ops when plan.DryRun is true.
func CopyBundle(ctx context.Context, plan *ClonePlan, ex macos.Execer) error {
	if plan == nil {
		return fmt.Errorf("%w: nil plan", apperr.ErrInvalidInput)
	}
	if plan.DryRun {
		return nil
	}
	return macos.Copy(ctx, ex, plan.SourceApp, plan.TargetApp)
}
