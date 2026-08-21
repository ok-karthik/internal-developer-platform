package templater

import (
	"bytes"
	"flag"
	"maps"
	"os"
	"path/filepath"
	"scaffolder/internal/catalog"
	"slices"
	"testing"
)

// 1. Declare the -update flag
var update = flag.Bool("update", false, "update golden files")

// Path to 1-platform-catalog relative to internal/templater directory
const catalogDir = "../../../../1-platform-catalog"

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
		TeamName: "payments",
		AppName:  "checkout",
		Env:      "dev",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// D. Render the service
	if err := r.RenderService(cfg); err != nil {
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
		TeamName: "payments",
		AppName:  "checkout",
		Runtime:  "doesnotexist",
	}

	// 1. Assert non-nil error
	err = r.RenderService(cfg)
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
		TeamName: "payments",
		AppName:  "checkout",
		Env:      "dev",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if err := r.RenderService(cfg); err != nil {
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
		TeamName: "payments",
		AppName:  "checkout",
		Env:      "dev",
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// First render: creates files
	if err := r.RenderService(cfg); err != nil {
		t.Fatalf("First RenderService failed: %v", err)
	}

	// Hand edit main.go
	mainGoPath := filepath.Join(tmpOut, "payments", "apps", "checkout", "main.go")
	handEdit := []byte("// hand edit\n")
	if err := os.WriteFile(mainGoPath, handEdit, 0644); err != nil {
		t.Fatalf("Failed writing hand edit: %v", err)
	}

	// Second render with Force: false (should skip existing files)
	if err := r.RenderService(cfg); err != nil {
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
	if err := r.RenderService(cfg); err != nil {
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
