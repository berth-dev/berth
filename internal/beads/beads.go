// Package beads wraps the bd (beads) CLI for task management.
// This file provides bd create, bd ready, bd close, and related operations.
package beads

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Bead represents a single unit of work tracked by the beads system.
type Bead struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Status      string   `json:"status"` // "open", "in_progress", "done", "stuck"
	DependsOn   []string `json:"depends_on"`
	Files       []string `json:"files"`
}

// ErrBDNotInstalled is returned when the bd CLI is not found in PATH.
var ErrBDNotInstalled = errors.New("bd CLI not found in PATH; install beads first")

// ErrNoBead is returned when no beads are ready for execution.
var ErrNoBead = errors.New("no beads ready for execution")

// ensureBD checks that the bd CLI is available in PATH.
func ensureBD() error {
	_, err := exec.LookPath("bd")
	if err != nil {
		return ErrBDNotInstalled
	}
	return nil
}

// Create creates a new bead via `bd create` and returns its ID.
// It parses the bead ID from command output (e.g., "Created bead bt-a1b2c").
func Create(title, description string) (string, error) {
	if err := ensureBD(); err != nil {
		return "", err
	}

	cmd := exec.Command("bd", "create", "--title", title, "--type", "task", "--description", description)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("bd create failed: %w: %s", err, output)
	}

	// Parse the bead ID from output like "Created bead bt-a1b2c"
	line := strings.TrimSpace(string(output))
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return "", fmt.Errorf("bd create: unexpected output format: %s", line)
	}
	id := parts[len(parts)-1]
	return id, nil
}

// Ready returns the next unblocked bead ready for execution.
// Returns nil, nil if no bead is ready (all done or all blocked).
func Ready() (*Bead, error) {
	if err := ensureBD(); err != nil {
		return nil, err
	}

	cmd := exec.Command("bd", "ready", "--json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("bd ready failed: %w: %s", err, output)
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "null" || trimmed == "[]" {
		return nil, nil
	}

	var bead Bead
	if err := json.Unmarshal([]byte(trimmed), &bead); err != nil {
		return nil, fmt.Errorf("bd ready: failed to parse JSON: %w: %s", err, trimmed)
	}

	return &bead, nil
}

// Close marks a bead as completed with a reason.
func Close(id, reason string) error {
	if err := ensureBD(); err != nil {
		return err
	}

	cmd := exec.Command("bd", "close", id, "--reason", reason)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd close failed: %w: %s", err, output)
	}

	return nil
}

// UpdateStatus updates a bead's status.
func UpdateStatus(id, status string) error {
	if err := ensureBD(); err != nil {
		return err
	}

	cmd := exec.Command("bd", "update", id, "--status", status)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd update failed: %w: %s", err, output)
	}

	return nil
}

// AddDependency adds a dependency: child depends on parent.
func AddDependency(child, parent string) error {
	if err := ensureBD(); err != nil {
		return err
	}

	cmd := exec.Command("bd", "dep", "add", child, parent)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd dep add failed: %w: %s", err, output)
	}

	return nil
}

// List returns all beads in the current project.
func List() ([]Bead, error) {
	if err := ensureBD(); err != nil {
		return nil, err
	}

	cmd := exec.Command("bd", "list", "--json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("bd list failed: %w: %s", err, output)
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" || trimmed == "[]" {
		return nil, nil
	}

	var beads []Bead
	if err := json.Unmarshal([]byte(trimmed), &beads); err != nil {
		return nil, fmt.Errorf("bd list: failed to parse JSON: %w: %s", err, trimmed)
	}

	return beads, nil
}

// Init initializes the beads system in the current directory.
func Init() error {
	if err := ensureBD(); err != nil {
		return err
	}

	cmd := exec.Command("bd", "init")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd init failed: %w: %s", err, output)
	}

	return nil
}
