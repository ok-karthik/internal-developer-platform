package catalog

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
)

func catalogYAML(goldenPathRuntime string, declared ...string) string {
	var runtimes strings.Builder
	for _, r := range declared {
		fmt.Fprintf(&runtimes, "  %s: { description: %s HTTP service }\n", r, r)
	}
	return fmt.Sprintf(`capabilities_source_base: "git::https://example.com//modules"

golden-paths:
  - name: svc
    runtime: %s
    capabilities: []

runtimes:
%s
capabilities:
  postgres: { module: aws-postgres, version: v1.1.0 }

destinations:
  blueprints/team/apps: "{team}/apps/"
  blueprints/team/infra: "{team}/infra/"
  blueprints/team/gitops: "{team}/gitops/"
  building-blocks/runtimes: "{team}/apps/{app}/"
  building-blocks/service-meta: "{team}/apps/{app}/"
  building-blocks/capabilities: "{team}/infra/apps/{app}/{env}/"
  building-blocks/delivery/release: "{team}/gitops/apps/{app}/{env}/"
`, goldenPathRuntime, runtimes.String())

}

func TestLoadCatalog(t *testing.T) {
	tests := []struct {
		name              string
		goldenPathRuntime string
		declared          []string
		runtimeDirs       []string
		wantErrContains   string
	}{
		{
			name:              "undeclared runtime directory is ignored",
			goldenPathRuntime: "go",
			declared:          []string{"go"},
			runtimeDirs:       []string{"go", "rust"}, // rust on disk, but not declared in YAML
			wantErrContains:   "",                     // "" means WE EXPECT IT TO SUCCEED!
		},
		{
			name:              "golden path names an undeclared runtime",
			goldenPathRuntime: "rust",         // YAML wants rust
			declared:          []string{"go"}, // YAML only offers go
			runtimeDirs:       []string{"go", "rust"},
			wantErrContains:   "unknown runtime", // WE EXPECT AN ERROR containing "unknown runtime"
		},
		{
			name:              "declared runtime has no directory",
			goldenPathRuntime: "go",
			declared:          []string{"go", "rust"}, // YAML offers go AND rust
			runtimeDirs:       []string{"go"},         // But rust folder is missing on disk!
			wantErrContains:   "does not exist",       // WE EXPECT AN ERROR containing "does not exist"
		},
	}

	for _, tt := range tests {
		// tt represents the current row in the loop!

		t.Run(tt.name, func(t *testing.T) {
			// 1. Create mock filesystem using tt's inputs
			mockFS := fstest.MapFS{
				"catalog.yaml": {Data: []byte(catalogYAML(tt.goldenPathRuntime, tt.declared...))},
			}

			// 2. Create folders specified in tt.runtimeDirs
			for _, dir := range tt.runtimeDirs {
				mockFS["building-blocks/runtimes/"+dir+"/main.tmpl"] = &fstest.MapFile{Data: []byte("x")}
			}

			// 3. Call LoadCatalog
			_, err := LoadCatalog(mockFS)

			// 4. Assert the result!

			// CASE A: We expected SUCCESS (wantErrContains is empty)
			if tt.wantErrContains == "" {
				if err != nil {
					t.Fatalf("LoadCatalog() returned unexpected error: %v", err)
				}
				return
			}

			// CASE B: We expected FAILURE (wantErrContains has text)
			if err == nil || !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Fatalf("want error containing %q, got: %v", tt.wantErrContains, err)
			}
		})
	}

}

// A runtime directory that the catalog does not declare is deliberately
// invisible, NOT an error — that is the whole point of declaring the offered
// set in catalog.yaml. Deleting this test would let someone "fix" validate()
// by adding a reverse check and silently remove the feature.
func TestLoadCatalog_UndeclaredRuntimeDirectoryIsIgnored(t *testing.T) {
	mockFS := fstest.MapFS{
		"catalog.yaml": {Data: []byte(catalogYAML("go", "go"))},
		"building-blocks/runtimes/go/main.go.tmpl": {Data: []byte("package main")},
		// on disk, but absent from runtimes: — must not fail the load
		"building-blocks/runtimes/rust/main.rs.tmpl": {Data: []byte("fn main() {}")},
	}
	cat, err := LoadCatalog(mockFS)
	if err != nil {
		t.Fatalf("LoadCatalog() returned unexpected error: %v", err)
	}
	if _, offered := cat.Runtimes["rust"]; offered {
		t.Error("rust is on disk but undeclared; it must not appear in the offered set")
	}
}
