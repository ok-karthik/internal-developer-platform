package templater

import (
	"scaffolder/internal/catalog"
	"slices"
)

// Resolve merges a golden path with explicit overrides into the final Config.
// It reads nothing outside its parameters, so it is safe for any caller — CLI or API.
func Resolve(spec *catalog.Catalog, goldenPath string, in Config) (Config, error) {
	out := in
	// Clone so appending and sorting below cannot write into the caller's
	// backing array. See TestResolveDoesNotMutateInput — removing this line
	// still returns the right value, and still corrupts the caller's slice.
	out.Capabilities = slices.Clone(in.Capabilities)

	if goldenPath != "" {
		gp, found := spec.FindGoldenPath(goldenPath)
		if !found {
			return Config{}, &ValidationError{
				Field: "golden-path",
				Value: goldenPath,
				Err:   ErrUnknownGoldenPath,
			}
		}
		// An explicit --runtime wins over the golden path's.
		if out.Runtime == "" {
			out.Runtime = gp.Runtime
		}
		out.Capabilities = append(out.Capabilities, gp.Capabilities...)
	}

	if out.Runtime == "" {
		return Config{}, &ValidationError{
			Field: "runtime",
			Err:   ErrRuntimeRequired,
		}
	}

	// Golden-path runtimes are checked at catalog load, but an explicit --runtime
	// never passes through that check — so it is verified here against the same
	// declared set. A directory on disk that the catalog does not offer stays
	// deliberately unreachable.
	if _, ok := spec.Runtimes[out.Runtime]; !ok {
		return Config{}, &ValidationError{
			Field: "runtime",
			Value: out.Runtime,
			Err:   ErrUnknownRuntime,
		}
	}

	// Sort makes output deterministic; Compact then drops the duplicates
	// Sort has made adjacent.
	slices.Sort(out.Capabilities)
	out.Capabilities = slices.Compact(out.Capabilities)

	return out, nil
}
