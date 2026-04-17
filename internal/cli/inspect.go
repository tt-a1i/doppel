package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tt-a1i/appclone/internal/core/appinfo"
	"github.com/tt-a1i/appclone/internal/core/doctor"
	"github.com/tt-a1i/appclone/internal/core/macos"
	"github.com/tt-a1i/appclone/internal/core/signing"
)

func newInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <app>",
		Short: "Inspect a .app bundle",
		Args:  appArgValidator,
		RunE:  runInspect,
	}
}

type inspectJSON struct {
	Command            string                `json:"command"`
	AppPath            string                `json:"app_path"`
	Identity           appinfo.AppIdentity   `json:"identity"`
	HasSignature       bool                  `json:"has_signature"`
	Executable         string                `json:"executable"`
	DetectedComponents componentsJSON        `json:"detected_components"`
	Findings           []doctor.Finding      `json:"findings,omitempty"`
}

type componentsJSON struct {
	Frameworks  []string `json:"frameworks"`
	Helpers     []string `json:"helpers"`
	XPCServices []string `json:"xpc_services"`
	Plugins     []string `json:"plugins"`
	LoginItems  []string `json:"login_items"`
}

func componentsFromItems(items []signing.SignableItem) componentsJSON {
	var c componentsJSON
	for _, it := range items {
		switch it.Kind {
		case signing.KindFramework:
			c.Frameworks = append(c.Frameworks, it.Path)
		case signing.KindHelperApp:
			c.Helpers = append(c.Helpers, it.Path)
		case signing.KindXPCService:
			c.XPCServices = append(c.XPCServices, it.Path)
		case signing.KindPlugin:
			c.Plugins = append(c.Plugins, it.Path)
		case signing.KindLoginItem:
			c.LoginItems = append(c.LoginItems, it.Path)
		}
	}
	return c
}

func runInspect(cmd *cobra.Command, args []string) error {
	appPath := args[0]
	ctx := context.Background()
	ex := macos.NewExecer()

	report, err := appinfo.Inspect(appPath)
	if err != nil {
		return err
	}
	items, _ := signing.Discover(appPath)
	findings, _ := doctor.DiagnoseApp(ctx, ex, appPath)

	if flagJSON {
		out := inspectJSON{
			Command:            "inspect",
			AppPath:            appPath,
			Identity:           report.Identity,
			HasSignature:       report.HasSignature,
			Executable:         report.Executable,
			DetectedComponents: componentsFromItems(items),
			Findings:           findings,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	fmt.Printf("App:          %s\n", appPath)
	fmt.Printf("Bundle ID:    %s\n", report.Identity.BundleID)
	fmt.Printf("Bundle Name:  %s\n", report.Identity.BundleName)
	fmt.Printf("Display Name: %s\n", report.Identity.DisplayName)
	fmt.Printf("Executable:   %s\n", report.Identity.ExecutableName)
	fmt.Printf("Version:      %s (build %s)\n", report.Identity.Version, report.Identity.Build)
	fmt.Printf("Signed:       %v\n", report.HasSignature)
	fmt.Println()

	counts := map[string]int{}
	for _, it := range items {
		if it.Kind != signing.KindMainBundle {
			counts[it.Kind.String()]++
		}
	}
	fmt.Println("Components:")
	if len(counts) == 0 {
		fmt.Println("  (none)")
	}
	for k, v := range counts {
		fmt.Printf("  %-12s %d\n", k, v)
	}

	if len(findings) > 0 {
		fmt.Println()
		fmt.Println("Doctor:")
		for _, f := range findings {
			fmt.Printf("  [%s] %s — %s\n", f.Severity, f.Code, f.Title)
		}
	}
	return nil
}
