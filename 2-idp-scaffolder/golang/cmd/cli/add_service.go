package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// We need temporary variables to capture flags before merging them into `cfg`
var (
	goldenPathFlag     string
	capabilitiesString string
)

var addServiceCmd = &cobra.Command{
	Use:   "add-service",
	Short: "Adds a microservice to a system (Runtime + Delivery + Infra)",
	RunE: func(cmd *cobra.Command, args []string) error {

		// If a golden path is provided, seed the configuration
		if goldenPathFlag != "" {
			gp, found := renderer.Spec.FindGoldenPath(goldenPathFlag)
			if !found {
				return fmt.Errorf("golden path '%s' not found in catalog", goldenPathFlag)
			}

			// Seed the config
			cfg.Runtime = gp.Runtime
			cfg.Capabilities = gp.Capabilities
		}

		// Ensure we have a runtime either from --golden-path or --runtime override
		if cfg.Runtime == "" {
			return fmt.Errorf("a valid runtime or --golden-path must be specified (e.g. --golden-path go-microservice OR --runtime go)")
		}

		// If the user passed explicit capabilities, override/extend the list
		if capabilitiesString != "" {
			// Split the comma-separated string into a slice: "postgres,s3" -> ["postgres", "s3"]
			extraCaps := strings.Split(capabilitiesString, ",")
			cfg.Capabilities = append(cfg.Capabilities, extraCaps...)
		}

		// (Optional for learning): You might want to deduplicate cfg.Capabilities here
		// so if a user asks for 'postgres' twice, it only renders once!

		fmt.Printf("Generating app '%s' [Runtime: %s, Capabilities: %v]\n", cfg.AppName, cfg.Runtime, cfg.Capabilities)
		return renderer.RenderService(cfg)
	},
}

func init() {
	rootCmd.AddCommand(addServiceCmd)

	addServiceCmd.Flags().StringVarP(&cfg.TeamName, "team-name", "t", "", "Name of the tenant/team")
	addServiceCmd.Flags().StringVarP(&cfg.SystemName, "system-name", "s", "", "Name of the logical system")
	addServiceCmd.Flags().StringVarP(&cfg.AppName, "app-name", "a", "", "Name of the application")

	// Flags for the Seed & Override logic
	addServiceCmd.Flags().StringVar(&goldenPathFlag, "golden-path", "", "Seed configuration from a named golden path")
	addServiceCmd.Flags().StringVar(&cfg.Runtime, "runtime", "", "Override the application runtime (e.g., golang, python)")
	addServiceCmd.Flags().StringVar(&capabilitiesString, "capabilities", "", "Comma-separated list of extra capabilities (e.g., postgres,s3)")

	addServiceCmd.MarkFlagRequired("team-name")
	addServiceCmd.MarkFlagRequired("system-name")
	addServiceCmd.MarkFlagRequired("app-name")
}
