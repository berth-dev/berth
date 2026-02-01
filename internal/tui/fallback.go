// Package tui implements the terminal user interface using Bubble Tea.
package tui

import (
	"errors"
	"fmt"

	"github.com/berth-dev/berth/internal/config"
)

// ErrDescriptionRequired is returned when a task description is required but not provided.
var ErrDescriptionRequired = errors.New("task description required in non-interactive mode")

// FallbackRunner handles non-TTY execution by guiding users to CLI commands.
type FallbackRunner struct {
	cfg         *config.Config
	projectRoot string
}

// NewFallbackRunner creates a new FallbackRunner.
func NewFallbackRunner(cfg *config.Config, projectRoot string) *FallbackRunner {
	return &FallbackRunner{
		cfg:         cfg,
		projectRoot: projectRoot,
	}
}

// Run executes the fallback behavior for non-interactive mode.
// It prints guidance messages and returns an error if description is empty.
func (f *FallbackRunner) Run(description string) error {
	fmt.Println("Running in non-interactive mode...")

	if description == "" {
		return ErrDescriptionRequired
	}

	fmt.Printf("Task: %s\n", description)
	fmt.Printf("Use 'berth run \"%s\"' for full execution\n", description)

	return nil
}
