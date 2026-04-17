package doctor

import (
	"testing"

	"github.com/tt-a1i/appclone/internal/core/plistops"
	"github.com/tt-a1i/appclone/internal/core/signing"
)

func findByCode(findings []Finding, code string) *Finding {
	for i := range findings {
		if findings[i].Code == code {
			return &findings[i]
		}
	}
	return nil
}

func TestDiagnose_CleanApp(t *testing.T) {
	in := Input{
		ExecutableOK: true,
		HasSignature: true,
		CodesignOK:   true,
	}
	findings := Diagnose(in)
	if len(findings) != 0 {
		t.Errorf("expected no findings, got %+v", findings)
	}
}

func TestDiagnose_MissingExecutable(t *testing.T) {
	findings := Diagnose(Input{ExecutableOK: false, HasSignature: true, CodesignOK: true})
	f := findByCode(findings, "missing_executable")
	if f == nil {
		t.Fatal("expected missing_executable")
	}
	if f.Severity != "error" {
		t.Errorf("severity = %s, want error", f.Severity)
	}
}

func TestDiagnose_Sparkle(t *testing.T) {
	findings := Diagnose(Input{
		ExecutableOK: true,
		HasSignature: true,
		CodesignOK:   true,
		SignableItems: []signing.SignableItem{
			{Path: "/x/Contents/Frameworks/Sparkle.framework", Kind: signing.KindFramework, Depth: 1},
		},
	})
	if findByCode(findings, "sparkle_present") == nil {
		t.Errorf("expected sparkle_present, got %+v", findings)
	}
}

func TestDiagnose_ElectronInfo(t *testing.T) {
	findings := Diagnose(Input{
		ExecutableOK: true,
		HasSignature: true,
		CodesignOK:   true,
		SignableItems: []signing.SignableItem{
			{Path: "/x/Contents/Frameworks/Electron Framework.framework", Kind: signing.KindFramework, Depth: 1},
		},
	})
	f := findByCode(findings, "electron_helper")
	if f == nil || f.Severity != "info" {
		t.Errorf("expected electron info, got %+v", findings)
	}
}

func TestDiagnose_Sandbox(t *testing.T) {
	findings := Diagnose(Input{
		ExecutableOK: true, HasSignature: true, CodesignOK: true,
		Entitlements: plistops.Plist{"com.apple.security.app-sandbox": true},
	})
	if findByCode(findings, "sandbox_entitled") == nil {
		t.Errorf("expected sandbox_entitled, got %+v", findings)
	}
}

func TestDiagnose_LoginItem(t *testing.T) {
	findings := Diagnose(Input{
		ExecutableOK: true, HasSignature: true, CodesignOK: true,
		SignableItems: []signing.SignableItem{
			{Path: "/x/Contents/Library/LoginItems/L.app", Kind: signing.KindLoginItem, Depth: 1},
		},
	})
	if findByCode(findings, "login_item_present") == nil {
		t.Errorf("expected login_item_present, got %+v", findings)
	}
}

func TestDiagnose_CodesignFailed(t *testing.T) {
	findings := Diagnose(Input{
		ExecutableOK:   true,
		HasSignature:   true,
		CodesignOK:     false,
		CodesignStderr: "resource fork bad",
	})
	f := findByCode(findings, "codesign_failed")
	if f == nil {
		t.Fatal("expected codesign_failed")
	}
	if f.Severity != "error" {
		t.Errorf("severity = %s, want error", f.Severity)
	}
}

func TestDiagnose_UnsignedSourceOnlyWhenNoSignature(t *testing.T) {
	findings := Diagnose(Input{
		ExecutableOK: true, HasSignature: false, CodesignOK: true,
	})
	if findByCode(findings, "unsigned_source") == nil {
		t.Error("expected unsigned_source")
	}
}
