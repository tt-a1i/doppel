package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/spf13/cobra"

	"github.com/tt-a1i/appclone/internal/core/clone"
	"github.com/tt-a1i/appclone/internal/core/macos"
	"github.com/tt-a1i/appclone/internal/core/verify"
)

func newCloneCmd() *cobra.Command {
	var (
		name        string
		target      string
		bundleID    string
		displayName string
		dryRun      bool
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
			})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "short name for the clone (derives target path if --target omitted)")
	cmd.Flags().StringVar(&target, "target", "", "explicit target .app path (default /Applications/<name>.app)")
	cmd.Flags().StringVar(&bundleID, "bundle-id", "", "new CFBundleIdentifier for the clone (required)")
	cmd.Flags().StringVar(&displayName, "display-name", "", "CFBundleDisplayName for the clone (defaults to --name)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "derive plan and emit stages without touching disk")
	return cmd
}

type cloneFlags struct {
	name, target, bundleID, displayName string
	dryRun                              bool
}

type stageJSON struct {
	Stage   string `json:"stage"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type cloneJSON struct {
	Command          string                `json:"command"`
	Success          bool                  `json:"success"`
	SourceApp        string                `json:"source_app"`
	TargetApp        string                `json:"target_app"`
	BundleIDBefore   string                `json:"bundle_id_before"`
	BundleIDAfter    string                `json:"bundle_id_after"`
	DryRun           bool                  `json:"dry_run"`
	Stages           []stageJSON           `json:"stages"`
	HelperRewrites   []clone.HelperRewrite `json:"helper_rewrites,omitempty"`
	EntChanges       []string              `json:"entitlement_changes,omitempty"`
	Verify           *verify.VerifyReport  `json:"verify,omitempty"`
	Warnings         []string              `json:"warnings,omitempty"`
	Error            string                `json:"error,omitempty"`
}

func runClone(appPath string, f cloneFlags) error {
	plan, err := clone.DerivePlan(clone.PlanOptions{
		SourceApp:   appPath,
		Name:        f.name,
		TargetApp:   f.target,
		BundleID:    f.bundleID,
		DisplayName: f.displayName,
		DryRun:      f.dryRun,
	})
	if err != nil {
		return err
	}

	ctx := context.Background()
	ex := macos.NewExecer()

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
			Command:        "clone",
			Success:        runErr == nil,
			SourceApp:      plan.SourceApp,
			TargetApp:      plan.TargetApp,
			BundleIDBefore: plan.BundleIDBefore,
			BundleIDAfter:  plan.BundleIDAfter,
			DryRun:         plan.DryRun,
			Stages:         collected,
			HelperRewrites: plan.HelperRewrites,
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
