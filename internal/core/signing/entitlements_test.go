package signing

import (
	"strings"
	"testing"

	"github.com/tt-a1i/doppel/internal/core/plistops"
)

func TestFilterEntitlements_StripsIdentityKeys(t *testing.T) {
	src := plistops.Plist{
		"com.apple.security.app-sandbox":                true,
		"application-identifier":                        "ABCDE12345.com.example.app",
		"keychain-access-groups":                        []any{"ABCDE12345.com.example.app"},
		"com.apple.developer.team-identifier":           "ABCDE12345",
		"com.apple.developer.ubiquity-container-identifiers": []any{"iCloud.com.example.app"},
	}
	out, changes := FilterEntitlements(src, "com.example.app", "com.example.clone")
	if out["com.apple.security.app-sandbox"] != true {
		t.Errorf("sandbox entitlement preserved, got %v", out["com.apple.security.app-sandbox"])
	}
	if _, has := out["application-identifier"]; has {
		t.Error("application-identifier should be stripped")
	}
	if _, has := out["keychain-access-groups"]; has {
		t.Error("keychain-access-groups should be stripped")
	}
	stripped := strings.Join(changes, ",")
	if !strings.Contains(stripped, "stripped:application-identifier") {
		t.Errorf("changes missing application-identifier strip: %v", changes)
	}
}

func TestFilterEntitlements_RewritesStringValuesContainingOldID(t *testing.T) {
	src := plistops.Plist{
		"com.apple.security.inherit":                         true,
		"com.apple.developer.default-data-protection":        "com.example.app.default",
	}
	out, changes := FilterEntitlements(src, "com.example.app", "com.example.clone")
	val, _ := out["com.apple.developer.default-data-protection"].(string)
	if val != "com.example.clone.default" {
		t.Errorf("expected rewritten value, got %q", val)
	}
	if !containsPrefix(changes, "rewrote:") {
		t.Errorf("expected 'rewrote:' change, got %v", changes)
	}
}

func TestFilterEntitlements_NilInput(t *testing.T) {
	out, changes := FilterEntitlements(nil, "a", "b")
	if out != nil || changes != nil {
		t.Errorf("expected nil, got %v / %v", out, changes)
	}
}

func containsPrefix(ss []string, prefix string) bool {
	for _, s := range ss {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
