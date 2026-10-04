package templater

import (
	"fmt"
	"os"
	"path/filepath"
)

// Writer is the single seam through which Renderer touches the filesystem.
// It allows switching between real disk writes, --dry-run, and in-memory test mocks.
type Writer interface {
	WriteFile(path string, content []byte) error
	MkdirAll(path string) error
}

// OSWriter performs real filesystem operations.
type OSWriter struct{}

func (OSWriter) MkdirAll(path string) error {
	return os.MkdirAll(path, 0755)
}
func (OSWriter) WriteFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0644)
}

// DryRunWriter reports what would happen without touching the filesystem.
type DryRunWriter struct{}

func (DryRunWriter) MkdirAll(path string) error {
	return nil
}
func (DryRunWriter) WriteFile(path string, content []byte) error {
	fmt.Printf("[WOULD WRITE] %s (%d bytes)\n", path, len(content))
	return nil
}
