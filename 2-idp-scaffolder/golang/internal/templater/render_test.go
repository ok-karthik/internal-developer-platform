package templater

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"scaffolder/internal/catalog"
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

func compareGolden(t *testing.T, actualDir, goldenDir string) {
	t.Helper()

	if err := filepath.WalkDir(goldenDir, func(expectedPath string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(goldenDir, expectedPath)
		if err != nil {
			return err
		}

		actualPath := filepath.Join(actualDir, rel)
		if _, err := os.Stat(actualPath); os.IsNotExist(err) {
			t.Errorf("Missing file in actual output: %s", rel)
			return nil
		}

		expectedContent, err := os.ReadFile(expectedPath)
		if err != nil {
			return err
		}

		actualContent, err := os.ReadFile(actualPath)
		if err != nil {
			return err
		}

		if !bytes.Equal(expectedContent, actualContent) {
			t.Errorf("File mismatch: %s", rel)
		}

		return nil
	}); err != nil {
		t.Fatalf("filepath.WalkDir failed: %v", err)
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
