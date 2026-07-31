package cli

import (
	"fmt"
	"scaffolder/internal/templater"

	"github.com/spf13/cobra"
)

// Define the global config specifically for CLI flags to write into.
var cfg templater.Config

var onboardTeamCmd = &cobra.Command{
	Use:   "onboard-team",
	Short: "Scaffolds the tenancy boundary for a new team",
	RunE: func(cmd *cobra.Command, args []string) error {
		// RunE is like Run, but allows us to return errors which Cobra will print nicely!
		fmt.Printf("Onboarding new team: %s\n", cfg.TeamName)

		// Call our rendering logic
		return templater.RenderTenantFoundation(cfg)
	},
}

func init() {
	// Attach this command to the root CLI
	rootCmd.AddCommand(onboardTeamCmd)

	// Define the flags (Long flag, short flag, default value, description)
	onboardTeamCmd.Flags().StringVarP(&cfg.TeamName, "team-name", "t", "", "Name of the tenant/team (Required)")

	// Force the user to provide this flag, otherwise Cobra throws an error
	onboardTeamCmd.MarkFlagRequired("team-name")
}
