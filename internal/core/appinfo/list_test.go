package appinfo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListInstalled_SkipsUnreadableAndNonApp(t *testing.T) {
	dir := t.TempDir()

	// Make a valid bundle
	good := filepath.Join(dir, "Good.app")
	if err := os.MkdirAll(filepath.Join(good, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0"?>
<plist><dict>
<key>CFBundleIdentifier</key><string>com.test.good</string>
<key>CFBundleExecutable</key><string>Good</string>
</dict></plist>`
	if err := os.WriteFile(filepath.Join(good, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}

	// Non-app dir
	if err := os.MkdirAll(filepath.Join(dir, "NotAnApp"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Broken .app (no Info.plist)
	if err := os.MkdirAll(filepath.Join(dir, "Broken.app"), 0o755); err != nil {
		t.Fatal(err)
	}

	reports, err := ListInstalled([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].Identity.BundleID != "com.test.good" {
		t.Errorf("wrong report: %+v", reports[0])
	}
}

func TestScanInstalled_ReportsSkippedApps(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Broken.app"), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := ScanInstalled([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Reports) != 0 {
		t.Fatalf("expected no reports, got %d", len(result.Reports))
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected one skipped app, got %+v", result.Skipped)
	}
	if result.Skipped[0].Path != filepath.Join(dir, "Broken.app") {
		t.Fatalf("skipped path = %q", result.Skipped[0].Path)
	}
	if result.Skipped[0].Reason == "" {
		t.Fatal("skipped reason should be user-visible")
	}
}

func TestScanInstalled_IgnoresMissingScanDirectory(t *testing.T) {
	result, err := ScanInstalled([]string{filepath.Join(t.TempDir(), "missing")})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skipped) != 0 {
		t.Fatalf("missing scan directory should not be user-visible skip noise: %+v", result.Skipped)
	}
}

func TestListInstalled_DedupAcrossDirs(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "Good.app")
	if err := os.MkdirAll(filepath.Join(good, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0"?>
<plist><dict><key>CFBundleIdentifier</key><string>com.test.dup</string>
<key>CFBundleExecutable</key><string>Good</string></dict></plist>`
	_ = os.WriteFile(filepath.Join(good, "Contents", "Info.plist"), []byte(plist), 0o644)

	// Pass the same dir twice.
	reports, err := ListInstalled([]string{dir, dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 (dedupped), got %d", len(reports))
	}
}
