package templater

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

type Config struct {
	TeamName     string   // e.g., "payments"
	SystemName   string   // e.g., "checkout"
	AppName      string   // e.g., "checkout-api"
	Runtime      string   // e.g., "go" (From golden-path)
	Capabilities []string // e.g., ["postgres", "s3"] (From flags or golden-path)
}

func RenderTemplates(cfg Config) {

	sourceDir := filepath.Join("..", "1-idp-scaffolder-templates", "tenant-template")
	if err := filepath.WalkDir(sourceDir, func(srcPath string, d fs.DirEntry, err error) error {
		// Handle permission denied / broken symlinks / other fs errors
		if err != nil {
			return err
		}

		// Skip Copier config files meant for Python scaffolder
		if d.Name() == "copier.yml" || d.Name() == "copier.yaml" {
			return nil
		}

		// Calculate relative path (removes "internal/templater/templates/")
		relPath, err := filepath.Rel(sourceDir, srcPath)
		if err != nil {
			return err
		}
		// Compute target destination path
		renderedRelPath, err := renderPath(relPath, cfg)
		if err != nil {
			return fmt.Errorf("failed to render path %s: %w", relPath, err)
		}
		// renderedRelPath becomes: "payments/app/main.py.tmpl"
		targetPath := filepath.Join("..", "3-tenant-workloads", renderedRelPath)
		// targetPath becomes: "2-tenant-workloads/payments/app/main.py"
		//targetPath := filepath.Join(targetDir, relPath)

		// Create directory structure if item is a folder
		if d.IsDir() {
			return os.MkdirAll(targetPath, 0775)
		}

		return processSingleTemplate(srcPath, targetPath, cfg)
	}); err != nil {
		fmt.Printf("Error walking template directory: %v\n", err)
		return
	}
	fmt.Println("\nSuccessfully rendered all templates for", cfg.TeamName, "team and", cfg.AppName, "app")

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
