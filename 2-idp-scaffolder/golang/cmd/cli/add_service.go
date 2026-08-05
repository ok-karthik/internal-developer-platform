package cli

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"
)

// We need temporary variables to capture flags before merging them into `cfg`
var (
	goldenPathFlag     string
	capabilitiesString []string
)

var addServiceCmd = &cobra.Command{
	Use:   "add-service",
	Short: "Adds a microservice to a system (Runtime + Delivery + Infra)",
	RunE: func(cmd *cobra.Command, args []string) error {

		// Seed from the golden path; explicit flags layer on top.
		if goldenPathFlag != "" {
			gp, found := renderer.Spec.FindGoldenPath(goldenPathFlag)
			if !found {
				return fmt.Errorf("golden path '%s' not found in catalog", goldenPathFlag)
			}
			// An explicit --runtime wins over the golden path's.
			if cfg.Runtime == "" {
				cfg.Runtime = gp.Runtime
			}
			cfg.Capabilities = append(cfg.Capabilities, gp.Capabilities...)
		}

		if cfg.Runtime == "" {
			return fmt.Errorf("a valid runtime or --golden-path must be specified (e.g. --golden-path go-microservice OR --runtime go)")
		}

		// Union of golden-path and user capabilities. Sort makes output deterministic;
		// Compact then drops the duplicates Sort has made adjacent.
		cfg.Capabilities = append(cfg.Capabilities, capabilitiesString...)
		slices.Sort(cfg.Capabilities)
		cfg.Capabilities = slices.Compact(cfg.Capabilities)

		fmt.Printf("Generating app '%s' [Runtime: %s, Capabilities: %v]\n", cfg.AppName, cfg.Runtime, cfg.Capabilities)
		return renderer.RenderService(cfg)
	},
}

func init() {
	rootCmd.AddCommand(addServiceCmd)

	addServiceCmd.Flags().StringVarP(&cfg.TeamName, "team-name", "t", "", "Name of the tenant/team")
	addServiceCmd.Flags().StringVarP(&cfg.AppName, "app-name", "a", "", "Name of the application")

	// Optional Backstage grouping. It is metadata only — it appears in catalog-info.yaml
	// and deliberately does NOT create a directory level.
	addServiceCmd.Flags().StringVarP(&cfg.SystemName, "system", "s", "", "Backstage system this service belongs to (metadata only)")

	// Flags for the Seed & Override logic
	addServiceCmd.Flags().StringVar(&goldenPathFlag, "golden-path", "", "Seed configuration from a named golden path")
	addServiceCmd.Flags().StringVar(&cfg.Runtime, "runtime", "", "Override the application runtime (e.g., golang, python)")
	addServiceCmd.Flags().StringSliceVar(&capabilitiesString, "capabilities", nil, "Comma-separated list of extra capabilities (e.g., postgres,s3)")
	addServiceCmd.Flags().StringVar(&cfg.Env, "env", "dev", "Target environment for scaffolding (e.g. dev, prod)")

	addServiceCmd.MarkFlagRequired("team-name")
	addServiceCmd.MarkFlagRequired("app-name")
}
