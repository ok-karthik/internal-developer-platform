package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"scaffolder/internal/catalog"
	"scaffolder/internal/templater"
	"syscall"
	"time"

	"github.com/briandowns/spinner"
	getter "github.com/hashicorp/go-getter"
	"github.com/spf13/cobra"
)

var (
	outputRoot     string
	catalogRoot    string
	dryRun         bool
	force          bool
	catalogRefresh bool

	catalogRef = "main"

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
	Version:      catalogRef,
	SilenceUsage: true,
	// PersistentPreRunE runs shared setup before any subcommand:
	// resolving output directory, fetching remote/local catalog, and initializing the renderer.
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// 1. Default output directory to current working directory (cwd)
		if outputRoot == "" {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			outputRoot = wd
		}

		if catalogRoot == "" {
			// 2. Fetch templates from remote git repository if no local catalog is provided
			s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
			s.Suffix = " Fetching templates from GitHub..."
			s.Start()

			var err error
			catalogRoot, err = fetchRemoteCatalog(cmd.Context(), catalogRef)
			if err != nil {
				s.Stop()
				return err
			}

			s.Stop()
			fmt.Println("✅ Templates fetched!")
		} else {
			fmt.Printf("📁 Using local catalog from: %s\n", catalogRoot)
		}

		// 3. Load and validate catalog.yaml from the catalog filesystem
		catalogFS := os.DirFS(catalogRoot)
		spec, err := catalog.LoadCatalog(catalogFS)
		if err != nil {
			return err
		}

		var w templater.Writer = templater.OSWriter{}
		if dryRun {
			w = templater.DryRunWriter{}
		}

		// 4. Initialize the Renderer
		renderer = &templater.Renderer{
			CatalogFS: catalogFS,
			Spec:      spec,
			OutputDir: outputRoot,
			Writer:    w,
			Force:     force,
		}

		return nil
	},
}

// Execute configures root signal handling and runs the CLI command.
func Execute() {
	// Listen for Ctrl+C (SIGINT) and SIGTERM for graceful cancellation
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			os.Exit(130) // Standard Unix exit code for SIGINT (128 + 2)
		}
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&outputRoot, "output-root", "", "directory to write scaffolded files into (default: current directory)")
	rootCmd.PersistentFlags().StringVar(&catalogRoot, "catalog-root", "", "path to 1-platform-catalog (default: auto-discovered)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "print what would be written without writing it")
	rootCmd.PersistentFlags().BoolVar(&force, "force", false, "force overwrite of existing files")
	rootCmd.PersistentFlags().StringVar(&catalogRef, "catalog-ref", catalogRef, "catalog git ref (tag, branch, or SHA)")
	rootCmd.PersistentFlags().BoolVar(&catalogRefresh, "catalog-refresh", false, "force re-download of cached catalog")
}

// fetchRemoteCatalog downloads the catalog from GitHub into ~/.scaffolder-cache/<ref>.
func fetchRemoteCatalog(ctx context.Context, version string) (string, error) {
	// 1. Locate cache directory in user home
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate home directory for the catalog cache: %w", err)
	}
	cacheDir := filepath.Join(home, ".scaffolder-cache", version)

	// 2. Clear cache if refresh is explicitly requested
	if catalogRefresh {
		if err := os.RemoveAll(cacheDir); err != nil {
			return "", fmt.Errorf("clearing catalog cache: %w", err)
		}
	}

	// 3. Download catalog using go-getter client with context cancellation
	url := "git::https://github.com/ok-karthik/internal-developer-platform.git//1-platform-catalog?ref=" + version

	client := &getter.Client{
		Ctx:  ctx,
		Src:  url,
		Dst:  cacheDir,
		Mode: getter.ClientModeAny,
	}
	if err := client.Get(); err != nil {
		return "", fmt.Errorf("fetching catalog %s: %w", version, err)
	}

	return cacheDir, nil
}
