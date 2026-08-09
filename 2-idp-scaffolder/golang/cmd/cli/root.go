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
		// 1. Where do we save the output? The current directory, full stop — the
		// same contract as terraform, npm, and `kubectl apply -f .`.
		//
		// A client running this binary is standing in their own checkout, so any
		// cleverer default is a guess about THIS repo's layout that is simply wrong
		// on their machine: matching a directory named "golang" silently scattered
		// output into cmd/cli/ or internal/templater/, and walking up to a .git
		// marker fails on a tarball or Docker layer while answering the wrong
		// question anyway. `--output-root` covers everything else.
		//
		// The demo flow that relied on that guess now lives in the Makefile, which
		// passes --output-root "$(git rev-parse --show-toplevel)/3-tenant-workloads"
		// — git's own root-finder, and not something this binary should reimplement.
		//
		// Note there is no "3-tenant-workloads" segment appended below either. That
		// directory is this repo's simulation wrapper, and splicing it into every
		// caller's path meant a client scaffolding into <org>/payments-apps got a
		// folder they never asked for. The binary writes where it is told; naming
		// the destination is the caller's job.
		if outputRoot == "" {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			outputRoot = wd
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

		// 3. Load the spec. One fs.FS serves both the load and the render, so
		// validation checks declarations against the very tree we will walk.
		catalogFS := os.DirFS(catalogRoot)
		spec, err := catalog.LoadCatalog(catalogFS)
		if err != nil {
			return err
		}

		// 4. Initialize the Renderer
		renderer = &templater.Renderer{
			CatalogFS: catalogFS,
			Spec:      spec,
			OutputDir: outputRoot,
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

	rootCmd.PersistentFlags().StringVar(&outputRoot, "output-root", "", "directory to write scaffolded files into (default: current directory)")
	rootCmd.PersistentFlags().StringVar(&catalogRoot, "catalog-root", "", "path to 1-platform-catalog (default: auto-discovered)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "print what would be written without writing it")
}

// This downloads the catalog from GitHub into a temporary folder
func fetchRemoteCatalog(version string) (string, error) {
	// 1. Where to save it locally (e.g. ~/.scaffolder-cache/v1.0.0/)
	//
	// Discarding this error used to leave home as "", which makes cacheDir a
	// RELATIVE .scaffolder-cache/<ref> — silently written into whatever
	// directory the user happened to run from, and never found again.
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate home directory for the catalog cache: %w", err)
	}
	cacheDir := filepath.Join(home, ".scaffolder-cache", version)
	// 2. The remote Git URL (you can even specify branches or tags using ?ref=)
	url := "git::https://github.com/ok-karthik/internal-developer-platform.git//1-platform-catalog?ref=" + version
	// 3. HashiCorp's go-getter handles the actual download
	if err := getter.Get(cacheDir, url); err != nil {
		return "", fmt.Errorf("fetching catalog %s: %w", version, err)
	}

	return cacheDir, nil
}
