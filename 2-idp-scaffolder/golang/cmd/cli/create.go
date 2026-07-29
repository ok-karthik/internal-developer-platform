package cli

import (
	"scaffolder/internal/templater"

	"github.com/spf13/cobra"
)

var cfg templater.Config

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create [appname]",
	Short: "Create application from templates",
	Long: `To scaffold a new microservice for a team
(including dedicated VPC CIDR block, Terraform code, microservice template, Helm charts, and CI/CD pipelines)`,
	Run: func(cmd *cobra.Command, args []string) {

		templater.RenderTemplates(cfg)
	},
}

func init() {
	rootCmd.AddCommand(createCmd)

	createCmd.Flags().StringVarP(&cfg.AppName, "app-name", "a", "", "Name of the application to be generated")
	createCmd.Flags().StringVarP(&cfg.AppType, "app-type", "t", "", "Type of the application to be generated")
	createCmd.Flags().IntVarP(&cfg.AppPort, "app-port", "p", 8080, "Port of the application to be generated")
	createCmd.Flags().StringVar(&cfg.TeamName, "team-name", "", "Team/Tenant name")
	createCmd.Flags().StringVar(&cfg.CloudServices, "cloud-services", "", "Cloud services to be enabled for the application")

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// createCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// createCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
