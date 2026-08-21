package templater

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
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
	Env          string   // e.g., "dev" (Default value, can be overridden by flags or golden-path)
	Runtime      string   // e.g., "go" (From runtimes)
	Capabilities []string // e.g., ["postgres", "s3"] (From flags or golden-path)
}

type Renderer struct {
	CatalogFS fs.FS
	Spec      *catalog.Catalog
	OutputDir string
	Writer    Writer // The interface for writing files and directories
	Force     bool   // Whether to force the overwrite of existing files
}

type CapabilityView struct {
	Config
	Name                   string
	Module                 string
	Version                string
	CapabilitiesSourceBase string
}

// walkAndRender recursively traverses sourceDir and renders every template file into targetDir.
func (r *Renderer) walkAndRender(sourceDir string, targetDir string, cfg Config) error {

	// 1. Define an anonymous callback function (closure) for processing each file/directory.
	// Because this function is declared INSIDE walkAndRender, it automatically captures
	// outer variables: sourceDir, targetDir, and cfg.
	handleFile := func(srcPath string, d fs.DirEntry, err error) error {
		// If filepath.WalkDir encountered a file system error (e.g. permission denied), return it immediately
		if err != nil {
			return err
		}

		// Calculate relative path inside template folder (e.g., "[[ .TeamName ]]/values.yaml.tmpl")
		relPath := strings.TrimPrefix(srcPath, sourceDir)
		relPath = strings.TrimPrefix(relPath, "/")

		// Evaluate template variables embedded inside directory or file names (e.g. "[[ .TeamName ]]" -> "payments")
		renderedRelPath, err := renderPath(relPath, cfg)
		if err != nil {
			return fmt.Errorf("failed to render path %s: %w", relPath, err)
		}

		// Compute final destination path on disk inside targetDir
		targetPath := filepath.Join(targetDir, renderedRelPath)

		// If current item is a directory, create it on disk and continue walking
		if d.IsDir() {
			return r.writer().MkdirAll(targetPath)
		}

		// If current item is a file, parse the template and write the rendered output to targetPath
		return r.processSingleTemplate(srcPath, targetPath, cfg)
	}

	// 2. Start walking the directory tree.
	// fs.WalkDir calls handleFile on every file and folder it finds under sourceDir.
	return fs.WalkDir(r.CatalogFS, sourceDir, handleFile)
}

func (r *Renderer) processSingleTemplate(srcPath string, targetPath string, data any) error {
	// Strip .tmpl extension for the output file
	targetPath = strings.TrimSuffix(targetPath, ".tmpl")

	// Skip existing files unless --force is passed
	// Skip check lives ABOVE the Writer so dry-run reports skips too!
	if !r.Force {
		if _, err := os.Stat(targetPath); err == nil {
			fmt.Printf("[SKIP] %s (exists; use --force to overwrite)\n", targetPath)
			return nil
		}
	}

	// Parse and execute template from source path to outFile
	// 1. Read & compile the template file at 'srcPath'
	rawBytes, err := fs.ReadFile(r.CatalogFS, srcPath)
	if err != nil {
		return fmt.Errorf("failed to read template %s: %w", srcPath, err)
	}

	tmpl, err := template.New(filepath.Base(srcPath)).Delims("[[", "]]").Parse(string(rawBytes))
	if err != nil {
		return fmt.Errorf("failed to parse template %s: %w", srcPath, err)
	}

	// 2. Render into a memory-buffer first, to avoid 0-byte files on disk if execution fails
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to render template %s: %w", srcPath, err)
	}

	// 3. Write the output file on disk.
	if err := r.writer().WriteFile(targetPath, buf.Bytes()); err != nil {
		return fmt.Errorf("failed to write output file %s: %w", targetPath, err)
	}

	fmt.Println(srcPath, "-->", targetPath)

	return nil
}

func (r *Renderer) writer() Writer {
	if r.Writer != nil {
		return r.Writer
	}
	return OSWriter{}
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

// Substitutes {team}/{system}/{app}/{env} and prefixes OutputDir
func (r *Renderer) resolveDestination(destTemplate string, cfg Config) string {
	dest := destTemplate
	dest = strings.ReplaceAll(dest, "{team}", cfg.TeamName)
	dest = strings.ReplaceAll(dest, "{system}", cfg.SystemName)
	dest = strings.ReplaceAll(dest, "{app}", cfg.AppName)

	env := "dev" // default to dev environment for scaffolding
	if cfg.Env != "" {
		env = cfg.Env
	}
	dest = strings.ReplaceAll(dest, "{env}", env)

	// Prepend the root target directory
	return filepath.Join(r.OutputDir, dest)
}

// blueprint pairs a source directory inside the catalog with the destinations key
// that decides where its output lands. The two are usually the same string; runtime
// is the exception, since its source carries the runtime name as a final segment.
type blueprint struct {
	src     string
	destKey string
}

// RenderTenantFoundation scaffolds everything a team gets exactly once. All of it
// is platform-owned, which is why it sits behind a single verb.
func (r *Renderer) RenderTenantFoundation(cfg Config) error {
	teamBlueprints := []blueprint{
		// Each destination key IS the source directory inside the catalog.yaml
		{src: "blueprints/team/apps", destKey: "blueprints/team/apps"},     // CODEOWNERS for the app-source repo
		{src: "blueprints/team/infra", destKey: "blueprints/team/infra"},   // CODEOWNERS + platform/ (providers, backend, team IAM)
		{src: "blueprints/team/gitops", destKey: "blueprints/team/gitops"}, // CODEOWNERS + platform/ (tenancy boundary, ApplicationSet)
	}

	return r.renderDestinations(teamBlueprints, cfg)
}

// RenderService renders runtime, delivery, and capability templates for a service
func (r *Renderer) RenderService(cfg Config) error {
	// Guard the exported boundary rather than trusting the caller: path.Join drops
	// empty segments, so an empty Runtime would silently point the walk at
	// building-blocks/runtimes and render EVERY runtime into the one app directory.
	if cfg.Runtime == "" {
		return &ValidationError{
			Field: "runtime",
			Err:   ErrRuntimeRequired,
		}
	}

	buildingBlocks := []blueprint{
		{src: path.Join("building-blocks", "runtimes", cfg.Runtime), destKey: "building-blocks/runtimes"},
		{src: "building-blocks/service-meta", destKey: "building-blocks/service-meta"},
		{src: "building-blocks/delivery/release", destKey: "building-blocks/delivery/release"},
	}

	// Render Runtime, Service Meta, Delivery (Release)
	if err := r.renderDestinations(buildingBlocks, cfg); err != nil {
		return err
	}

	// Render Infrastructure Capabilities
	infraTargetDir := r.resolveDestination(r.Spec.Destinations["building-blocks/capabilities"], cfg)
	if err := r.writer().MkdirAll(infraTargetDir); err != nil {
		return err
	}

	for _, capName := range cfg.Capabilities {
		// 1. Look up the capability's details from the catalog
		spec, ok := r.Spec.Capabilities[capName]
		if !ok {
			return &ValidationError{
				Field: "capability",
				Value: capName,
				Err:   ErrUnknownCapability,
			}
		}

		// 2. Construct the focused view for this specific template
		view := CapabilityView{
			Config:                 cfg,
			Name:                   capName,
			Module:                 spec.Module,
			Version:                spec.Version,
			CapabilitiesSourceBase: r.Spec.CapabilitiesSourceBase,
		}

		srcFile := path.Join("building-blocks", "capabilities", capName+".tf.tmpl")
		destFile := filepath.Join(infraTargetDir, capName+".tf")

		fmt.Printf("Adding infrastructure capability: %s\n", capName)

		// 3. Pass `view` instead of `cfg`
		if err := r.processSingleTemplate(srcFile, destFile, view); err != nil {
			return fmt.Errorf("failed rendering capability %s: %w", capName, err)
		}
	}

	return nil
}

func (r *Renderer) renderDestinations(bps []blueprint, cfg Config) error {
	for _, item := range bps {
		target := r.resolveDestination(r.Spec.Destinations[item.destKey], cfg)
		if err := r.walkAndRender(item.src, target, cfg); err != nil {
			return fmt.Errorf("rendering %s: %w", item.src, err)
		}
	}
	return nil
}
