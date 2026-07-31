package templater

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"scaffolder/internal/catalog"
	"strings"
	"text/template"
)

// Config holds all the data that our templates might need to render.
// Note: Fields MUST be capitalized so they are exported and accessible by the text/template engine!
type Config struct {
	TeamName     string   // e.g., "payments"
	SystemName   string   // e.g., "checkout"
	AppName      string   // e.g., "checkout-api"
	Runtime      string   // e.g., "go" (From golden-path)
	Capabilities []string // e.g., ["postgres", "s3"] (From flags or golden-path)
}

// walkAndRender recursively traverses sourceDir and renders every template file into targetDir.
func walkAndRender(sourceDir string, targetDir string, cfg Config) error {

	// 1. Define an anonymous callback function (closure) for processing each file/directory.
	// Because this function is declared INSIDE walkAndRender, it automatically captures
	// outer variables: sourceDir, targetDir, and cfg.
	handleFile := func(srcPath string, d fs.DirEntry, err error) error {
		// If filepath.WalkDir encountered a file system error (e.g. permission denied), return it immediately
		if err != nil {
			return err
		}

		// Skip Python Copier configuration files (these are not template files)
		if d.Name() == "copier.yml" || d.Name() == "copier.yaml" {
			return nil
		}

		// Calculate relative path inside template folder (e.g., "[[ .TeamName ]]/values.yaml.tmpl")
		relPath, err := filepath.Rel(sourceDir, srcPath)
		if err != nil {
			return err
		}

		// Evaluate template variables embedded inside directory or file names (e.g. "[[ .TeamName ]]" -> "payments")
		renderedRelPath, err := renderPath(relPath, cfg)
		if err != nil {
			return fmt.Errorf("failed to render path %s: %w", relPath, err)
		}

		// Compute final destination path on disk inside targetDir
		targetPath := filepath.Join(targetDir, renderedRelPath)

		// If current item is a directory, create it on disk and continue walking
		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		// If current item is a file, parse the template and write the rendered output to targetPath
		return processSingleTemplate(srcPath, targetPath, cfg)
	}

	// 2. Start walking the directory tree.
	// filepath.WalkDir calls handleFile on every file and folder it finds under sourceDir.
	return filepath.WalkDir(sourceDir, handleFile)
}

func processSingleTemplate(srcPath string, targetPath string, cfg Config) error {
	// Strip .tmpl extension for the output file
	targetPath = strings.TrimSuffix(targetPath, ".tmpl")

	// Create the output file on disk
	outFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create output file %s: %w", targetPath, err)
	}
	defer outFile.Close()

	// Parse and execute template from source path to outFile
	// 1. Read & compile the template file at 'srcPath'
	tmpl, err := template.New(filepath.Base(srcPath)).Delims("[[", "]]").ParseFiles(srcPath)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	// 2. Inject 'cfg' data into the compiled template and write to 'outFile'
	if err := tmpl.Execute(outFile, cfg); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	fmt.Println(srcPath, "-->", targetPath)

	return nil
}

// renderPath takes a path string like "[[ .TeamName ]]/app" and evaluates the template variables inside it
func renderPath(pathStr string, cfg Config) (string, error) {
	tmpl, err := template.New("path").Delims("[[", "]]").Parse(pathStr)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Add this helper function above RenderTenantFoundation
func resolveDestination(destTemplate string, cfg Config) string {
	dest := destTemplate
	dest = strings.ReplaceAll(dest, "{team}", cfg.TeamName)
	dest = strings.ReplaceAll(dest, "{system}", cfg.SystemName)
	dest = strings.ReplaceAll(dest, "{app}", cfg.AppName)
	dest = strings.ReplaceAll(dest, "{env}", "dev") // default to dev environment for scaffolding

	// Prepend the root target directory
	return filepath.Join("..", "..", "3-tenant-workloads", dest)
}

// RenderTenantFoundation scaffolds the gitops and infra base for a team
func RenderTenantFoundation(cfg Config) error {
	cat, err := catalog.LoadCatalog("../../1-platform-catalog/catalog.yaml")
	if err != nil {
		return err
	}

	// Render gitops-base
	gitopsSource := filepath.Join("..", "..", "1-platform-catalog", "blueprints", "team", "gitops")
	gitopsTarget := resolveDestination(cat.Destinations["blueprints/team/gitops"], cfg)
	if err := walkAndRender(gitopsSource, gitopsTarget, cfg); err != nil {
		return err
	}

	// Render infra-base
	infraSource := filepath.Join("..", "..", "1-platform-catalog", "blueprints", "team", "infra")
	infraTarget := resolveDestination(cat.Destinations["blueprints/team/infra"], cfg)
	return walkAndRender(infraSource, infraTarget, cfg)
}

// RenderSystem renders the ApplicationSet for a system grouping
func RenderSystem(cfg Config) error {
	cat, err := catalog.LoadCatalog("../../1-platform-catalog/catalog.yaml")
	if err != nil {
		return err
	}

	sourceDir := filepath.Join("..", "..", "1-platform-catalog", "blueprints", "system", "gitops")
	targetDir := resolveDestination(cat.Destinations["blueprints/system/gitops"], cfg)

	return walkAndRender(sourceDir, targetDir, cfg)
}

// RenderService renders runtime, delivery, and capability templates for a service
func RenderService(cfg Config) error {
	cat, err := catalog.LoadCatalog("../../1-platform-catalog/catalog.yaml")
	if err != nil {
		return err
	}

	// 1. Render Runtime
	if cfg.Runtime != "" {
		runtimeSrc := filepath.Join("..", "..", "1-platform-catalog", "building-blocks", "runtimes", cfg.Runtime)
		runtimeTarget := resolveDestination(cat.Destinations["runtimes"], cfg)
		if err := walkAndRender(runtimeSrc, runtimeTarget, cfg); err != nil {
			return err
		}
	}

	// 2. Render Delivery (Release Values)
	deliverySrc := filepath.Join("..", "..", "1-platform-catalog", "building-blocks", "delivery", "release")
	deliveryTarget := resolveDestination(cat.Destinations["delivery/release"], cfg)
	if err := walkAndRender(deliverySrc, deliveryTarget, cfg); err != nil {
		return err
	}

	// 3. Render Infrastructure Capabilities
	infraTargetDir := resolveDestination(cat.Destinations["capabilities"], cfg)
	if err := os.MkdirAll(infraTargetDir, 0755); err != nil {
		return err
	}

	for _, cap := range cfg.Capabilities {
		srcFile := filepath.Join("..", "..", "1-platform-catalog", "building-blocks", "capabilities", cap+".tf.tmpl")
		destFile := filepath.Join(infraTargetDir, cap+".tf")

		fmt.Printf("Adding infrastructure capability: %s\n", cap)
		if err := processSingleTemplate(srcFile, destFile, cfg); err != nil {
			return fmt.Errorf("failed rendering capability %s: %w", cap, err)
		}
	}

	return nil
}
