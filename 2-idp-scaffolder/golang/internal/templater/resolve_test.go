package templater

import (
	"scaffolder/internal/catalog"
	"slices"
	"testing"
)

// testCatalog returns a small in-memory catalog. Resolve reads GoldenPaths and
// Runtimes, so nothing else needs filling in — and because Resolve takes the
// catalog as a parameter, this test never touches the filesystem or the network.
// That is the payoff of extracting it out of the cobra RunE closure.
func testCatalog() *catalog.Catalog {
	return &catalog.Catalog{
		// The offered set. Resolve rejects any runtime absent from this map, so a
		// case exercising an unknown runtime just leaves it out.
		Runtimes: map[string]catalog.Runtime{
			"go":     {Description: "Go HTTP service"},
			"python": {Description: "Python FastAPI service"},
		},
		GoldenPaths: []catalog.GoldenPath{
			{
				Name:         "go-service-postgres",
				Runtime:      "go",
				Capabilities: []string{"postgres"},
			},
			{
				Name:         "python-worker-s3",
				Runtime:      "python",
				Capabilities: []string{"s3"},
			},
		},
	}
}

// TestResolve is a table-driven test: one slice of cases, one loop, one t.Run
// subtest per case. This is the dominant test idiom in Go — adding coverage means
// adding a struct literal, not copying a function. Run a single case with:
//
//	go test ./internal/templater -run 'TestResolve/golden_path_only' -v
func TestResolve(t *testing.T) {
	tests := []struct {
		name        string
		goldenPath  string   // the --golden-path argument
		in          Config   // the flag-populated Config
		wantRuntime string   // expected resolved runtime
		wantCaps    []string // expected resolved capabilities (sorted, deduped)
		wantErr     bool
	}{
		{
			name:        "runtime flag only",
			goldenPath:  "",
			in:          Config{Runtime: "go", Capabilities: []string{"postgres", "s3"}},
			wantRuntime: "go",
			wantCaps:    []string{"postgres", "s3"},
		},
		{
			name:        "golden path only seeds runtime and capabilities",
			goldenPath:  "go-service-postgres",
			in:          Config{},
			wantRuntime: "go",
			wantCaps:    []string{"postgres"},
		},
		{
			name:        "explicit runtime overrides the golden path",
			goldenPath:  "python-worker-s3",
			in:          Config{Runtime: "go"},
			wantRuntime: "go",
			wantCaps:    []string{"s3"}, // capabilities still come from the golden path
		},
		{
			name:       "unknown golden path is an error",
			goldenPath: "does-not-exist",
			in:         Config{},
			wantErr:    true,
		},
		// TODO(you): add these three.
		//
		// 1. "duplicates are collapsed" — golden path go-service-postgres plus
		//    in.Capabilities of {"postgres", "iam", "postgres"}. Expect
		//    {"iam", "postgres"}: sorted, each appearing once. This is the case
		//    that broke when the dedup was first rewritten.
		//
		// 2. "no runtime and no golden path is an error" — goldenPath "" and an
		//    empty Config. The CLI blocks this with MarkFlagsOneRequired, but
		//    Resolve is exported and cmd/api will not go through cobra.
		//
		// 3. "capabilities are sorted deterministically" — pass them out of order
		//    and assert the resolved order is stable. Sorting is what makes the
		//    rendered output diffable across runs.
		{
			name:        "duplicates are collapsed",
			goldenPath:  "go-service-postgres",
			in:          Config{Runtime: "go", Capabilities: []string{"postgres", "iam", "postgres"}},
			wantRuntime: "go",
			wantCaps:    []string{"iam", "postgres"}, // capabilities has to come de-duplicated and sorted
		},
		{
			name:       "no runtime and no golden path is an error",
			goldenPath: "",
			in:         Config{},
			wantErr:    true,
		},
		{
			name:        "capabilities are sorted deterministically",
			goldenPath:  "python-worker-s3",
			in:          Config{Runtime: "go", Capabilities: []string{"postgres", "s3", "postgres", "s3", "iam", "iam"}},
			wantRuntime: "go",
			wantCaps:    []string{"iam", "postgres", "s3"}, // capabilities has to come sorted deterministically
		},
	}

	for _, tt := range tests {
		// tt is captured per-iteration; each subtest gets its own name in output.
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(testCatalog(), tt.goldenPath, tt.in)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("Resolve() succeeded, want an error")
				}
				return // nothing else to assert on the error path
			}
			if err != nil {
				t.Fatalf("Resolve() returned unexpected error: %v", err)
			}

			if got.Runtime != tt.wantRuntime {
				t.Errorf("Runtime = %q, want %q", got.Runtime, tt.wantRuntime)
			}
			// slices.Equal compares element-wise; it treats nil and empty as equal,
			// which is what we want since "no capabilities" can legitimately be either.
			if !slices.Equal(got.Capabilities, tt.wantCaps) {
				t.Errorf("Capabilities = %v, want %v", got.Capabilities, tt.wantCaps)
			}
		})
	}
}

// TestResolveDoesNotMutateInput pins down the reason Resolve clones its input
// slice. Config is passed by value, so assigning to got.Runtime cannot affect the
// caller — but a struct copy shares the *backing array* of any slice field, so an
// append inside Resolve could write through into the caller's slice.
//
// TODO(you): this test currently asserts nothing. Fill it in:
//   - build a Config whose Capabilities has spare capacity, e.g.
//     caps := make([]string, 1, 4); caps[0] = "iam"
//   - call Resolve with a golden path that contributes more capabilities
//   - assert caps is still exactly []string{"iam"} afterwards
//
// Then delete the slices.Clone line in Resolve and watch this test fail. That
// failure is the whole lesson — the aliasing bug it prevents is invisible
// otherwise, and no compiler or linter will point at it.
func TestResolveDoesNotMutateInput(t *testing.T) {
	// The setup needs three properties at once, and dropping any one of them
	// makes the bug invisible and the test worthless:
	//
	//  1. len < cap. append only writes into the caller's backing array while
	//     there is spare room; at len == cap it allocates a fresh array and the
	//     aliasing quietly disappears. That capacity dependence is exactly why
	//     this class of bug is intermittent in production.
	//  2. At least TWO elements. The mutation that is actually observable is
	//     slices.Sort reordering in place — a one-element slice has nothing to
	//     reorder.
	//  3. Unsorted. If the input is already in sorted order, Sort is a no-op and
	//     changes nothing to detect.
	caps := make([]string, 2, 4)
	caps[0] = "s3"
	caps[1] = "iam"

	in := Config{Capabilities: caps}

	// go-service-postgres contributes "postgres", so append runs.
	got, err := Resolve(testCatalog(), "go-service-postgres", in)
	if err != nil {
		t.Fatalf("Resolve() returned unexpected error: %v", err)
	}

	// The RETURN VALUE is correct with or without the clone — that is the trap.
	// Asserting only on `got` would leave the bug completely undetected.
	if want := []string{"iam", "postgres", "s3"}; !slices.Equal(got.Capabilities, want) {
		t.Errorf("got.Capabilities = %v, want %v", got.Capabilities, want)
	}

	// This is the assertion that matters: the caller handed Resolve a slice and
	// must get it back untouched. Without slices.Clone, append writes "postgres"
	// into caps[2] (past len, invisible) and then slices.Sort reorders indices
	// 0..2 of the shared array, so caps becomes ["iam", "postgres"] — visibly
	// corrupted, and the caller never called anything that should reorder it.
	if want := []string{"s3", "iam"}; !slices.Equal(caps, want) {
		t.Errorf("Resolve mutated the caller's slice: caps = %v, want %v", caps, want)
	}
}
