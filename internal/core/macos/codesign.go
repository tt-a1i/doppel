package macos

import (
	"context"
	"fmt"
	"strings"
)

type VerifyResult struct {
	OK     bool
	Deep   bool
	Strict bool
	Stderr string
}

func Verify(ctx context.Context, ex Execer, appPath string, deep, strict bool) (*VerifyResult, error) {
	args := []string{"--verify", "--verbose=2"}
	if deep {
		args = append(args, "--deep")
	}
	if strict {
		args = append(args, "--strict")
	}
	args = append(args, appPath)
	_, stderr, code, err := ex.Run(ctx, "codesign", args...)
	if err != nil {
		return nil, err
	}
	return &VerifyResult{
		OK:     code == 0,
		Deep:   deep,
		Strict: strict,
		Stderr: string(stderr),
	}, nil
}

type SignOptions struct {
	Identity         string // "-" for ad-hoc
	EntitlementsFile string // optional path
	Force            bool
	TimestampNone    bool
}

func Sign(ctx context.Context, ex Execer, path string, opts SignOptions) error {
	var args []string
	if opts.Force {
		args = append(args, "--force")
	}
	args = append(args, "--sign", opts.Identity)
	if opts.TimestampNone {
		args = append(args, "--timestamp=none")
	}
	if opts.EntitlementsFile != "" {
		args = append(args, "--entitlements", opts.EntitlementsFile)
	}
	args = append(args, path)
	_, stderr, code, err := ex.Run(ctx, "codesign", args...)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("codesign failed on %s (exit %d): %s", path, code, strings.TrimSpace(string(stderr)))
	}
	return nil
}

// ExtractEntitlements returns the embedded entitlements plist for an app.
// If the app is unsigned or has no entitlements, returns (nil, nil).
func ExtractEntitlements(ctx context.Context, ex Execer, appPath string) ([]byte, error) {
	stdout, stderr, code, err := ex.Run(ctx, "codesign", "-d", "--entitlements", ":-", appPath)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		msg := string(stderr)
		if containsAny(msg, "not signed at all", "no entitlements", "not found") {
			return nil, nil
		}
		return nil, fmt.Errorf("codesign entitlements extraction failed (exit %d): %s", code, strings.TrimSpace(msg))
	}
	if len(stdout) == 0 {
		return nil, nil
	}
	return stdout, nil
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
