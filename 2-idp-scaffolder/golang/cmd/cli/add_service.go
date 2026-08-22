package cli

import (
	"fmt"
	"scaffolder/internal/templater"

	"github.com/spf13/cobra"
)

// We need temporary variables to capture flags before merging them into `cfg`
var (
	goldenPathFlag string
)

var addServiceCmd = &cobra.Command{
	Use:   "add-service",
	Short: "Adds a microservice to a system (Runtime + Delivery + Infra)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.TenantName == "" {
			return fmt.Errorf("either --tenant-name or --team-name must be provided")
		}

		resolvedCfg, err := templater.Resolve(renderer.Spec, goldenPathFlag, cfg)
		if err != nil {
			return err
		}

		fmt.Printf("Generating app '%s' [Runtime: %s, Capabilities: %v]\n", resolvedCfg.AppName, resolvedCfg.Runtime, resolvedCfg.Capabilities)
		return renderer.RenderService(cmd.Context(), resolvedCfg)
	},
}

func init() {
	rootCmd.AddCommand(addServiceCmd)

	addServiceCmd.Flags().StringVarP(&cfg.TenantName, "tenant-name", "t", "", "Name of the tenant")
	
	// Deprecated team-name alias
	addServiceCmd.Flags().StringVar(&cfg.TenantName, "team-name", "", "Name of the tenant")
	addServiceCmd.Flags().MarkDeprecated("team-name", "use --tenant-name instead")
	addServiceCmd.Flags().MarkHidden("team-name")

	addServiceCmd.Flags().StringVarP(&cfg.AppName, "app-name", "a", "", "Name of the application")

	// Optional Backstage grouping. It is metadata only — it appears in catalog-info.yaml
	// and deliberately does NOT create a directory level.
	addServiceCmd.Flags().StringVarP(&cfg.SystemName, "system", "s", "", "Backstage system this service belongs to (metadata only)")

	// Flags for the Seed & Override logic
	addServiceCmd.Flags().StringVar(&goldenPathFlag, "golden-path", "", "Seed configuration from a named golden path")
	addServiceCmd.Flags().StringVar(&cfg.Runtime, "runtime", "", "Override the application runtime (e.g., go, python, nodejs, java-springboot)")
	addServiceCmd.Flags().StringSliceVar(&cfg.Capabilities, "capabilities", nil, "Comma-separated list of extra capabilities (e.g., postgres,s3)")
	addServiceCmd.Flags().StringVar(&cfg.Env, "env", "dev", "Target environment for scaffolding (e.g. dev, prod)")

	addServiceCmd.MarkFlagRequired("app-name")
	addServiceCmd.MarkFlagsOneRequired("golden-path", "runtime")
}
