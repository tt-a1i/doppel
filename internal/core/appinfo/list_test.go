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
