package appinfo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAppIdentityJSONUsesStableSnakeCase(t *testing.T) {
	b, err := json.Marshal(AppIdentity{
		AppPath:        "/Applications/Foo.app",
		BundleID:       "com.example.foo",
		BundleName:     "Foo",
		DisplayName:    "Foo",
		ExecutableName: "Foo",
		Version:        "1.0.0",
		Build:          "100",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"app_path", "bundle_id", "bundle_name", "display_name", "executable_name", "version", "build"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("missing JSON key %q in %s", key, b)
		}
	}
	if _, ok := got["BundleID"]; ok {
		t.Fatalf("unstable Go field key leaked in JSON: %s", b)
	}
}

const fullPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleIdentifier</key><string>com.example.app</string>
    <key>CFBundleName</key><string>Example</string>
    <key>CFBundleDisplayName</key><string>Example App</string>
    <key>CFBundleExecutable</key><string>Example</string>
    <key>CFBundleShortVersionString</key><string>1.2.3</string>
    <key>CFBundleVersion</key><string>456</string>
</dict>
</plist>`

type bundleOpt func(app string) error

func withExecutable(name string) bundleOpt {
	return func(app string) error {
		return os.WriteFile(filepath.Join(app, "Contents", "MacOS", name), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	}
}

func withCodeSignature() bundleOpt {
	return func(app string) error {
		dir := filepath.Join(app, "Contents", "_CodeSignature")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "CodeResources"), []byte("<plist/>"), 0o644)
	}
}

func makeBundle(t *testing.T, name, plistXML string, opts ...bundleOpt) string {
	t.Helper()
	app := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plistXML), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, opt := range opts {
		if err := opt(app); err != nil {
			t.Fatal(err)
		}
	}
	return app
}

func TestInspect_FullIdentity(t *testing.T) {
	app := makeBundle(t, "full.app", fullPlist, withExecutable("Example"), withCodeSignature())
	report, err := Inspect(app)
	if err != nil {
		t.Fatal(err)
	}
	got := report.Identity
	checks := []struct{ name, got, want string }{
		{"BundleID", got.BundleID, "com.example.app"},
		{"BundleName", got.BundleName, "Example"},
		{"DisplayName", got.DisplayName, "Example App"},
		{"ExecutableName", got.ExecutableName, "Example"},
		{"Version", got.Version, "1.2.3"},
		{"Build", got.Build, "456"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if !report.HasSignature {
		t.Error("expected HasSignature=true")
	}
	if report.Executable == "" {
		t.Error("expected Executable path resolved")
	}
}

func TestInspect_MissingOptionalFields(t *testing.T) {
	minimal := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
    <key>CFBundleIdentifier</key><string>com.example.min</string>
    <key>CFBundleExecutable</key><string>Min</string>
</dict>
</plist>`
	app := makeBundle(t, "min.app", minimal, withExecutable("Min"))
	report, err := Inspect(app)
	if err != nil {
		t.Fatal(err)
	}
	if report.Identity.BundleID != "com.example.min" {
		t.Errorf("BundleID = %q", report.Identity.BundleID)
	}
	if report.Identity.DisplayName != "" {
		t.Errorf("DisplayName should be empty, got %q", report.Identity.DisplayName)
	}
	if report.Identity.Version != "" {
		t.Errorf("Version should be empty, got %q", report.Identity.Version)
	}
	if report.HasSignature {
		t.Error("expected HasSignature=false")
	}
}

func TestInspect_MissingExecutableFile(t *testing.T) {
	app := makeBundle(t, "noexec.app", fullPlist)
	report, err := Inspect(app)
	if err != nil {
		t.Fatal(err)
	}
	if report.Executable != "" {
		t.Errorf("expected empty Executable when file missing, got %q", report.Executable)
	}
	if report.Identity.ExecutableName != "Example" {
		t.Errorf("ExecutableName = %q", report.Identity.ExecutableName)
	}
}

func TestInspect_MalformedPlist(t *testing.T) {
	app := makeBundle(t, "bad.app", "not valid plist", withExecutable("X"))
	_, err := Inspect(app)
	if err == nil {
		t.Fatal("expected error on malformed plist")
	}
}
