package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tt-a1i/doppel/internal/core/apperr"
	"github.com/tt-a1i/doppel/internal/core/macos"
)

func TestRunClonePreflight_BlocksDoctorErrors(t *testing.T) {
	app := mkCLIApp(t)
	ex := &macos.FakeExecer{
		Responses: map[string]macos.FakeResponse{
			"codesign -d --entitlements :- " + app:                 {ExitCode: 1, Stderr: []byte("not signed at all")},
			"codesign --verify --verbose=2 --deep --strict " + app: {ExitCode: 1, Stderr: []byte("resource fork bad")},
			"codesign -d --verbose=4 " + app:                       {ExitCode: 1},
		},
	}

	findings, err := runClonePreflight(context.Background(), ex, app, false)
	if !errors.Is(err, apperr.ErrInvalidInput) {
		t.Fatalf("preflight error = %v, want ErrInvalidInput", err)
	}
	if len(findings) == 0 || findings[0].Code != "codesign_failed" {
		t.Fatalf("findings = %+v, want codesign_failed", findings)
	}
}

func TestRunClonePreflight_SkipDoctorBypassesChecks(t *testing.T) {
	app := mkCLIApp(t)
	ex := &macos.FakeExecer{
		Default: macos.FakeResponse{ExitCode: 1, Stderr: []byte("would fail")},
	}

	findings, err := runClonePreflight(context.Background(), ex, app, true)
	if err != nil {
		t.Fatalf("skip preflight error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("skip findings = %+v, want none", findings)
	}
}

func mkCLIApp(t *testing.T) string {
	t.Helper()
	app := filepath.Join(t.TempDir(), "Example.app")
	if err := os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(app, "Contents", "_CodeSignature"), 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0"?>
<plist><dict>
<key>CFBundleIdentifier</key><string>com.example.app</string>
<key>CFBundleExecutable</key><string>Example</string>
</dict></plist>`
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "MacOS", "Example"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "_CodeSignature", "CodeResources"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return app
}
