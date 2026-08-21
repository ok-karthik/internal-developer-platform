package catalog

import (
	"fmt"
	"io/fs"
	"path"
	"slices"

	"gopkg.in/yaml.v3"
)

// Capability binds a friendly name to a version-pinned Terraform module.
type Capability struct {
	Module  string `yaml:"module"`
	Version string `yaml:"version"`
}

type Runtime struct {
	Description string `yaml:"description"`
}

// GoldenPath represents a single paved-road configuration.
// The `yaml:"..."` tags tell the parser which YAML key maps to which field.
type GoldenPath struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	Runtime      string   `yaml:"runtime"`
	Capabilities []string `yaml:"capabilities"`
}

// Catalog represents the top-level structure of the YAML file.
type Catalog struct {
	GoldenPaths            []GoldenPath          `yaml:"golden-paths"`
	Runtimes               map[string]Runtime    `yaml:"runtimes"`
	CapabilitiesSourceBase string                `yaml:"capabilities_source_base"`
	Capabilities           map[string]Capability `yaml:"capabilities"`
	Destinations           map[string]string     `yaml:"destinations"`
}

// requiredDestinations are the keys the renderer will look up. Missing any of them
// is a catalog authoring error, so we fail at load rather than mid-render.
var requiredDestinations = []string{
	"blueprints/team/apps",
	"blueprints/team/infra",
	"blueprints/team/gitops",
	"building-blocks/runtimes",
	"building-blocks/service-meta",
	"building-blocks/capabilities",
	"building-blocks/delivery/release",
}

// LoadCatalog parses catalog.yaml out of the catalog filesystem.
//
// It takes an fs.FS rather than a path so that validation can check declarations
// against the template tree they refer to — and so tests can supply an
// fstest.MapFS, the same seam Renderer.CatalogFS uses.
func LoadCatalog(catalogFS fs.FS) (*Catalog, error) {
	// 1. Read the raw bytes from the file
	data, err := fs.ReadFile(catalogFS, "catalog.yaml")
	if err != nil {
		return nil, fmt.Errorf("reading catalog.yaml: %w", err)
	}

	var catalog Catalog
	// 2. Unmarshal converts the raw YAML bytes into our Go struct
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("parsing catalog.yaml: %w", err)
	}

	// 3. Fail at load, where the error can still name the offending key, rather
	// than mid-render with half the tree already on disk.
	if err := catalog.validate(catalogFS); err != nil {
		return nil, fmt.Errorf("invalid catalog.yaml: %w", err)
	}

	return &catalog, nil
}

// FindPath is a helper method to easily look up a specific golden path by name.
func (c *Catalog) FindGoldenPath(name string) (*GoldenPath, bool) {
	// Index into the slice rather than taking the address of a range variable —
	// clearer about what the pointer refers to, and safe on every Go version.
	for i := range c.GoldenPaths {
		if c.GoldenPaths[i].Name == name {
			return &c.GoldenPaths[i], true // Returns pointer to the actual element in the catalog
		}
	}

	return nil, false // Not found
}

func (c *Catalog) validate(catalogFS fs.FS) error {
	if c.CapabilitiesSourceBase == "" {
		return fmt.Errorf("capabilities_source_base is not defined in the catalog")
	}

	// All destinations keys must exist
	for _, d := range requiredDestinations {
		if _, ok := c.Destinations[d]; !ok {
			return fmt.Errorf("missing destination key: %s", d)
		}
	}

	for _, gp := range c.GoldenPaths {
		if gp.Name == "" {
			return fmt.Errorf("found a golden-path with empty name")
		}
		if gp.Runtime == "" {
			return fmt.Errorf("golden-path '%s' is missing required field runtime", gp.Name)
		}

		// Check if runtime referenced under goldenpaths is valid
		if _, exists := c.Runtimes[gp.Runtime]; !exists {
			return fmt.Errorf("golden-path '%s' references unknown runtime '%s'", gp.Name, gp.Runtime)
		}

		// Check if capabilities referenced under goldenpaths are valid
		for _, name := range gp.Capabilities {
			if _, exists := c.Capabilities[name]; !exists {
				return fmt.Errorf("golden-path '%s' references unknown capability '%s'", gp.Name, name)
			}
		}
	}

	// Ensure every declared runtime has a directory in the template set.
	for name := range c.Runtimes {
		if _, err := fs.Stat(catalogFS, path.Join("building-blocks/runtimes", name)); err != nil {
			return fmt.Errorf("runtime %q is declared in the catalog but building-blocks/runtimes/%s/ does not exist", name, name)
		}
	}

	return nil
}

func (c *Catalog) GetGoldenPathNames() []string {
	s := make([]string, 0, len(c.GoldenPaths))
	for _, gp := range c.GoldenPaths {
		s = append(s, gp.Name)
	}
	return s
}

// GetRuntimeNames lists the offered runtimes. Sorted, because it ends up in
// error messages and CLI help where map iteration order would make the output
// change between runs.
func (c *Catalog) GetRuntimeNames() []string {
	s := make([]string, 0, len(c.Runtimes))
	for name := range c.Runtimes {
		s = append(s, name)
	}
	slices.Sort(s)
	return s
}

func (c *Catalog) GetCapabilityNames() []string {
	s := make([]string, 0, len(c.Capabilities))
	for capName := range c.Capabilities {
		s = append(s, capName)
	}
	return s
}
