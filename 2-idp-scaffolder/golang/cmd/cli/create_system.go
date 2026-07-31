package cli

import (
	"fmt"
	"scaffolder/internal/templater"

	"github.com/spf13/cobra"
)

var createSystemCmd = &cobra.Command{
	Use:   "create-system",
	Short: "Creates a new logical system grouping and ArgoCD ApplicationSet",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Creating system %s for team %s\n", cfg.SystemName, cfg.TeamName)
		return templater.RenderSystem(cfg)
	},
}

func init() {
	rootCmd.AddCommand(createSystemCmd)

	createSystemCmd.Flags().StringVarP(&cfg.TeamName, "team-name", "t", "", "Name of the tenant/team")
	createSystemCmd.Flags().StringVarP(&cfg.SystemName, "system-name", "s", "", "Name of the logical system")

	createSystemCmd.MarkFlagRequired("team-name")
	createSystemCmd.MarkFlagRequired("system-name")
}
