package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tt-a1i/doppel/internal/core/exitcodes"
	"github.com/tt-a1i/doppel/internal/core/macos"
	"github.com/tt-a1i/doppel/internal/core/verify"
)

func newVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify <app>",
		Short: "Verify a cloned .app",
		Args:  appArgValidator,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVerify(args[0])
		},
	}
}

type verifyJSON struct {
	Command string               `json:"command"`
	Success bool                 `json:"success"`
	Report  *verify.VerifyReport `json:"report"`
}

func runVerify(appPath string) error {
	ctx := context.Background()
	ex := macos.NewExecer()

	report, err := verify.Verify(ctx, appPath, verify.VerifyOptions{RunSPCTL: true}, ex)
	if err != nil {
		return err
	}

	success := len(report.Errors) == 0

	if flagJSON {
		out := verifyJSON{Command: "verify", Success: success, Report: report}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
	} else {
		fmt.Printf("App:        %s\n", report.AppPath)
		fmt.Printf("Plist OK:   %v\n", report.PlistValid)
		fmt.Printf("Executable: %s\n", report.ExecutablePath)
		if report.Codesign != nil {
			fmt.Printf("codesign:   ok=%v deep=%v strict=%v\n", report.Codesign.OK, report.Codesign.Deep, report.Codesign.Strict)
		}
		if report.SPCTL != nil {
			fmt.Printf("spctl:      accepted=%v\n", report.SPCTL.Accepted)
		}
		if len(report.Warnings) > 0 {
			fmt.Println("Warnings:")
			for _, w := range report.Warnings {
				fmt.Printf("  • %s\n", w)
			}
		}
		if len(report.Errors) > 0 {
			fmt.Println("Errors:")
			for _, e := range report.Errors {
				fmt.Printf("  ✗ %s\n", e)
			}
		}
	}

	if !success {
		os.Exit(exitcodes.VerificationFailed)
	}
	return nil
}
