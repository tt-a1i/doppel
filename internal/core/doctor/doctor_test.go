package doctor

import (
	"encoding/json"
	"testing"

	"github.com/tt-a1i/doppel/internal/core/plistops"
	"github.com/tt-a1i/doppel/internal/core/signing"
)

func findByCode(findings []Finding, code string) *Finding {
	for i := range findings {
		if findings[i].Code == code {
			return &findings[i]
		}
	}
	return nil
}

func TestBlockingFindings_ReturnsOnlyErrorSeverity(t *testing.T) {
	findings := []Finding{
		{Code: "sandbox_entitled", Severity: "warn"},
		{Code: "codesign_failed", Severity: "error"},
		{Code: "electron_helper", Severity: "info"},
	}
	blocking := BlockingFindings(findings)
	if len(blocking) != 1 {
		t.Fatalf("blocking findings = %d, want 1", len(blocking))
	}
	if blocking[0].Code != "codesign_failed" {
		t.Fatalf("blocking code = %s, want codesign_failed", blocking[0].Code)
	}
}

func TestSummarizeCompatibilityLevels(t *testing.T) {
	tests := []struct {
		name     string
		findings []Finding
		want     string
	}{
		{name: "ready", findings: nil, want: "ready"},
		{name: "caution", findings: []Finding{{Code: "sparkle_present", Severity: "warn"}}, want: "caution"},
		{name: "blocked", findings: []Finding{{Code: "codesign_failed", Severity: "error"}}, want: "blocked"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SummarizeCompatibility(tc.findings)
			if got.Level != tc.want {
				t.Fatalf("Level = %q, want %q", got.Level, tc.want)
			}
			if got.Title == "" || got.Recommendation == "" {
				t.Fatalf("summary should include user-facing title and recommendation: %+v", got)
			}
		})
	}
}

func TestCompatibilitySummaryJSONUsesStableSnakeCase(t *testing.T) {
	b, err := json.Marshal(SummarizeCompatibility([]Finding{{Code: "codesign_failed", Severity: "error"}}))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"level", "title", "recommendation", "error_count", "warning_count", "info_count"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("missing JSON key %q in %s", key, b)
		}
	}
}

func TestFindingJSONUsesStableSnakeCase(t *testing.T) {
	b, err := json.Marshal(Finding{
		Code:     "codesign_failed",
		Title:    "codesign verify reports problems",
		Severity: "error",
		Category: "signature",
		Evidence: []string{"bad signature"},
		Fix:      "fix it",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"code", "title", "severity", "category", "evidence", "fix"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("missing JSON key %q in %s", key, b)
		}
	}
	if _, ok := got["Code"]; ok {
		t.Fatalf("unstable Go field key leaked in JSON: %s", b)
	}
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
