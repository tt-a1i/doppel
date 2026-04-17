package clone

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tt-a1i/appclone/internal/core/macos"
	"github.com/tt-a1i/appclone/internal/core/plistops"
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
