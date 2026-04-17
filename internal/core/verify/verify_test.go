package verify

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tt-a1i/appclone/internal/core/macos"
)

const testPlist = `<?xml version="1.0"?>
<plist><dict>
<key>CFBundleIdentifier</key><string>com.example.app</string>
<key>CFBundleExecutable</key><string>Example</string>
</dict></plist>`

func mkBundle(t *testing.T, withExec bool) string {
	t.Helper()
	app := filepath.Join(t.TempDir(), "app.app")
	if err := os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(testPlist), 0o644); err != nil {
		t.Fatal(err)
	}
	if withExec {
		if err := os.WriteFile(filepath.Join(app, "Contents", "MacOS", "Example"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return app
}

func TestVerify_Happy(t *testing.T) {
	app := mkBundle(t, true)
	ex := &macos.FakeExecer{Default: macos.FakeResponse{ExitCode: 0}}
	r, err := Verify(context.Background(), app, VerifyOptions{}, ex)
	if err != nil {
		t.Fatal(err)
	}
	if !r.PlistValid || !r.ExecutableResolved {
		t.Errorf("unexpected: plist=%v exec=%v", r.PlistValid, r.ExecutableResolved)
	}
	if r.Codesign == nil || !r.Codesign.OK {
		t.Errorf("codesign should be ok, got %+v", r.Codesign)
	}
	if len(r.Errors) != 0 {
		t.Errorf("unexpected errors: %v", r.Errors)
	}
}

func TestVerify_BadCodesign(t *testing.T) {
	app := mkBundle(t, true)
	ex := &macos.FakeExecer{Default: macos.FakeResponse{ExitCode: 1, Stderr: []byte("invalid signature")}}
	r, _ := Verify(context.Background(), app, VerifyOptions{}, ex)
	if r.Codesign.OK {
		t.Error("expected codesign to fail")
	}
	if len(r.Errors) == 0 {
		t.Error("expected errors for bad codesign")
	}
	if !strings.Contains(strings.Join(r.Errors, " "), "codesign verify failed") {
		t.Errorf("errors should mention codesign, got %v", r.Errors)
	}
}

func TestVerify_SPCTLRejectIsWarning(t *testing.T) {
	app := mkBundle(t, true)
	ex := &macos.FakeExecer{
		Responses: map[string]macos.FakeResponse{
			"codesign --verify --verbose=2 --deep --strict " + app: {ExitCode: 0},
		},
		Default: macos.FakeResponse{ExitCode: 3, Stderr: []byte("rejected")},
	}
	r, err := Verify(context.Background(), app, VerifyOptions{RunSPCTL: true}, ex)
	if err != nil {
		t.Fatal(err)
	}
	if r.Codesign == nil || !r.Codesign.OK {
		t.Errorf("codesign should be ok, got %+v", r.Codesign)
	}
	if r.SPCTL == nil || r.SPCTL.Accepted {
		t.Errorf("SPCTL should be rejected, got %+v", r.SPCTL)
	}
	if len(r.Errors) != 0 {
		t.Errorf("spctl reject should be warning, not error: %v", r.Errors)
	}
	if len(r.Warnings) == 0 {
		t.Error("expected spctl warning")
	}
}

func TestVerify_MissingExecutable(t *testing.T) {
	app := mkBundle(t, false)
	ex := &macos.FakeExecer{Default: macos.FakeResponse{ExitCode: 0}}
	r, _ := Verify(context.Background(), app, VerifyOptions{}, ex)
	if r.ExecutableResolved {
		t.Error("should not resolve missing exec")
	}
	if len(r.Errors) == 0 {
		t.Error("expected error for missing executable")
	}
}
