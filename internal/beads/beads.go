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
	VerifyExtra []string `json:"verify_extra,omitempty"`
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

	// Parse the bead ID from output like "✓ Created issue: testproject-20g"
	// The ID is on the first line after "Created issue:"
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Created issue:") {
			parts := strings.SplitN(line, "Created issue:", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}
	return "", fmt.Errorf("bd create: could not parse bead ID from output: %s", string(output))
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

	// bd ready --json returns an array; try array first, then single object
	var beadList []Bead
	if err := json.Unmarshal([]byte(trimmed), &beadList); err == nil {
		if len(beadList) == 0 {
			return nil, nil
		}
		return &beadList[0], nil
	}

	var bead Bead
	if err := json.Unmarshal([]byte(trimmed), &bead); err != nil {
		return nil, fmt.Errorf("bd ready: failed to parse JSON: %w: %s", err, trimmed)
	}

	return &bead, nil
}

// ReadyAll returns all unblocked beads ready for execution.
// Returns nil, nil if no beads are ready (all done or all blocked).
func ReadyAll() ([]*Bead, error) {
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

	// bd ready --json returns an array; try array first, then single object.
	var beadList []Bead
	if err := json.Unmarshal([]byte(trimmed), &beadList); err == nil {
		if len(beadList) == 0 {
			return nil, nil
		}
		result := make([]*Bead, len(beadList))
		for i := range beadList {
			result[i] = &beadList[i]
		}
		return result, nil
	}

	var bead Bead
	if err := json.Unmarshal([]byte(trimmed), &bead); err != nil {
		return nil, fmt.Errorf("bd ready: failed to parse JSON: %w: %s", err, trimmed)
	}

	return []*Bead{&bead}, nil
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

// List returns all open/in-progress beads in the current project.
func List() ([]Bead, error) {
	return listBeads(false)
}

// ListAll returns all beads including closed ones.
func ListAll() ([]Bead, error) {
	return listBeads(true)
}

func listBeads(includeAll bool) ([]Bead, error) {
	if err := ensureBD(); err != nil {
		return nil, err
	}

	args := []string{"list", "--json", "--limit", "0"}
	if includeAll {
		args = append(args, "--all")
	}
	cmd := exec.Command("bd", args...)
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
// Uses --stealth mode so beads files are excluded via .git/info/exclude
// instead of polluting the user's working tree with AGENTS.md and .gitattributes.
func Init() error {
	if err := ensureBD(); err != nil {
		return err
	}

	cmd := exec.Command("bd", "init", "--stealth", "--skip-hooks")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bd init failed: %w: %s", err, output)
	}

	return nil
}

// maxSummaryLength is the maximum character length for a close reason summary.
const maxSummaryLength = 500

// ExtractSummary extracts a meaningful summary from Claude's output for the
// close reason. It attempts to find a summary in the output text, truncating
// if necessary. Falls back to the bead title if no output is available.
func ExtractSummary(claudeOutput string, beadTitle string) string {
	// Fall back to title if no output.
	if strings.TrimSpace(claudeOutput) == "" {
		return "Completed: " + beadTitle
	}

	summary := strings.TrimSpace(claudeOutput)

	// Truncate if too long, preserving word boundaries where possible.
	if len(summary) > maxSummaryLength {
		truncated := summary[:maxSummaryLength]
		// Try to break at a word boundary (last space within limit).
		if lastSpace := strings.LastIndex(truncated, " "); lastSpace > maxSummaryLength/2 {
			truncated = truncated[:lastSpace]
		}
		summary = truncated + "..."
	}

	return summary
}
