package clone

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/tt-a1i/doppel/internal/core/macos"
	"github.com/tt-a1i/doppel/internal/core/plistops"
)

func TestMutatePlists_MainAndHelper(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "clone.app")
	if err := os.MkdirAll(filepath.Join(target, "Contents", "Helpers", "H.app", "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}

	mainPlist := `<?xml version="1.0"?><plist><dict>
<key>CFBundleIdentifier</key><string>com.old.app</string>
<key>CFBundleName</key><string>Old</string>
</dict></plist>`
	helperPlist := `<?xml version="1.0"?><plist><dict>
<key>CFBundleIdentifier</key><string>com.old.app.helper</string>
</dict></plist>`

	if err := os.WriteFile(filepath.Join(target, "Contents", "Info.plist"), []byte(mainPlist), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "Contents", "Helpers", "H.app", "Contents", "Info.plist"), []byte(helperPlist), 0o644); err != nil {
		t.Fatal(err)
	}

	plan := &ClonePlan{
		TargetApp:        target,
		BundleIDBefore:   "com.old.app",
		BundleIDAfter:    "com.new.app",
		NameAfter:        "New",
		DisplayNameAfter: "New Display",
		HelperRewrites: []HelperRewrite{
			{RelativePath: filepath.Join("Contents", "Helpers", "H.app"), OldBundleID: "com.old.app.helper", NewBundleID: "com.new.app.helper"},
		},
	}
	if _, err := MutatePlists(plan); err != nil {
		t.Fatal(err)
	}

	p, _, err := plistops.Read(filepath.Join(target, "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	if p["CFBundleIdentifier"] != "com.new.app" {
		t.Errorf("main id = %v", p["CFBundleIdentifier"])
	}
	if p["CFBundleName"] != "New" {
		t.Errorf("bundle name = %v", p["CFBundleName"])
	}
	if p["CFBundleDisplayName"] != "New Display" {
		t.Errorf("display name = %v", p["CFBundleDisplayName"])
	}

	hp, _, err := plistops.Read(filepath.Join(target, "Contents", "Helpers", "H.app", "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	if hp["CFBundleIdentifier"] != "com.new.app.helper" {
		t.Errorf("helper id = %v", hp["CFBundleIdentifier"])
	}
}

func TestMutatePlists_FrameworkHelperPreservesBundleName(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "clone.app")
	if err := os.MkdirAll(filepath.Join(target, "Contents", "Resources"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeMinimalAsar(t, filepath.Join(target, "Contents", "Resources", "app.asar"), `{"name":"CherryStudio","productName":"Cherry Studio"}`)

	mainPlist := `<?xml version="1.0"?><plist><dict>
<key>CFBundleIdentifier</key><string>com.old.app</string>
<key>CFBundleName</key><string>Cherry Studio</string>
<key>CFBundleDisplayName</key><string>Cherry Studio</string>
</dict></plist>`
	if err := os.WriteFile(filepath.Join(target, "Contents", "Info.plist"), []byte(mainPlist), 0o644); err != nil {
		t.Fatal(err)
	}
	withHelperAt(t, filepath.Join(target, "Contents", "Frameworks"), "Cherry Studio Helper.app", "com.old.app.helper")

	plan := &ClonePlan{
		TargetApp:        target,
		BundleIDBefore:   "com.old.app",
		BundleIDAfter:    "com.new.app",
		NameAfter:        "Clone Name",
		DisplayNameAfter: "Clone Display",
		HelperRewrites: []HelperRewrite{
			{RelativePath: filepath.Join("Contents", "Frameworks", "Cherry Studio Helper.app"), OldBundleID: "com.old.app.helper", NewBundleID: "com.new.app.helper"},
		},
	}
	if _, err := MutatePlists(plan); err != nil {
		t.Fatal(err)
	}

	p, _, err := plistops.Read(filepath.Join(target, "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	if p["CFBundleIdentifier"] != "com.new.app" {
		t.Errorf("main id = %v", p["CFBundleIdentifier"])
	}
	if p["CFBundleName"] != "Cherry Studio" {
		t.Errorf("bundle name = %v, want Cherry Studio", p["CFBundleName"])
	}
	if p["CFBundleDisplayName"] != "Clone Display" {
		t.Errorf("display name = %v, want Clone Display", p["CFBundleDisplayName"])
	}
}

func TestMutatePlists_RewritesElectronPackageIdentity(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "clone.app")
	if err := os.MkdirAll(filepath.Join(target, "Contents", "Resources"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeMinimalAsar(t, filepath.Join(target, "Contents", "Resources", "app.asar"), `{"name":"CherryStudio","productName":"Cherry Studio"}`)

	mainPlist := `<?xml version="1.0"?><plist><dict>
<key>CFBundleIdentifier</key><string>com.old.app</string>
<key>CFBundleName</key><string>Cherry Studio</string>
<key>CFBundleDisplayName</key><string>Cherry Studio</string>
</dict></plist>`
	if err := os.WriteFile(filepath.Join(target, "Contents", "Info.plist"), []byte(mainPlist), 0o644); err != nil {
		t.Fatal(err)
	}

	plan := &ClonePlan{
		TargetApp:        target,
		BundleIDBefore:   "com.old.app",
		BundleIDAfter:    "com.new.app",
		NameAfter:        "clone-01",
		DisplayNameAfter: "Cherry Studio",
	}
	if _, err := MutatePlists(plan); err != nil {
		t.Fatal(err)
	}

	pkg := readPackageJSONFromAsar(t, filepath.Join(target, "Contents", "Resources", "app.asar"))
	if pkg["name"] != "clone-01" {
		t.Errorf("package name = %v, want clone-01", pkg["name"])
	}
	if pkg["productName"] != "Cherry Studio Clone" {
		t.Errorf("productName = %v, want Cherry Studio Clone", pkg["productName"])
	}
}

func TestRun_DryRunEmitsAllStagesAsSkip(t *testing.T) {
	src := mkSourceBundle(t, "com.example.app")
	target := filepath.Join(t.TempDir(), "dst.app")
	plan, err := DerivePlan(PlanOptions{
		SourceApp: src, TargetApp: target, Name: "c", BundleID: "com.example.clone", DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	ex := &macos.FakeExecer{Default: macos.FakeResponse{ExitCode: 1, Stderr: []byte("not signed at all")}}

	events := make(chan StageEvent, 32)
	_, err = Run(context.Background(), plan, ex, events)
	if err != nil {
		t.Fatalf("dry-run should not error, got %v", err)
	}
	close(events)

	stages := map[string][]StageStatus{}
	for e := range events {
		stages[e.Stage] = append(stages[e.Stage], e.Status)
	}
	for _, s := range []string{"copy", "plist", "resign", "verify"} {
		statuses := stages[s]
		if len(statuses) < 2 || statuses[0] != StageStart {
			t.Errorf("stage %q missing start, got %v", s, statuses)
			continue
		}
		if statuses[len(statuses)-1] != StageSkip {
			t.Errorf("stage %q should end in skip for dry-run, got %v", s, statuses)
		}
	}

	// ditto / codesign sign / verify should not have been invoked.
	for _, call := range ex.Calls {
		if call.Name == "ditto" {
			t.Errorf("dry-run should not call ditto, got %v", call)
		}
	}
}

func writeMinimalAsar(t *testing.T, asarPath, packageJSON string) {
	t.Helper()

	header := map[string]any{
		"files": map[string]any{
			"package.json": map[string]any{
				"size":   len(packageJSON),
				"offset": "0",
			},
		},
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	aligned := asarAlign4(len(headerJSON))
	buf := make([]byte, 16+aligned+len(packageJSON))
	binary.LittleEndian.PutUint32(buf[0:4], 4)
	binary.LittleEndian.PutUint32(buf[4:8], uint32(aligned+8))
	binary.LittleEndian.PutUint32(buf[8:12], uint32(aligned+4))
	binary.LittleEndian.PutUint32(buf[12:16], uint32(len(headerJSON)))
	copy(buf[16:], headerJSON)
	copy(buf[16+aligned:], []byte(packageJSON))
	if err := os.WriteFile(asarPath, buf, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readPackageJSONFromAsar(t *testing.T, asarPath string) map[string]any {
	t.Helper()

	buf, err := os.ReadFile(asarPath)
	if err != nil {
		t.Fatal(err)
	}
	jsonLen := int(binary.LittleEndian.Uint32(buf[12:16]))
	dataStart := 16 + asarAlign4(jsonLen)

	var header map[string]any
	if err := json.Unmarshal(buf[16:16+jsonLen], &header); err != nil {
		t.Fatal(err)
	}
	files, _ := header["files"].(map[string]any)
	entry, _ := files["package.json"].(map[string]any)
	offStr, _ := entry["offset"].(string)
	offset, err := strconv.Atoi(offStr)
	if err != nil {
		t.Fatal(err)
	}
	size, ok := entry["size"].(float64)
	if !ok {
		t.Fatalf("package.json size type = %T", entry["size"])
	}

	var pkg map[string]any
	if err := json.Unmarshal(buf[dataStart+offset:dataStart+offset+int(size)], &pkg); err != nil {
		t.Fatal(err)
	}
	return pkg
}

func asarAlign4(n int) int {
	return (n + 3) &^ 3
}
