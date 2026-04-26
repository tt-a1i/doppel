package doctor

import (
	"context"
	"strings"

	"howett.net/plist"

	"github.com/tt-a1i/doppel/internal/core/appinfo"
	"github.com/tt-a1i/doppel/internal/core/macos"
	"github.com/tt-a1i/doppel/internal/core/plistops"
	"github.com/tt-a1i/doppel/internal/core/signing"
)

type Finding struct {
	Code     string   `json:"code"`
	Title    string   `json:"title"`
	Severity string   `json:"severity"` // "info" | "warn" | "error"
	Category string   `json:"category"`
	Evidence []string `json:"evidence,omitempty"`
	Fix      string   `json:"fix,omitempty"`
}

type Input struct {
	AppPath         string
	Identity        appinfo.AppIdentity
	HasSignature    bool
	ExecutableOK    bool
	SignableItems   []signing.SignableItem
	Entitlements    plistops.Plist
	CodesignOK      bool
	CodesignStderr  string
	HardenedRuntime bool
	SourceTeamID    string
}

var rules = []func(Input) *Finding{
	ruleMissingExecutable,
	ruleCodesignFailed,
	ruleSandbox,
	ruleSparkle,
	ruleElectron,
	ruleLoginItem,
	ruleUnsigned,
	ruleHardenedRuntime,
}

func Diagnose(in Input) []Finding {
	var out []Finding
	for _, r := range rules {
		if f := r(in); f != nil {
			out = append(out, *f)
		}
	}
	return out
}

func BlockingFindings(findings []Finding) []Finding {
	var out []Finding
	for _, f := range findings {
		if f.Severity == "error" {
			out = append(out, f)
		}
	}
	return out
}

// DiagnoseApp is a convenience wrapper that gathers Input from live tools
// and runs Diagnose in one call.
func DiagnoseApp(ctx context.Context, ex macos.Execer, appPath string) ([]Finding, error) {
	insp, err := appinfo.Inspect(appPath)
	if err != nil {
		return nil, err
	}
	items, _ := signing.Discover(appPath)

	var ent plistops.Plist
	if entBytes, _ := macos.ExtractEntitlements(ctx, ex, appPath); len(entBytes) > 0 {
		_, _ = plist.Unmarshal(entBytes, &ent)
	}
	cs, _ := macos.Verify(ctx, ex, appPath, true, true)
	meta, _ := macos.GetSigningMeta(ctx, ex, appPath)

	in := Input{
		AppPath:       appPath,
		Identity:      insp.Identity,
		HasSignature:  insp.HasSignature,
		ExecutableOK:  insp.Executable != "",
		SignableItems: items,
		Entitlements:  ent,
	}
	if cs != nil {
		in.CodesignOK = cs.OK
		in.CodesignStderr = cs.Stderr
	}
	if meta != nil {
		in.HardenedRuntime = meta.HardenedRuntime
		in.SourceTeamID = meta.TeamID
	}
	return Diagnose(in), nil
}

func ruleMissingExecutable(in Input) *Finding {
	if in.ExecutableOK {
		return nil
	}
	evidence := in.Identity.ExecutableName
	if evidence == "" {
		evidence = "(CFBundleExecutable missing)"
	}
	return &Finding{
		Code:     "missing_executable",
		Title:    "Main executable does not resolve",
		Severity: "error",
		Category: "executable",
		Evidence: []string{evidence},
		Fix:      "CFBundleExecutable points to a file that does not exist under Contents/MacOS/. The bundle is broken; a clone will not launch.",
	}
}

func ruleCodesignFailed(in Input) *Finding {
	if in.CodesignOK {
		return nil
	}
	// Only surface this when we actually ran codesign (stderr present).
	if in.CodesignStderr == "" && !in.HasSignature {
		return nil
	}
	return &Finding{
		Code:     "codesign_failed",
		Title:    "codesign verify reports problems",
		Severity: "error",
		Category: "signature",
		Evidence: []string{strings.TrimSpace(in.CodesignStderr)},
		Fix:      "The clone's signature is not self-consistent. Usually caused by a nested signable that was missed or signed out of order.",
	}
}

func ruleSandbox(in Input) *Finding {
	if in.Entitlements == nil {
		return nil
	}
	v, ok := in.Entitlements["com.apple.security.app-sandbox"].(bool)
	if !ok || !v {
		return nil
	}
	return &Finding{
		Code:     "sandbox_entitled",
		Title:    "App is sandboxed",
		Severity: "warn",
		Category: "sandbox",
		Evidence: []string{"com.apple.security.app-sandbox = true"},
		Fix:      "Sandbox containers are keyed by bundle ID. The clone starts with an empty container — preferences, keychain data, and saved files from the original will not carry over.",
	}
}

func ruleSparkle(in Input) *Finding {
	for _, item := range in.SignableItems {
		if item.Kind == signing.KindFramework && strings.Contains(strings.ToLower(item.Path), "sparkle.framework") {
			return &Finding{
				Code:     "sparkle_present",
				Title:    "Sparkle updater detected",
				Severity: "warn",
				Category: "updater",
				Evidence: []string{item.Path},
				Fix:      "The Sparkle updater validates updates against the original bundle identity and signature. Auto-updates on a clone will typically fail silently or clobber the bundle ID back. Consider disabling updates inside the cloned app.",
			}
		}
	}
	return nil
}

func ruleElectron(in Input) *Finding {
	for _, item := range in.SignableItems {
		if item.Kind == signing.KindFramework && strings.Contains(item.Path, "Electron Framework") {
			return &Finding{
				Code:     "electron_helper",
				Title:    "Electron app — helpers require bundle ID rewrites",
				Severity: "info",
				Category: "helper",
				Evidence: []string{item.Path},
				Fix:      "Electron helpers embed the parent bundle ID. doppel rewrites helper Info.plists before signing so the clone stays internally consistent.",
			}
		}
	}
	return nil
}

func ruleLoginItem(in Input) *Finding {
	for _, item := range in.SignableItems {
		if item.Kind == signing.KindLoginItem {
			return &Finding{
				Code:     "login_item_present",
				Title:    "Login item detected",
				Severity: "warn",
				Category: "helper",
				Evidence: []string{item.Path},
				Fix:      "Login items register by bundle ID through ServiceManagement/SMLoginItem. The clone's login item will not auto-register — enable it manually under System Settings > General > Login Items if needed.",
			}
		}
	}
	return nil
}

func ruleHardenedRuntime(in Input) *Finding {
	if !in.HardenedRuntime {
		return nil
	}
	return &Finding{
		Code:     "hardened_runtime",
		Title:    "Source app uses hardened runtime",
		Severity: "info",
		Category: "signature",
		Evidence: []string{"codesign flags include (runtime)"},
		Fix:      "Apps with hardened runtime sometimes verify their own signature at launch and can abort on clones. Not a guarantee — pass --launch-test to confirm whether the clone actually runs.",
	}
}

func ruleUnsigned(in Input) *Finding {
	if in.HasSignature {
		return nil
	}
	return &Finding{
		Code:     "unsigned_source",
		Title:    "Source app has no code signature",
		Severity: "info",
		Category: "signature",
		Evidence: []string{"no Contents/_CodeSignature/CodeResources"},
		Fix:      "Source bundle ships unsigned; clone will be ad-hoc signed from scratch without entitlements.",
	}
}
