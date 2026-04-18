package clone

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tt-a1i/doppel/internal/core/apperr"
)

func mkSourceBundle(t *testing.T, bundleID string) string {
	t.Helper()
	dir := t.TempDir()
	app := filepath.Join(dir, "src.app")
	if err := os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	plistXML := `<?xml version="1.0"?>
<plist><dict>
<key>CFBundleIdentifier</key><string>` + bundleID + `</string>
<key>CFBundleExecutable</key><string>Src</string>
</dict></plist>`
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plistXML), 0o644); err != nil {
		t.Fatal(err)
	}
	return app
}

func withHelper(t *testing.T, app, helperName, helperBundleID string) {
	t.Helper()
	withHelperAt(t, filepath.Join(app, "Contents", "Helpers"), helperName, helperBundleID)
}

func withHelperAt(t *testing.T, root, helperName, helperBundleID string) {
	t.Helper()
	hd := root
	if err := os.MkdirAll(hd, 0o755); err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(hd, helperName)
	if err := os.MkdirAll(filepath.Join(helper, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	plistXML := `<?xml version="1.0"?>
<plist><dict>
<key>CFBundleIdentifier</key><string>` + helperBundleID + `</string>
<key>CFBundleExecutable</key><string>h</string>
</dict></plist>`
	if err := os.WriteFile(filepath.Join(helper, "Contents", "Info.plist"), []byte(plistXML), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDerivePlan_RequiresNameAndBundleID(t *testing.T) {
	src := mkSourceBundle(t, "com.example.app")
	target := filepath.Join(t.TempDir(), "dst.app")

	_, err := DerivePlan(PlanOptions{SourceApp: src, TargetApp: target, BundleID: "com.x"})
	if !errors.Is(err, apperr.ErrInvalidInput) {
		t.Errorf("missing Name should be invalid input, got %v", err)
	}
	_, err = DerivePlan(PlanOptions{SourceApp: src, TargetApp: target, Name: "x"})
	if !errors.Is(err, apperr.ErrInvalidInput) {
		t.Errorf("missing BundleID should be invalid input, got %v", err)
	}
}

func TestDerivePlan_DefaultTargetUnderDefaultDir(t *testing.T) {
	src := mkSourceBundle(t, "com.example.app")

	origDir := DefaultTargetDir
	DefaultTargetDir = t.TempDir()
	defer func() { DefaultTargetDir = origDir }()

	plan, err := DerivePlan(PlanOptions{
		SourceApp: src,
		Name:      "myclone",
		BundleID:  "com.example.clone",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantTarget := filepath.Join(DefaultTargetDir, "myclone.app")
	if plan.TargetApp != wantTarget {
		t.Errorf("TargetApp = %q, want %q", plan.TargetApp, wantTarget)
	}
}

func TestDerivePlan_RefusesSourceEqualTarget(t *testing.T) {
	src := mkSourceBundle(t, "com.example.app")
	_, err := DerivePlan(PlanOptions{
		SourceApp: src,
		TargetApp: src,
		Name:      "same",
		BundleID:  "com.example.same",
	})
	if !errors.Is(err, apperr.ErrInvalidInput) {
		t.Errorf("expected invalid input when target == source, got %v", err)
	}
}

func TestDerivePlan_RefusesExistingTarget(t *testing.T) {
	src := mkSourceBundle(t, "com.example.app")
	target := filepath.Join(t.TempDir(), "existing.app")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := DerivePlan(PlanOptions{
		SourceApp: src,
		TargetApp: target,
		Name:      "x",
		BundleID:  "com.x",
	})
	if !errors.Is(err, apperr.ErrTargetExists) {
		t.Errorf("expected ErrTargetExists, got %v", err)
	}
}

func TestDerivePlan_DisplayNameDefaultsToName(t *testing.T) {
	src := mkSourceBundle(t, "com.example.app")
	target := filepath.Join(t.TempDir(), "dst.app")

	plan, err := DerivePlan(PlanOptions{
		SourceApp: src, TargetApp: target, Name: "MyClone", BundleID: "com.x.y",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.DisplayNameAfter != "MyClone" {
		t.Errorf("DisplayNameAfter = %q, want MyClone", plan.DisplayNameAfter)
	}
}

func TestDerivePlan_HelperRewritePrefix(t *testing.T) {
	src := mkSourceBundle(t, "com.example.app")
	withHelper(t, src, "MyHelper.app", "com.example.app.helper")
	withHelper(t, src, "Unrelated.app", "org.other.thing")

	target := filepath.Join(t.TempDir(), "dst.app")
	plan, err := DerivePlan(PlanOptions{
		SourceApp: src,
		TargetApp: target,
		Name:      "cloned",
		BundleID:  "com.example.clone",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.HelperRewrites) != 1 {
		t.Fatalf("expected 1 rewrite, got %d: %+v", len(plan.HelperRewrites), plan.HelperRewrites)
	}
	r := plan.HelperRewrites[0]
	if r.OldBundleID != "com.example.app.helper" {
		t.Errorf("Old = %q", r.OldBundleID)
	}
	if r.NewBundleID != "com.example.clone.helper" {
		t.Errorf("New = %q", r.NewBundleID)
	}
	if !strings.HasSuffix(r.RelativePath, filepath.Join("Contents", "Helpers", "MyHelper.app")) {
		t.Errorf("RelativePath = %q", r.RelativePath)
	}
}

func TestDerivePlan_ForceAllowsExistingAppTarget(t *testing.T) {
	src := mkSourceBundle(t, "com.example.app")
	target := filepath.Join(t.TempDir(), "existing.app")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := DerivePlan(PlanOptions{
		SourceApp: src, TargetApp: target,
		Name: "x", BundleID: "com.x.y", Force: true,
	})
	if err != nil {
		t.Fatalf("Force should allow existing target, got %v", err)
	}
	if !plan.Force {
		t.Error("plan.Force not propagated")
	}
}

func TestDerivePlan_ForceRejectsNonAppTarget(t *testing.T) {
	src := mkSourceBundle(t, "com.example.app")
	target := filepath.Join(t.TempDir(), "existing_not_app")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := DerivePlan(PlanOptions{
		SourceApp: src, TargetApp: target,
		Name: "x", BundleID: "com.x.y", Force: true,
	})
	if !errors.Is(err, apperr.ErrInvalidInput) {
		t.Errorf("--force on non-.app should reject, got %v", err)
	}
}

func TestDerivePlan_HelperEqualToParent(t *testing.T) {
	src := mkSourceBundle(t, "com.example.app")
	withHelper(t, src, "Same.app", "com.example.app")

	target := filepath.Join(t.TempDir(), "dst.app")
	plan, err := DerivePlan(PlanOptions{
		SourceApp: src, TargetApp: target, Name: "cloned", BundleID: "com.example.clone",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.HelperRewrites) != 1 {
		t.Fatalf("expected 1 rewrite, got %d", len(plan.HelperRewrites))
	}
	if plan.HelperRewrites[0].NewBundleID != "com.example.clone" {
		t.Errorf("rewrite NewBundleID = %q, want com.example.clone", plan.HelperRewrites[0].NewBundleID)
	}
}

func TestDerivePlan_FrameworkHelperRewritePrefix(t *testing.T) {
	src := mkSourceBundle(t, "com.example.app")
	withHelperAt(t, filepath.Join(src, "Contents", "Frameworks"), "Electron Helper.app", "com.example.app.helper")

	target := filepath.Join(t.TempDir(), "dst.app")
	plan, err := DerivePlan(PlanOptions{
		SourceApp: src,
		TargetApp: target,
		Name:      "cloned",
		BundleID:  "com.example.clone",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.HelperRewrites) != 1 {
		t.Fatalf("expected 1 rewrite, got %d: %+v", len(plan.HelperRewrites), plan.HelperRewrites)
	}
	r := plan.HelperRewrites[0]
	if r.OldBundleID != "com.example.app.helper" {
		t.Errorf("Old = %q", r.OldBundleID)
	}
	if r.NewBundleID != "com.example.clone.helper" {
		t.Errorf("New = %q", r.NewBundleID)
	}
	if !strings.HasSuffix(r.RelativePath, filepath.Join("Contents", "Frameworks", "Electron Helper.app")) {
		t.Errorf("RelativePath = %q", r.RelativePath)
	}
}
