package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "appclone",
	Short: "Clone a macOS .app bundle with a new identity",
	Long: "AppClone creates a second, locally-signed copy of a macOS app.\n" +
		"Run without arguments to launch the interactive TUI.",
}

var inspectCmd = &cobra.Command{
	Use:   "inspect <app>",
	Short: "Inspect a .app bundle",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("inspect: not implemented yet")
	},
}

var cloneCmd = &cobra.Command{
	Use:   "clone <app>",
	Short: "Clone a .app bundle",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("clone: not implemented yet")
	},
}

var verifyCmd = &cobra.Command{
	Use:   "verify <app>",
	Short: "Verify a cloned .app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("verify: not implemented yet")
	},
}

var doctorCmd = &cobra.Command{
	Use:   "doctor <app>",
	Short: "Diagnose issues with a cloned .app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("doctor: not implemented yet")
	},
}

func init() {
	rootCmd.AddCommand(inspectCmd, cloneCmd, verifyCmd, doctorCmd)
}

func Execute() error {
	return rootCmd.Execute()
}
