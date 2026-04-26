package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/tt-a1i/doppel/internal/core/apperr"
	"github.com/tt-a1i/doppel/internal/core/clone"
	"github.com/tt-a1i/doppel/internal/core/doctor"
	"github.com/tt-a1i/doppel/internal/core/macos"
	"github.com/tt-a1i/doppel/internal/core/verify"
)

func newCloneCmd() *cobra.Command {
	var (
		name        string
		target      string
		bundleID    string
		displayName string
		dryRun      bool
		force       bool
		launchTest  bool
		skipDoctor  bool
	)
	cmd := &cobra.Command{
		Use:   "clone <app>",
		Short: "Clone a .app bundle",
		Args:  appArgValidator,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClone(args[0], cloneFlags{
				name:        name,
				target:      target,
				bundleID:    bundleID,
				displayName: displayName,
				dryRun:      dryRun,
				force:       force,
				launchTest:  launchTest,
				skipDoctor:  skipDoctor,
			})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "short name for the clone (derives target path if --target omitted)")
	cmd.Flags().StringVar(&target, "target", "", "explicit target .app path (default ~/Applications/<name>.app)")
	cmd.Flags().StringVar(&bundleID, "bundle-id", "", "new CFBundleIdentifier for the clone (default: source bundle ID + clone name)")
	cmd.Flags().StringVar(&displayName, "display-name", "", "CFBundleDisplayName for the clone (defaults to --name)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "derive plan and emit stages without touching disk")
	cmd.Flags().BoolVar(&force, "force", false, "if target already exists, remove it first (path must end in .app)")
	cmd.Flags().BoolVar(&launchTest, "launch-test", false, "briefly launch the clone after signing to confirm it survives (detects anti-tamper crashes)")
	cmd.Flags().BoolVar(&skipDoctor, "skip-doctor", false, "skip preflight diagnostics before cloning")
	return cmd
}

type cloneFlags struct {
	name, target, bundleID, displayName   string
	dryRun, force, launchTest, skipDoctor bool
}

type stageJSON struct {
	Stage   string `json:"stage"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type cloneJSON struct {
	Command           string                `json:"command"`
	Success           bool                  `json:"success"`
	SourceApp         string                `json:"source_app"`
	TargetApp         string                `json:"target_app"`
	BundleIDBefore    string                `json:"bundle_id_before"`
	BundleIDAfter     string                `json:"bundle_id_after"`
	DryRun            bool                  `json:"dry_run"`
	Stages            []stageJSON           `json:"stages"`
	HelperRewrites    []clone.HelperRewrite `json:"helper_rewrites,omitempty"`
	EntChanges        []string              `json:"entitlement_changes,omitempty"`
	Verify            *verify.VerifyReport  `json:"verify,omitempty"`
	PreflightFindings []doctor.Finding      `json:"preflight_findings,omitempty"`
	Warnings          []string              `json:"warnings,omitempty"`
	Error             string                `json:"error,omitempty"`
}

func runClone(appPath string, f cloneFlags) error {
	plan, err := clone.DerivePlan(clone.PlanOptions{
		SourceApp:   appPath,
		Name:        f.name,
		TargetApp:   f.target,
		BundleID:    f.bundleID,
		DisplayName: f.displayName,
		DryRun:      f.dryRun,
		Force:       f.force,
		LaunchTest:  f.launchTest,
	})
	if err != nil {
		return err
	}

	ctx := context.Background()
	ex := macos.NewExecer()

	preflightFindings, preflightErr := runClonePreflight(ctx, ex, plan.SourceApp, f.skipDoctor)
	if len(preflightFindings) > 0 && !flagJSON {
		printPreflightFindings(preflightFindings)
	}
	if preflightErr != nil {
		if flagJSON {
			out := cloneJSON{
				Command:           "clone",
				Success:           false,
				SourceApp:         plan.SourceApp,
				TargetApp:         plan.TargetApp,
				BundleIDBefore:    plan.BundleIDBefore,
				BundleIDAfter:     plan.BundleIDAfter,
				DryRun:            plan.DryRun,
				HelperRewrites:    plan.HelperRewrites,
				PreflightFindings: preflightFindings,
				Error:             preflightErr.Error(),
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(out)
		}
		return preflightErr
	}

	events := make(chan clone.StageEvent, 32)
	var wg sync.WaitGroup
	var collected []stageJSON

	wg.Add(1)
	go func() {
		defer wg.Done()
		for ev := range events {
			if flagJSON {
				collected = append(collected, stageJSON{
					Stage: ev.Stage, Status: string(ev.Status), Message: ev.Message,
				})
			} else {
				printStage(ev)
			}
		}
	}()

	result, runErr := clone.Run(ctx, plan, ex, events)
	close(events)
	wg.Wait()

	if flagJSON {
		out := cloneJSON{
			Command:           "clone",
			Success:           runErr == nil,
			SourceApp:         plan.SourceApp,
			TargetApp:         plan.TargetApp,
			BundleIDBefore:    plan.BundleIDBefore,
			BundleIDAfter:     plan.BundleIDAfter,
			DryRun:            plan.DryRun,
			Stages:            collected,
			HelperRewrites:    plan.HelperRewrites,
			PreflightFindings: preflightFindings,
		}
		if result != nil {
			out.EntChanges = result.EntChanges
			out.Verify = result.Verify
			out.Warnings = result.Warnings
		}
		if runErr != nil {
			out.Error = runErr.Error()
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return runErr
	}

	fmt.Println()
	if runErr != nil {
		fmt.Printf("FAILED: %v\n", runErr)
		return runErr
	}
	if plan.DryRun {
		fmt.Printf("DRY-RUN OK → would clone to %s\n", plan.TargetApp)
	} else {
		fmt.Printf("SUCCESS → %s\n", plan.TargetApp)
	}
	if result != nil && len(result.Warnings) > 0 {
		fmt.Println("Warnings:")
		for _, w := range result.Warnings {
			fmt.Printf("  • %s\n", w)
		}
	}
	if result != nil && result.Verify != nil && len(result.Verify.Warnings) > 0 {
		for _, w := range result.Verify.Warnings {
			fmt.Printf("  • %s\n", w)
		}
	}
	return nil
}

func runClonePreflight(ctx context.Context, ex macos.Execer, appPath string, skip bool) ([]doctor.Finding, error) {
	if skip {
		return nil, nil
	}
	findings, err := doctor.DiagnoseApp(ctx, ex, appPath)
	if err != nil {
		return nil, err
	}
	blocking := doctor.BlockingFindings(findings)
	if len(blocking) == 0 {
		return findings, nil
	}
	return findings, fmt.Errorf("%w: preflight blocked clone: %s", apperr.ErrInvalidInput, findingCodes(blocking))
}

func printPreflightFindings(findings []doctor.Finding) {
	fmt.Println("Preflight:")
	for _, f := range findings {
		fmt.Printf("  [%s] %s — %s\n", f.Severity, f.Code, f.Title)
	}
	fmt.Println()
}

func findingCodes(findings []doctor.Finding) string {
	codes := make([]string, 0, len(findings))
	for _, f := range findings {
		codes = append(codes, f.Code)
	}
	return strings.Join(codes, ", ")
}

func printStage(ev clone.StageEvent) {
	symbol := "•"
	switch ev.Status {
	case clone.StageStart:
		symbol = "→"
	case clone.StageOK:
		symbol = "✓"
	case clone.StageWarn:
		symbol = "⚠"
	case clone.StageError:
		symbol = "✗"
	case clone.StageSkip:
		symbol = "-"
	}
	fmt.Printf("  %s  %-14s %s\n", symbol, ev.Stage, ev.Message)
}
