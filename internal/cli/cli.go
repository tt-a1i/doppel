package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tt-a1i/appclone/internal/core/appinfo"
	"github.com/tt-a1i/appclone/internal/core/apperr"
	"github.com/tt-a1i/appclone/internal/core/exitcodes"
)

var rootCmd = &cobra.Command{
	Use:   "appclone",
	Short: "Clone a macOS .app bundle with a new identity",
	Long: "AppClone creates a second, locally-signed copy of a macOS app.\n" +
		"Run without arguments to launch the interactive TUI.",
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return appinfo.RequireMacOS()
	},
}

func appArgValidator(cmd *cobra.Command, args []string) error {
	if err := cobra.ExactArgs(1)(cmd, args); err != nil {
		return err
	}
	return appinfo.ValidateAppPath(args[0])
}

var inspectCmd = &cobra.Command{
	Use:   "inspect <app>",
	Short: "Inspect a .app bundle",
	Args:  appArgValidator,
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("inspect: not implemented yet")
	},
}

var cloneCmd = &cobra.Command{
	Use:   "clone <app>",
	Short: "Clone a .app bundle",
	Args:  appArgValidator,
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("clone: not implemented yet")
	},
}

var verifyCmd = &cobra.Command{
	Use:   "verify <app>",
	Short: "Verify a cloned .app",
	Args:  appArgValidator,
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("verify: not implemented yet")
	},
}

var doctorCmd = &cobra.Command{
	Use:   "doctor <app>",
	Short: "Diagnose issues with a cloned .app",
	Args:  appArgValidator,
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("doctor: not implemented yet")
	},
}

func init() {
	rootCmd.AddCommand(inspectCmd, cloneCmd, verifyCmd, doctorCmd)
}

// Execute runs the CLI and returns an exit code. Prints errors to stderr.
func Execute() int {
	err := rootCmd.Execute()
	if err == nil {
		return exitcodes.OK
	}
	fmt.Fprintln(os.Stderr, "Error:", err)
	return errToExitCode(err)
}

func errToExitCode(err error) int {
	switch {
	case errors.Is(err, apperr.ErrUnsupportedOS):
		return exitcodes.UnsupportedEnvironment
	case errors.Is(err, apperr.ErrAppMissing),
		errors.Is(err, apperr.ErrNotAnApp),
		errors.Is(err, apperr.ErrAppUnreadable),
		errors.Is(err, apperr.ErrInvalidInput),
		errors.Is(err, apperr.ErrTargetExists):
		return exitcodes.InvalidInput
	default:
		return exitcodes.GeneralError
	}
}
