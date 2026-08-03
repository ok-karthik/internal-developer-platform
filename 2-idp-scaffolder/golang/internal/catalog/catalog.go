package catalog

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Capability binds a friendly name to a version-pinned Terraform module.
type Capability struct {
	Module  string `yaml:"module"`
	Version string `yaml:"version"`
}

// GoldenPath represents a single paved-road configuration.
// The `yaml:"..."` tags tell the parser which YAML key maps to which field.
type GoldenPath struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	Runtime      string   `yaml:"runtime"`
	Capabilities []string `yaml:"capabilities"`
	Delivery     string   `yaml:"delivery"`
}

// Catalog represents the top-level structure of the YAML file.
type Catalog struct {
	CapabilitiesSourceBase string                `yaml:"capabilities_source_base"`
	GoldenPaths            []GoldenPath          `yaml:"golden-paths"`
	Capabilities           map[string]Capability `yaml:"capabilities"`
	Destinations           map[string]string     `yaml:"destinations"`
}

// requiredDestinations are the keys the renderer will look up. Missing any of them
// is a catalog authoring error, so we fail at load rather than mid-render.
var requiredDestinations = []string{
	"blueprints/team/gitops",
	"blueprints/team/infra",
	"blueprints/system/gitops",
	"building-blocks/runtimes",
	"building-blocks/service-meta",
	"building-blocks/capabilities",
	"building-blocks/delivery/release",
}

// LoadCatalog reads the YAML file from disk and parses it into our Structs.
func LoadCatalog(filePath string) (*Catalog, error) {
	// 1. Read the raw bytes from the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err // Return the error if file doesn't exist
	}

	var catalog Catalog
	// 2. Unmarshal converts the raw YAML bytes into our Go struct
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, err
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

func (c *Catalog) validate() error {
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

		for _, name := range gp.Capabilities {
			if _, exists := c.Capabilities[name]; !exists {
				return fmt.Errorf("golden-path '%s' references unknown capability '%s'", gp.Name, name)
			}
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

func (c *Catalog) GetCapabilityNames() []string {
	s := make([]string, 0, len(c.Capabilities))
	for capName := range c.Capabilities {
		s = append(s, capName)
	}
	return s
}
