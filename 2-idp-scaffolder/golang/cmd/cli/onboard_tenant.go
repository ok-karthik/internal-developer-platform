package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Define the global config specifically for CLI flags to write into.
//var cfg templater.Config

var onboardTenantCmd = &cobra.Command{
	Use:   "onboard-tenant",
	Short: "Scaffolds the tenancy boundary for a new tenant",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cfg.TenantName == "" {
			return fmt.Errorf("either --tenant-name or --team-name must be provided")
		}
		// RunE is like Run, but allows us to return errors which Cobra will print nicely!
		fmt.Printf("Onboarding new tenant: %s\n", cfg.TenantName)

		// Call our rendering logic
		return renderer.RenderTenantFoundation(cmd.Context(), cfg)
	},
}

func init() {
	// Attach this command to the root CLI
	rootCmd.AddCommand(onboardTenantCmd)

	// Define the flags (Long flag, short flag, default value, description)
	onboardTenantCmd.Flags().StringVarP(&cfg.TenantName, "tenant-name", "t", "", "Name of the tenant (Required)")
	
	// Deprecated team-name alias
	onboardTenantCmd.Flags().StringVar(&cfg.TenantName, "team-name", "", "Name of the tenant")
	onboardTenantCmd.Flags().MarkDeprecated("team-name", "use --tenant-name instead")
	onboardTenantCmd.Flags().MarkHidden("team-name")

	// Owners flag
	onboardTenantCmd.Flags().StringSliceVar(&cfg.Owners, "owner", []string{}, "Owners of the tenant (can be specified multiple times)")

	// Force the user to provide this flag, otherwise Cobra throws an error
	// Note: since both flags map to cfg.TenantName, we just require the new one.
	// We handle if someone only provided team-name in a pre-run hook or we just require tenant-name for now.
	// Cobra requires the actual flag name passed to MarkFlagRequired.
	// Actually, requiring tenant-name makes team-name fail if tenant-name isn't also passed. 
	// A better way is to check manually in PreRunE, but for simplicity we will just remove MarkFlagRequired 
	// and check inside RunE.
}
