package templater

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"scaffolder/internal/catalog"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// 1. Declare the -update flag
var update = flag.Bool("update", false, "update golden files")

// Path to 1-platform-catalog relative to internal/templater directory
const catalogDir = "../../../1-platform-catalog"

func TestRenderServiceGolden(t *testing.T) {
	// A. Load catalog
	spec, err := catalog.LoadCatalog(os.DirFS(catalogDir))
	if err != nil {
		t.Fatalf("LoadCatalog failed: %v", err)
	}

	// B. Create temporary directory for test output
	tmpOut := t.TempDir()

	r := &Renderer{
		CatalogFS: os.DirFS(catalogDir),
		Spec:      spec,
		OutputDir: tmpOut,
	}

	// C. Resolve config for a test service (e.g. go-service-postgres)
	cfg, err := Resolve(spec, "go-service-postgres", Config{
		TenantName: "payments",
		AppName:    "checkout",
		Env:        "dev",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// D. Render the service
	if err := r.RenderService(context.Background(), cfg); err != nil {
		t.Fatalf("RenderService failed: %v", err)
	}

	// E. Target golden directory: internal/templater/testdata/render_service
	goldenDir := filepath.Join("testdata", "render_service")

	// F. If -update flag is passed, copy tmpOut -> goldenDir
	if *update {
		// Update golden files logic
		updateGolden(t, tmpOut, goldenDir)
		return
	}

	// G. Otherwise compare tmpOut vs goldenDir
	compareGolden(t, tmpOut, goldenDir)
}

func updateGolden(t *testing.T, tmpDir, goldenDir string) {
	t.Helper()
	if err := os.MkdirAll(goldenDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	if err := filepath.WalkDir(tmpDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(tmpDir, path)
		if err != nil {
			return err
		}

		goldenPath := filepath.Join(goldenDir, rel)
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
			return err
		}

		if err := copyFile(path, goldenPath); err != nil {
			return err
		}

		t.Logf("Updated golden file: %s", goldenPath)
		return nil
	}); err != nil {
		t.Fatalf("filepath.WalkDir failed: %v", err)
	}
}

func copyFile(src, dst string) error {
	content, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, content, 0644)
}

// collectFiles reads every file under dir into a map keyed by its path relative
// to dir. Loading both trees up front is what lets compareGolden check the set of
// paths in both directions, not just golden -> actual.
func collectFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()

	files := map[string][]byte{}
	if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		files[rel] = content
		return nil
	}); err != nil {
		t.Fatalf("filepath.WalkDir failed: %v", err)
	}

	return files
}

func compareGolden(t *testing.T, actualDir, goldenDir string) {
	t.Helper()

	expected := collectFiles(t, goldenDir)
	actual := collectFiles(t, actualDir)

	// Sorted so a failing run reports the same paths in the same order every
	// time; map iteration order would shuffle the output between runs.
	for _, rel := range slices.Sorted(maps.Keys(expected)) {
		actualContent, ok := actual[rel]
		if !ok {
			t.Errorf("Missing file in actual output: %s", rel)
			continue
		}
		if !bytes.Equal(expected[rel], actualContent) {
			t.Errorf("File mismatch: %s", rel)
		}
	}

	// The direction a walk over goldenDir alone cannot see: a file the renderer
	// emits that has no golden counterpart. Letting that pass would defeat the
	// point of pinning a scaffolder's output — a stray template or a destination
	// key writing somewhere new is exactly the regression this test exists for.
	for _, rel := range slices.Sorted(maps.Keys(actual)) {
		if _, ok := expected[rel]; !ok {
			t.Errorf("Unexpected file in actual output: %s", rel)
		}
	}
}

func TestRenderService_BadRuntimeErrorPath(t *testing.T) {
	spec, err := catalog.LoadCatalog(os.DirFS(catalogDir))
	if err != nil {
		t.Fatalf("LoadCatalog failed: %v", err)
	}

	tmpOut := t.TempDir()

	r := &Renderer{
		CatalogFS: os.DirFS(catalogDir),
		Spec:      spec,
		OutputDir: tmpOut,
	}

	// Pass a runtime that does not exist in the catalog
	cfg := Config{
		TenantName: "payments",
		AppName:    "checkout",
		Runtime:    "doesnotexist",
	}

	// 1. Assert non-nil error
	err = r.RenderService(context.Background(), cfg)
	if err == nil {
		t.Fatalf("RenderService() succeeded with invalid runtime, expected error")
	}

	// 2. Count files in tmpOut to ensure zero files were written
	fileCount := 0
	err = filepath.WalkDir(tmpOut, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			fileCount++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("filepath.WalkDir failed: %v", err)
	}

	if fileCount != 0 {
		t.Errorf("RenderService() wrote %d files on error path, want 0", fileCount)
	}
}

func TestRenderService_DryRun(t *testing.T) {
	spec, err := catalog.LoadCatalog(os.DirFS(catalogDir))
	if err != nil {
		t.Fatalf("LoadCatalog failed: %v", err)
	}

	tmpOut := t.TempDir()

	r := &Renderer{
		CatalogFS: os.DirFS(catalogDir),
		Spec:      spec,
		OutputDir: tmpOut,
		Writer:    DryRunWriter{},
	}

	cfg, err := Resolve(spec, "go-service-postgres", Config{
		TenantName: "payments",
		AppName:    "checkout",
		Env:        "dev",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if err := r.RenderService(context.Background(), cfg); err != nil {
		t.Fatalf("RenderService failed: %v", err)
	}

	fileCount := 0
	err = filepath.WalkDir(tmpOut, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			fileCount++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("filepath.WalkDir failed: %v", err)
	}

	if fileCount != 0 {
		t.Errorf("DryRunWriter wrote %d files to disk, expected 0", fileCount)
	}
}

func TestRenderService_SkipIfExists(t *testing.T) {
	spec, err := catalog.LoadCatalog(os.DirFS(catalogDir))
	if err != nil {
		t.Fatalf("LoadCatalog failed: %v", err)
	}

	tmpOut := t.TempDir()

	r := &Renderer{
		CatalogFS: os.DirFS(catalogDir),
		Spec:      spec,
		OutputDir: tmpOut,
		Writer:    OSWriter{},
		Force:     false,
	}

	cfg, err := Resolve(spec, "go-service-postgres", Config{
		TenantName: "payments",
		AppName:    "checkout",
		Env:        "dev",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// First render: creates files
	if err := r.RenderService(context.Background(), cfg); err != nil {
		t.Fatalf("First RenderService failed: %v", err)
	}

	// Hand edit main.go
	mainGoPath := filepath.Join(tmpOut, "payments", "workloads-repo", "services", "checkout", "main.go")
	handEdit := []byte("// hand edit\n")
	if err := os.WriteFile(mainGoPath, handEdit, 0644); err != nil {
		t.Fatalf("Failed writing hand edit: %v", err)
	}

	// Second render with Force: false (should skip existing files)
	if err := r.RenderService(context.Background(), cfg); err != nil {
		t.Fatalf("Second RenderService failed: %v", err)
	}

	gotContent, err := os.ReadFile(mainGoPath)
	if err != nil {
		t.Fatalf("Failed reading main.go: %v", err)
	}
	if !bytes.Equal(gotContent, handEdit) {
		t.Errorf("RenderService overwritten hand-edited file without --force")
	}

	// Third render with Force: true (should overwrite existing files)
	r.Force = true
	if err := r.RenderService(context.Background(), cfg); err != nil {
		t.Fatalf("Third RenderService failed: %v", err)
	}

	gotContentAfterForce, err := os.ReadFile(mainGoPath)
	if err != nil {
		t.Fatalf("Failed reading main.go: %v", err)
	}
	if bytes.Equal(gotContentAfterForce, handEdit) {
		t.Errorf("RenderService failed to overwrite file even with --force")
	}
}

func TestRenderService_ContextCanceled(t *testing.T) {
	// 1. Load the catalog into memory
	spec, err := catalog.LoadCatalog(os.DirFS(catalogDir))
	if err != nil {
		t.Fatalf("LoadCatalog failed: %v", err)
	}

	// 2. Create an isolated temporary directory for this test
	tmpOut := t.TempDir()

	r := &Renderer{
		CatalogFS: os.DirFS(catalogDir),
		Spec:      spec,
		OutputDir: tmpOut,
	}

	cfg := Config{
		TenantName: "payments",
		AppName:    "checkout",
		Env:        "dev",
		Runtime:    "go",
	}

	// 3. Create a context and cancel it IMMEDIATELY
	// This simulates: "The user pressed Ctrl+C or the HTTP request disconnected right at invocation"
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 👈 ctx is now marked as canceled!

	// 4. Call RenderService with the canceled context
	err = r.RenderService(ctx, cfg)

	// 5. Assertion 1: Did RenderService properly return context.Canceled?
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RenderService() error = %v, want errors.Is(..., context.Canceled)", err)
	}

	// 6. Assertion 2: Did it write zero files to disk?
	fileCount := 0
	err = filepath.WalkDir(tmpOut, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			fileCount++ // Count any files created in tmpOut
		}
		return nil
	})
	if err != nil {
		t.Fatalf("filepath.WalkDir failed: %v", err)
	}

	// If fileCount > 0, it means the scaffolder leaked half-written files to disk!
	if fileCount != 0 {
		t.Errorf("RenderService wrote %d files on canceled context, want 0", fileCount)
	}
}

// TestCatalogInfoIsValidBackstageEntity renders catalog-info.yaml and checks the
// shape Backstage needs: tags that obey its tag rule with no duplicates, links,
// a source-location annotation, and spec.system only when a system was given.
func TestCatalogInfoIsValidBackstageEntity(t *testing.T) {
	type entity struct {
		Metadata struct {
			Name        string   `yaml:"name"`
			Description string   `yaml:"description"`
			Tags        []string `yaml:"tags"`
			Links       []struct {
				URL   string `yaml:"url"`
				Title string `yaml:"title"`
				Icon  string `yaml:"icon"`
			} `yaml:"links"`
			Annotations map[string]string `yaml:"annotations"`
		} `yaml:"metadata"`
		// A map, not a struct with *string: an empty "system:" line decodes to a
		// nil pointer and would look absent, but Backstage rejects a null system.
		Spec map[string]any `yaml:"spec"`
	}
	tagRule := regexp.MustCompile(`^[a-z0-9:+#]+(-[a-z0-9:+#]+)*$`)
	nameRule := regexp.MustCompile(`^[a-zA-Z0-9]+([-_.][a-zA-Z0-9]+)*$`)
	slugRule := regexp.MustCompile(`^[^/\s]+/[^/\s]+$`)

	spec, err := catalog.LoadCatalog(os.DirFS(catalogDir))
	if err != nil {
		t.Fatalf("LoadCatalog failed: %v", err)
	}

	render := func(t *testing.T, system string) entity {
		t.Helper()
		out := t.TempDir()
		r := &Renderer{CatalogFS: os.DirFS(catalogDir), Spec: spec, OutputDir: out}
		cfg, err := Resolve(spec, "go-service-postgres", Config{
			TenantName:   "tenant-a",
			AppName:      "app-a",
			Env:          "dev",
			SystemName:   system,
			Capabilities: []string{"postgres", "s3"},
		})
		if err != nil {
			t.Fatalf("Resolve failed: %v", err)
		}
		if err := r.RenderService(context.Background(), cfg); err != nil {
			t.Fatalf("RenderService failed: %v", err)
		}
		raw, err := os.ReadFile(filepath.Join(out, "tenant-a/workloads-repo/services/app-a/catalog-info.yaml"))
		if err != nil {
			t.Fatalf("read catalog-info.yaml: %v", err)
		}
		var e entity
		if err := yaml.Unmarshal(raw, &e); err != nil {
			t.Fatalf("catalog-info.yaml is not valid YAML: %v\n%s", err, raw)
		}
		return e
	}

	t.Run("with system", func(t *testing.T) {
		e := render(t, "sys-a")
		if e.Metadata.Name != "app-a" {
			t.Errorf("metadata.name = %q, want app-a", e.Metadata.Name)
		}
		if len(e.Metadata.Name) > 63 || !nameRule.MatchString(e.Metadata.Name) {
			t.Errorf("metadata.name %q violates Backstage name rule", e.Metadata.Name)
		}
		if e.Metadata.Description == "" {
			t.Error("metadata.description is empty")
		}
		seen := map[string]bool{}
		for _, tag := range e.Metadata.Tags {
			if seen[tag] {
				t.Errorf("duplicate tag %q in %v", tag, e.Metadata.Tags)
			}
			seen[tag] = true
			if len(tag) > 63 || !tagRule.MatchString(tag) {
				t.Errorf("tag %q violates Backstage tag rule", tag)
			}
		}
		for _, want := range []string{"go", "postgres", "s3"} {
			if !seen[want] {
				t.Errorf("tags %v missing %q", e.Metadata.Tags, want)
			}
		}
		if len(e.Metadata.Links) != 3 {
			t.Errorf("got %d links, want 3", len(e.Metadata.Links))
		}
		for _, l := range e.Metadata.Links {
			if !strings.HasPrefix(l.URL, "https://") {
				t.Errorf("link %q is not https", l.URL)
			}
			if strings.Contains(l.URL, "/dev") {
				t.Errorf("link %q is env-specific; catalog-info is per-service", l.URL)
			}
		}
		// Backstage needs the trailing "/" on a folder location for relative paths to resolve.
		if loc := e.Metadata.Annotations["backstage.io/source-location"]; !strings.HasPrefix(loc, "url:https://") || !strings.HasSuffix(loc, "/") {
			t.Errorf("source-location = %q, want url:https://.../ (folder, trailing slash)", loc)
		}
		if slug := e.Metadata.Annotations["github.com/project-slug"]; !slugRule.MatchString(slug) {
			t.Errorf("github.com/project-slug = %q, want <owner>/<repo>", slug)
		}
		if got := e.Spec["system"]; got != "sys-a" {
			t.Errorf("spec.system = %v, want sys-a", got)
		}
	})

	t.Run("without system", func(t *testing.T) {
		e := render(t, "")
		if got, ok := e.Spec["system"]; ok {
			t.Errorf("spec.system = %v, want the key absent", got)
		}
	})
}

func TestRenderRejectsBadNamesAndWritesNothing(t *testing.T) {
	spec, err := catalog.LoadCatalog(os.DirFS(catalogDir))
	if err != nil {
		t.Fatalf("LoadCatalog failed: %v", err)
	}

	goodSvc := Config{TenantName: "tenant-a", AppName: "app-a", Env: "dev", Runtime: "go"}
	goodTenant := Config{TenantName: "tenant-a", Owners: []string{"team-a"}}

	with := func(c Config, f func(*Config)) Config { f(&c); return c }

	cases := []struct {
		name    string
		service bool
		cfg     Config
	}{
		{"tenant traversal onboard", false, with(goodTenant, func(c *Config) { c.TenantName = "../../x" })},
		{"tenant traversal add-service", true, with(goodSvc, func(c *Config) { c.TenantName = "../../x" })},
		{"bad app", true, with(goodSvc, func(c *Config) { c.AppName = "Bad Name: x" })},
		{"bad env", true, with(goodSvc, func(c *Config) { c.Env = "../prod" })},
		{"bad owner", false, with(goodTenant, func(c *Config) { c.Owners = []string{"Team A"} })},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Output goes two levels below the walked root, so a "../../x" name that
			// escapes OutputDir still lands inside root and is counted below.
			root := t.TempDir()
			tmpOut := filepath.Join(root, "out", "repo")
			r := &Renderer{CatalogFS: os.DirFS(catalogDir), Spec: spec, OutputDir: tmpOut}

			if tc.service {
				err = r.RenderService(context.Background(), tc.cfg)
			} else {
				err = r.RenderTenantFoundation(context.Background(), tc.cfg)
			}
			if !errors.Is(err, ErrInvalidName) {
				t.Fatalf("error = %v, want ErrInvalidName", err)
			}

			files := 0
			walkErr := filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() {
					files++
				}
				return nil
			})
			if walkErr != nil {
				t.Fatalf("WalkDir: %v", walkErr)
			}
			if files != 0 {
				t.Errorf("wrote %d files, want 0", files)
			}
		})
	}
}
