# 🚀 Complete Golang CLI Implementation Tutorial

This guide is designed for a Go beginner to incrementally build a production-grade Platform Engineering CLI. It covers how to replace a single rigid command with a multi-verb architecture (Cobra), how to parse YAML, and how to safely render `text/template` files to the correct output directories.

You can use this as your definitive blueprint when coding along!

---

## Step 1: Install Required Dependencies

Before writing code, we need a YAML parser to read `golden-paths.yaml`. Go doesn't have a built-in YAML parser in the standard library.

Run this in your terminal from the `2-idp-scaffolder-golang/` directory:
```bash
go get gopkg.in/yaml.v3
```

---

## Step 2: Update the Global Configuration Struct

**File:** `internal/templater/render.go`

In Go, templates use a struct to fill in variables (e.g., `[[ .TeamName ]]` maps to the `TeamName` field). We need to update this struct to support our new design decisions.

```go
package templater

// Config holds all the data that our templates might need to render.
// Note: Fields MUST be capitalized so they are exported and accessible by the text/template engine!
type Config struct {
	TeamName     string   // e.g., "payments"
	SystemName   string   // e.g., "checkout"
	AppName      string   // e.g., "checkout-api"
	Runtime      string   // e.g., "go" (From golden-path)
	Capabilities []string // e.g., ["postgres", "s3"] (From flags or golden-path)
}
```

---

## Step 3: Parse the Golden Paths (Seed Model)

**File:** `internal/catalog/goldenpath.go`

We want a robust way to read `1-idp-scaffolder-templates/golden-paths.yaml`. 
Create a new package `internal/catalog` and add the following code:

```go
package catalog

import (
	"os"
	"gopkg.in/yaml.v3"
)

// GoldenPath represents a single paved-road configuration.
// The `yaml:"..."` tags tell the parser which YAML key maps to which field.
type GoldenPath struct {
	Name         string   `yaml:"name"`
	Runtime      string   `yaml:"runtime"`
	Capabilities []string `yaml:"capabilities"`
}

// Catalog represents the top-level structure of the YAML file.
type Catalog struct {
	GoldenPaths []GoldenPath `yaml:"golden-paths"`
}

// LoadCatalog reads the YAML file from disk and parses it into our Structs.
func LoadCatalog(filePath string) (*Catalog, error) {
	// 1. Read the raw bytes from the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err // Return the error if file doesn't exist
	}

	var catalog Catalog
	// 2. Unmarshal converts the raw YAML bytes into our Go struct
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, err
	}

	return &catalog, nil
}

// FindPath is a helper method to easily look up a specific golden path by name.
func (c *Catalog) FindPath(name string) (*GoldenPath, bool) {
	for _, p := range c.GoldenPaths {
		if p.Name == name {
			return &p, true // Found it!
		}
	}
	return nil, false // Not found
}
```

---

## Step 4: Delete the Old CLI Command

**Delete:** `cmd/cli/create.go`
We are moving from a single `scaffolder create` command to a verb-driven architecture (`onboard-team`, `create-system`, `add-service`).

---

## Step 5: Implement `onboard-team`

**File:** `cmd/cli/onboard_team.go`

This command runs exactly once per team and sets up their foundation.

```go
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
		
		// Call our rendering logic (we will build this in Step 8)
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
```

---

## Step 6: Implement `create-system`

**File:** `cmd/cli/create_system.go`

This command runs once per logical system to set up ArgoCD ApplicationSets.

```go
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
```

---

## Step 7: Implement `add-service` (The Complex One!)

**File:** `cmd/cli/add_service.go`

This command merges our Seed (Golden Path) and Override (Explicit flags) logic.

```go
package cli

import (
	"fmt"
	"strings"
	"scaffolder/internal/catalog"
	"scaffolder/internal/templater"
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
		
		// 1. If a golden path is provided, seed the configuration
		if goldenPathFlag != "" {
			cat, err := catalog.LoadCatalog("../1-idp-scaffolder-templates/golden-paths.yaml")
			if err != nil {
				return fmt.Errorf("failed to load catalog: %w", err)
			}
			
			gp, found := cat.FindPath(goldenPathFlag)
			if !found {
				return fmt.Errorf("golden path '%s' not found in catalog", goldenPathFlag)
			}
			
			// Seed the config
			cfg.Runtime = gp.Runtime
			cfg.Capabilities = gp.Capabilities
		}

		// 2. If the user passed explicit capabilities, override/extend the list
		if capabilitiesString != "" {
			// Split the comma-separated string into a slice: "postgres,s3" -> ["postgres", "s3"]
			extraCaps := strings.Split(capabilitiesString, ",")
			cfg.Capabilities = append(cfg.Capabilities, extraCaps...)
		}

		// (Optional for learning): You might want to deduplicate cfg.Capabilities here 
		// so if a user asks for 'postgres' twice, it only renders once!

		fmt.Printf("Generating app '%s' [Runtime: %s, Capabilities: %v]\n", cfg.AppName, cfg.Runtime, cfg.Capabilities)
		return templater.RenderService(cfg)
	},
}

func init() {
	rootCmd.AddCommand(addServiceCmd)

	addServiceCmd.Flags().StringVarP(&cfg.TeamName, "team-name", "t", "", "Name of the tenant/team")
	addServiceCmd.Flags().StringVarP(&cfg.SystemName, "system-name", "s", "", "Name of the logical system")
	addServiceCmd.Flags().StringVarP(&cfg.AppName, "app-name", "a", "", "Name of the application")
	
	// Flags for the Seed & Override logic
	addServiceCmd.Flags().StringVar(&goldenPathFlag, "golden-path", "", "Seed configuration from a named golden path")
	addServiceCmd.Flags().StringVar(&cfg.Runtime, "runtime", "", "Override the application runtime (e.g., go, python)")
	addServiceCmd.Flags().StringVar(&capabilitiesString, "capabilities", "", "Comma-separated list of extra capabilities (e.g., postgres,s3)")

	addServiceCmd.MarkFlagRequired("team-name")
	addServiceCmd.MarkFlagRequired("system-name")
	addServiceCmd.MarkFlagRequired("app-name")
}
```

---

## Step 8: Writing the Rendering Engine Logic

**File:** `internal/templater/render.go`

Now we write the functions that do the actual file generation. Remember our rule: **GitOps and Infra both scaffold exclusively into `dev/`!**

```go
package templater

import (
	"fmt"
	"os"
	"path/filepath"
	// ... (include strings, text/template, fs)
)

// RenderTenantFoundation scaffolds 1-idp-scaffolder-templates/tenant-foundation/
func RenderTenantFoundation(cfg Config) error {
	sourceDir := filepath.Join("..", "1-idp-scaffolder-templates", "tenant-foundation")
	// Target: 3-tenant-workloads/<team>/gitops-repo/tenant-foundation/
	targetDir := filepath.Join("..", "3-tenant-workloads", cfg.TeamName, "gitops-repo", "tenant-foundation")
	
	// walkAndRender is a helper function you'll write that uses filepath.WalkDir
	return walkAndRender(sourceDir, targetDir, cfg)
}

// RenderService is the complex rendering logic for `add-service`
func RenderService(cfg Config) error {
	
	// --- 1. RENDER RUNTIME ---
	if cfg.Runtime != "" {
		runtimeSrc := filepath.Join("..", "1-idp-scaffolder-templates", "components", "runtimes", cfg.Runtime)
		runtimeTarget := filepath.Join("..", "3-tenant-workloads", cfg.TeamName, "apps-source", "systems", cfg.SystemName, cfg.AppName)
		if err := walkAndRender(runtimeSrc, runtimeTarget, cfg); err != nil {
			return err
		}
	}

	// --- 2. RENDER DELIVERY (dev/ only) ---
	deliverySrc := filepath.Join("..", "1-idp-scaffolder-templates", "components", "delivery")
	deliveryTarget := filepath.Join("..", "3-tenant-workloads", cfg.TeamName, "gitops-repo", "systems", cfg.SystemName, cfg.AppName, "dev")
	if err := walkAndRender(deliverySrc, deliveryTarget, cfg); err != nil {
		return err
	}

	// --- 3. RENDER INFRA CAPABILITIES (dev/ only) ---
	infraTargetDir := filepath.Join("..", "3-tenant-workloads", cfg.TeamName, "infra-repo", "systems", cfg.SystemName, cfg.AppName, "dev")
	
	// Create the infra target directory first
	os.MkdirAll(infraTargetDir, 0755)

	// Loop through requested capabilities
	for _, cap := range cfg.Capabilities {
		srcFile := filepath.Join("..", "1-idp-scaffolder-templates", "components", "infra", cap+".tf.tmpl")
		destFile := filepath.Join(infraTargetDir, cap+".tf")
		
		fmt.Printf("Adding infrastructure capability: %s\n", cap)
		if err := processSingleTemplate(srcFile, destFile, cfg); err != nil {
			return fmt.Errorf("failed rendering capability %s: %w", cap, err)
		}
	}
	
	return nil
}

// processSingleTemplate reads a .tmpl file, evaluates variables, and writes it.
func processSingleTemplate(srcPath string, targetPath string, cfg Config) error {
	// Strip the .tmpl extension from the final file name
	targetPath = strings.TrimSuffix(targetPath, ".tmpl")

	// Ensure the parent directory exists before creating the file
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return err
	}

	outFile, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	// Parse the template and define our custom [[ ]] delimiters
	tmpl, err := template.New(filepath.Base(srcPath)).Delims("[[", "]]").ParseFiles(srcPath)
	if err != nil {
		return err
	}

	// Execute it!
	return tmpl.Execute(outFile, cfg)
}
```

### Pro-Tip for Go Beginners
If you want to implement the `walkAndRender(sourceDir, targetDir, cfg)` function yourself, use `filepath.WalkDir`. It allows you to recurse through a directory. For every file it finds, compute the relative path from the `sourceDir`, append it to the `targetDir`, and call `processSingleTemplate`!

---

## 🌟 Extra Credit: Interactive UI (Charmbracelet)

Once you have the core Cobra flags and templating engine working, you can make the CLI feel like a premium, Principal-level tool by adding **Interactive Prompts**. 

Instead of forcing users to type long flags like `--golden-path go-service-postgres`, you can use [Charmbracelet's Huh](https://github.com/charmbracelet/huh) to create a beautiful terminal form, and [Lip Gloss](https://github.com/charmbracelet/lipgloss) to style the success output.

### Example: Adding an Interactive Form to `add-service`

**File:** `cmd/cli/add_service.go`

```bash
go get github.com/charmbracelet/huh
go get github.com/charmbracelet/lipgloss
```

```go
// Inside your RunE function, check if the user omitted the flags:
if cfg.AppName == "" {
	// Create a beautiful interactive form
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("What is the application name?").
				Value(&cfg.AppName),
				
			huh.NewSelect[string]().
				Title("Choose a Golden Path").
				Options(
					huh.NewOption("Go API with Postgres", "go-service-postgres"),
					huh.NewOption("Python API", "python-api"),
				).
				Value(&goldenPathFlag),
		),
	)

	// Run the interactive form
	if err := form.Run(); err != nil {
		return err
	}
}
```

**Recommendation:** Do not try to build the interactive UI at the same time as the templating logic! Build the CLI using standard Cobra flags first (Steps 1-8). Once it successfully writes files to disk, come back and replace the missing flags with a `huh` form for a massive UX upgrade!
