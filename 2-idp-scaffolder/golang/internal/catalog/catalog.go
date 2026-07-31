package catalog

import (
	"os"

	"gopkg.in/yaml.v3"
)

// GoldenPath represents a single paved-road configuration.
// The `yaml:"..."` tags tell the parser which YAML key maps to which field.
type GoldenPath struct {
	Name         string   `yaml:"name"`
	Runtime      string   `yaml:"runtime"`
	Capabilities []string `yaml:"capabilities"`
}

// Catalog represents the top-level structure of the YAML file.
type Catalog struct {
	GoldenPaths  []GoldenPath      `yaml:"golden-paths"`
	Destinations map[string]string `yaml:"destinations"`
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
func (c *Catalog) FindPath(name string) (*GoldenPath, bool) {
	// Index directly into the slice (Idiomatic & Safe across all Go versions)
	for i := range c.GoldenPaths {
		if c.GoldenPaths[i].Name == name {
			return &c.GoldenPaths[i], true // Returns pointer to the actual element in the catalog
		}
	}

	return nil, false // Not found
}
