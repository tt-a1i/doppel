package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tt-a1i/doppel/internal/core/doctor"
	"github.com/tt-a1i/doppel/internal/core/macos"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor <app>",
		Short: "Diagnose likely clone compatibility issues",
		Args:  appArgValidator,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(args[0])
		},
	}
}

type doctorJSON struct {
	Command  string                      `json:"command"`
	AppPath  string                      `json:"app_path"`
	Summary  doctor.CompatibilitySummary `json:"summary"`
	Findings []doctor.Finding            `json:"findings"`
}

func runDoctor(appPath string) error {
	ctx := context.Background()
	ex := macos.NewExecer()

	findings, err := doctor.DiagnoseApp(ctx, ex, appPath)
	if err != nil {
		return err
	}
	summary := doctor.SummarizeCompatibility(findings)

	if flagJSON {
		out := doctorJSON{Command: "doctor", AppPath: appPath, Summary: summary, Findings: findings}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	fmt.Printf("App: %s\n", appPath)
	fmt.Printf("Compatibility: %s — %s\n", summary.Level, summary.Title)
	fmt.Printf("Recommendation: %s\n", summary.Recommendation)
	if len(findings) == 0 {
		return nil
	}
	for _, f := range findings {
		fmt.Printf("\n[%s] %s (%s)\n", f.Severity, f.Title, f.Code)
		if len(f.Evidence) > 0 {
			fmt.Println("  Evidence:")
			for _, e := range f.Evidence {
				fmt.Printf("    • %s\n", e)
			}
		}
		if f.Fix != "" {
			fmt.Println("  Fix:")
			fmt.Printf("    %s\n", f.Fix)
		}
	}
	return nil
}
