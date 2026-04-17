package signing

import (
	"context"
	"fmt"
	"os"

	"github.com/tt-a1i/appclone/internal/core/macos"
	"github.com/tt-a1i/appclone/internal/core/plistops"
)

type ResignOptions struct {
	// Entitlements, if non-nil, is applied to the outermost bundle only.
	// Nested signables are signed without an explicit entitlements file so
	// they inherit or keep whatever codesign synthesizes.
	Entitlements plistops.Plist
	Force        bool
	TimestampNone bool
}

// DeepResign signs every item in order (items is assumed to already be
// sorted deepest-first via Discover). An ad-hoc identity ("-") is used.
// Returns on the first failing item.
func DeepResign(ctx context.Context, ex macos.Execer, items []SignableItem, opts ResignOptions) error {
	if len(items) == 0 {
		return fmt.Errorf("no items to sign")
	}

	var entFile string
	if opts.Entitlements != nil {
		var err error
		entFile, err = WriteEntitlementsFile(opts.Entitlements)
		if err != nil {
			return fmt.Errorf("write entitlements: %w", err)
		}
		defer os.Remove(entFile)
	}

	for _, item := range items {
		signOpts := macos.SignOptions{
			Identity:      "-",
			Force:         opts.Force,
			TimestampNone: opts.TimestampNone,
		}
		if item.Kind == KindMainBundle && entFile != "" {
			signOpts.EntitlementsFile = entFile
		}
		if err := macos.Sign(ctx, ex, item.Path, signOpts); err != nil {
			return fmt.Errorf("sign %s (%s): %w", item.Path, item.Kind, err)
		}
	}
	return nil
}
