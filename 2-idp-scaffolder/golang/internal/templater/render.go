package templater

import (
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
	Runtime      string   // e.g., "go" (From golden-path)
	Capabilities []string // e.g., ["postgres", "s3"] (From flags or golden-path)
}

type Renderer struct {
	CatalogFS fs.FS
	Spec      *catalog.Catalog
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
			return os.MkdirAll(targetPath, 0755)
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

	// Create the output file on disk
	outFile, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create output file %s: %w", targetPath, err)
	}
	defer outFile.Close()

	// Parse and execute template from source path to outFile
	// 1. Read & compile the template file at 'srcPath'
	rawBytes, err := fs.ReadFile(r.CatalogFS, srcPath)
	tmpl, err := template.New(filepath.Base(srcPath)).Delims("[[", "]]").Parse(string(rawBytes))
	if err != nil {
		return err
	}

	// 2. Inject 'cfg' data into the compiled template and write to 'outFile'
	if err := tmpl.Execute(outFile, data); err != nil {
		return err
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
	return filepath.Join("../../3-tenant-workloads", dest)
	//return path.Clean(dest)
}

// teamBlueprints are every catalog directory rendered once per team. Because each
// destinations key IS the source directory inside the catalog, one string drives
// both halves of the walk — add a blueprint by adding a key to catalog.yaml and a
// line here, with no new path logic.
var teamBlueprints = []string{
	"blueprints/team/apps",   // CODEOWNERS for the app-source repo
	"blueprints/team/infra",  // CODEOWNERS + platform/ (providers, backend, team IAM)
	"blueprints/team/gitops", // CODEOWNERS + platform/ (tenancy boundary, ApplicationSet)
}

// RenderTenantFoundation scaffolds everything a team gets exactly once. All of it
// is platform-owned, which is why it sits behind a single verb.
func (r *Renderer) RenderTenantFoundation(cfg Config) error {
	for _, source := range teamBlueprints {
		target := resolveDestination(r.Spec.Destinations[source], cfg)
		if err := r.walkAndRender(source, target, cfg); err != nil {
			return fmt.Errorf("rendering %s: %w", source, err)
		}
	}
	return nil
}

// RenderService renders runtime, delivery, and capability templates for a service
func (r *Renderer) RenderService(cfg Config) error {
	// 1. Render Runtime
	if cfg.Runtime != "" {
		runtimeSrc := path.Join("building-blocks", "runtimes", cfg.Runtime)
		runtimeTarget := resolveDestination(r.Spec.Destinations["building-blocks/runtimes"], cfg)
		if err := r.walkAndRender(runtimeSrc, runtimeTarget, cfg); err != nil {
			return err
		}
	}

	// 2. Render Service Metadata (Backstage catalog-info.yaml)
	// Lives outside runtimes/ because it is runtime-agnostic — a walk rooted at
	// building-blocks/runtimes/<runtime>/ would never reach it.
	metaSrc := path.Join("building-blocks", "service-meta")
	metaTarget := resolveDestination(r.Spec.Destinations["building-blocks/service-meta"], cfg)
	if err := r.walkAndRender(metaSrc, metaTarget, cfg); err != nil {
		return err
	}

	// 3. Render Delivery (Release Values)
	deliverySrc := path.Join("building-blocks", "delivery", "release")
	deliveryTarget := resolveDestination(r.Spec.Destinations["building-blocks/delivery/release"], cfg)
	if err := r.walkAndRender(deliverySrc, deliveryTarget, cfg); err != nil {
		return err
	}

	// 4. Render Infrastructure Capabilities
	infraTargetDir := resolveDestination(r.Spec.Destinations["building-blocks/capabilities"], cfg)
	if err := os.MkdirAll(infraTargetDir, 0755); err != nil {
		return err
	}

	for _, capName := range cfg.Capabilities {
		// 1. Look up the capability's details from the catalog
		spec, ok := r.Spec.Capabilities[capName]
		if !ok {
			return fmt.Errorf("unknown capability: %s", capName)
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
