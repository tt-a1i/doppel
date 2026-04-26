package macos

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

type VerifyResult struct {
	OK     bool   `json:"ok"`
	Deep   bool   `json:"deep"`
	Strict bool   `json:"strict"`
	Stderr string `json:"stderr,omitempty"`
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
	// PreserveFlags carries original code-directory flags (hardened runtime,
	// library-validation, etc.) through the re-sign. Strongly recommended on
	// modern macOS — without it, dyld will refuse to launch hardened-runtime
	// apps once the flag is stripped.
	PreserveFlags bool
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
	if opts.PreserveFlags {
		args = append(args, "--preserve-metadata=flags")
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

// SigningMeta captures a few codesign flags that matter for predicting
// clone compatibility. Populate via GetSigningMeta; a missing/unsigned app
// yields a zero-value struct, not an error.
type SigningMeta struct {
	HardenedRuntime bool
	TeamID          string
	Identifier      string
	Adhoc           bool
}

var codesignFlagsLine = regexp.MustCompile(`CodeDirectory v=\S+ size=\S+ flags=\S+\((.*?)\)`)
var codesignTeamLine = regexp.MustCompile(`TeamIdentifier=(\S+)`)
var codesignIDLine = regexp.MustCompile(`Identifier=(\S+)`)

func GetSigningMeta(ctx context.Context, ex Execer, appPath string) (*SigningMeta, error) {
	_, stderr, code, err := ex.Run(ctx, "codesign", "-d", "--verbose=4", appPath)
	if err != nil {
		return nil, err
	}
	meta := &SigningMeta{}
	if code != 0 {
		return meta, nil
	}
	s := string(stderr)
	if m := codesignFlagsLine.FindStringSubmatch(s); len(m) == 2 {
		flags := m[1]
		meta.HardenedRuntime = strings.Contains(flags, "runtime")
		meta.Adhoc = strings.Contains(flags, "adhoc")
	}
	if m := codesignTeamLine.FindStringSubmatch(s); len(m) == 2 {
		if m[1] != "not" {
			meta.TeamID = m[1]
		}
	}
	if m := codesignIDLine.FindStringSubmatch(s); len(m) == 2 {
		meta.Identifier = m[1]
	}
	return meta, nil
}
