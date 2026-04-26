package verify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tt-a1i/doppel/internal/core/appinfo"
	"github.com/tt-a1i/doppel/internal/core/macos"
)

type VerifyReport struct {
	AppPath            string              `json:"app_path"`
	PlistValid         bool                `json:"plist_valid"`
	ExecutableResolved bool                `json:"executable_resolved"`
	ExecutablePath     string              `json:"executable_path"`
	Codesign           *macos.VerifyResult `json:"codesign,omitempty"`
	SPCTL              *macos.AssessResult `json:"spctl,omitempty"`
	LaunchTest         *LaunchTestResult   `json:"launch_test,omitempty"`
	Warnings           []string            `json:"warnings,omitempty"`
	Errors             []string            `json:"errors,omitempty"`
}

type LaunchTestResult struct {
	Attempted       bool   `json:"attempted"`
	Launched        bool   `json:"launched"`
	Survived        bool   `json:"survived"`
	SurvivedMs      int64  `json:"survived_ms"`
	CrashSummary    string `json:"crash_summary,omitempty"`
	CrashReportPath string `json:"crash_report_path,omitempty"`
	Note            string `json:"note,omitempty"`
}

type VerifyOptions struct {
	RunSPCTL      bool
	RunLaunchTest bool
	LaunchTimeout time.Duration
}

// Verify runs structural + cryptographic checks on appPath. It never
// panics; caller should inspect Errors / Warnings for diagnostics. An
// error is returned only for out-of-band failures (e.g., source app
// cannot be inspected at all).
func Verify(ctx context.Context, appPath string, opts VerifyOptions, ex macos.Execer) (*VerifyReport, error) {
	report := &VerifyReport{AppPath: appPath}

	inspected, err := appinfo.Inspect(appPath)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("inspect failed: %v", err))
		return report, err
	}
	report.PlistValid = true
	if inspected.Executable != "" {
		report.ExecutableResolved = true
		report.ExecutablePath = inspected.Executable
	} else {
		report.Errors = append(report.Errors, "CFBundleExecutable does not resolve to a file under Contents/MacOS")
	}

	cs, err := macos.Verify(ctx, ex, appPath, true, true)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("codesign invocation failed: %v", err))
	} else {
		report.Codesign = cs
		if !cs.OK {
			report.Errors = append(report.Errors, "codesign verify failed: "+strings.TrimSpace(cs.Stderr))
		}
	}

	if opts.RunSPCTL {
		a, err := macos.Assess(ctx, ex, appPath)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("spctl invocation failed: %v", err))
		} else {
			report.SPCTL = a
			if !a.Accepted {
				report.Warnings = append(report.Warnings,
					"spctl rejected — expected for ad-hoc signed clones; local launch may still work")
			}
		}
	}

	if opts.RunLaunchTest {
		timeout := resolveLaunchTimeout(opts.LaunchTimeout)
		lt := macos.LaunchTest(ctx, ex, appPath, inspected.Identity.BundleID, timeout)
		report.LaunchTest = &LaunchTestResult{
			Attempted:       lt.Attempted,
			Launched:        lt.Launched,
			Survived:        lt.Survived,
			SurvivedMs:      lt.SurvivedMs,
			CrashSummary:    lt.CrashSummary,
			CrashReportPath: lt.CrashReportPath,
			Note:            lt.Note,
		}
		switch {
		case lt.Launched && !lt.Survived:
			msg := "launch test: process exited early"
			if lt.CrashSummary != "" {
				msg = "launch test: crashed — " + lt.CrashSummary
			}
			report.Errors = append(report.Errors, msg)
		case !lt.Launched:
			report.Warnings = append(report.Warnings, "launch test: `open` did not start the app: "+lt.Note)
		case lt.Survived:
			// happy path — nothing to add
		}
	}

	return report, nil
}

func resolveLaunchTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 10 * time.Second
	}
	return timeout
}
