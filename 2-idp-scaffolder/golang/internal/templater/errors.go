package templater

import (
	"errors"
	"fmt"
)

// Sentinel errors for validation failures
var (
	ErrUnknownCapability = errors.New("unknown capability")
	ErrUnknownRuntime    = errors.New("unknown runtime")
	ErrUnknownGoldenPath = errors.New("golden path not found")
	ErrRuntimeRequired   = errors.New("a runtime is required")
)

// ValidationError provides structured metadata about which field failed validation.
type ValidationError struct {
	Field string // "capability", "runtime", "golden-path"
	Value string // the offending input value (if any)
	Err   error  // the sentinel error above
}

func (e *ValidationError) Error() string {
	if e.Value != "" {
		return fmt.Sprintf("%s %q: %v", e.Field, e.Value, e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Field, e.Err)
}

func (e *ValidationError) Unwrap() error {
	return e.Err
}
