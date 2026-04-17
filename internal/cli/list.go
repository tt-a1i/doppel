package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tt-a1i/appclone/internal/core/appinfo"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed .app bundles in standard locations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			reports, err := appinfo.ListInstalled(nil)
			if err != nil {
				return err
			}
			if flagJSON {
				type row struct {
					Path         string `json:"path"`
					BundleID     string `json:"bundle_id"`
					Name         string `json:"name"`
					DisplayName  string `json:"display_name"`
					Version      string `json:"version"`
					Build        string `json:"build"`
					HasSignature bool   `json:"has_signature"`
				}
				rows := make([]row, 0, len(reports))
				for _, r := range reports {
					rows = append(rows, row{
						Path:         r.Identity.AppPath,
						BundleID:     r.Identity.BundleID,
						Name:         r.Identity.BundleName,
						DisplayName:  r.Identity.DisplayName,
						Version:      r.Identity.Version,
						Build:        r.Identity.Build,
						HasSignature: r.HasSignature,
					})
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"command": "list",
					"count":   len(rows),
					"apps":    rows,
				})
			}
			for _, r := range reports {
				name := r.Identity.DisplayName
				if name == "" {
					name = r.Identity.BundleName
				}
				sig := "unsigned"
				if r.HasSignature {
					sig = "signed"
				}
				fmt.Printf("%-40s  %-8s  %-50s  %s\n", truncate(name, 40), sig, truncate(r.Identity.BundleID, 50), r.Identity.AppPath)
			}
			return nil
		},
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n < 3 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
