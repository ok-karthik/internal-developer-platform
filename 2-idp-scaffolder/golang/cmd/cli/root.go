package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"scaffolder/internal/catalog"
	"scaffolder/internal/templater"
	"time"

	"github.com/briandowns/spinner"
	getter "github.com/hashicorp/go-getter"
	"github.com/spf13/cobra"
)

var (
	outputRoot  string
	catalogRoot string
	dryRun      bool

	// cfg holds the CLI flags (--team-name, --app-name, etc)
	cfg templater.Config
	// renderer holds the loaded catalog and the filesystem logic
	renderer *templater.Renderer
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:          "scaffolder",
	Short:        "Scaffolds templates for IDPs",
	Long:         `CLI tool that scaffolds application templates for IDPs. Created using GoLang Cobra CLI library.`,
	SilenceUsage: true,
	// PersistentPreRunE is inherited by all subcommands (e.g. onboard-team).
	// Unlike RunE (which runs command-specific logic), this runs BEFORE every subcommand
	// to execute shared setup: setting default output path and fetching/loading the catalog.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// 1. Where do we save the output?
		if outputRoot == "" {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			// If running from inside 2-idp-scaffolder/golang, step up to repo root
			if filepath.Base(wd) == "golang" || filepath.Base(wd) == "2-idp-scaffolder" {
				outputRoot = filepath.Clean(filepath.Join(wd, "../.."))
			} else {
				outputRoot = wd
			}
		}

		if catalogRoot == "" {
			// 2. Download the templates with a beautiful spinner!
			s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
			s.Suffix = " Fetching templates from GitHub..."
			s.Start()

			// TODO(platform): this should be a --catalog-ref flag defaulting to a
			// release tag stamped at build time via -ldflags, so a bad catalog
			// commit cannot break every engineer at once and a rollback is possible.
			// Tracked as Phase 4 in TODO.md. Until then, track the default branch —
			// never a feature branch, which disappears when it merges.
			var err error
			catalogRoot, err = fetchRemoteCatalog("main")
			if err != nil {
				s.Stop() // Make sure to stop it if there's an error!
				return err
			}

			s.Stop()
			fmt.Println("✅ Templates fetched!")
		} else {
			fmt.Printf("📁 Using local catalog from: %s\n", catalogRoot)
		}

		// 3. Load the spec (this assumes you will write catalog.Load later)
		spec, err := catalog.LoadCatalog(filepath.Join(catalogRoot, "catalog.yaml"))
		if err != nil {
			return err
		}

		// 4. Initialize the Renderer
		renderer = &templater.Renderer{
			CatalogFS: os.DirFS(catalogRoot),
			Spec:      spec,
			OutputDir: filepath.Join(outputRoot, "3-tenant-workloads"),
		}

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&outputRoot, "output-root", "", "path to 3-tenant-workloads (default: auto-discovered)")
	rootCmd.PersistentFlags().StringVar(&catalogRoot, "catalog-root", "", "path to 1-platform-catalog (default: auto-discovered)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "print what would be written without writing it")
}

// This downloads the catalog from GitHub into a temporary folder
func fetchRemoteCatalog(version string) (string, error) {
	// 1. Where to save it locally (e.g. ~/.scaffolder-cache/v1.0.0/)
	home, _ := os.UserHomeDir()
	cacheDir := filepath.Join(home, ".scaffolder-cache", version)
	// 2. The remote Git URL (you can even specify branches or tags using ?ref=)
	url := "git::https://github.com/ok-karthik/internal-developer-platform.git//1-platform-catalog?ref=" + version
	// 3. HashiCorp's go-getter handles the actual download
	err := getter.Get(cacheDir, url)
	if err != nil {
		return "", err
	}

	return cacheDir, nil
}
