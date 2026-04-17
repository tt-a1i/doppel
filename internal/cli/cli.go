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

var (
	flagJSON    bool
	flagVerbose bool
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

func init() {
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "emit structured JSON output")
	rootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "emit extra detail")

	rootCmd.AddCommand(
		newListCmd(),
		newInspectCmd(),
		newCloneCmd(),
		newVerifyCmd(),
		newDoctorCmd(),
	)
}

func appArgValidator(cmd *cobra.Command, args []string) error {
	if err := cobra.ExactArgs(1)(cmd, args); err != nil {
		return err
	}
	return appinfo.ValidateAppPath(args[0])
}

// Execute runs the CLI and returns an exit code. It prints any top-level
// error to stderr (Cobra usage is silenced; we render ourselves).
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
