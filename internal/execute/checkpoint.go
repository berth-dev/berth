// Package execute implements the bead execution loop.
package execute

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Checkpoint represents the execution state for resume capability.
type Checkpoint struct {
	RunID          string         `json:"run_id"`
	CurrentBeadID  string         `json:"current_bead_id"`
	CompletedBeads []string       `json:"completed_beads"`
	FailedBeads    []string       `json:"failed_beads"`
	RetryCount     map[string]int `json:"retry_count"`     // per-bead retry counts
	ConsecFailures int            `json:"consec_failures"` // for circuit breaker
	LastError      string         `json:"last_error,omitempty"`
	Timestamp      time.Time      `json:"timestamp"`
}

// SaveCheckpoint writes the current state to disk.
func SaveCheckpoint(runDir string, cp *Checkpoint) error {
	cp.Timestamp = time.Now()
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling checkpoint: %w", err)
	}
	path := filepath.Join(runDir, "checkpoint.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing checkpoint: %w", err)
	}
	return nil
}

// LoadCheckpoint reads the checkpoint from disk.
// Returns nil, nil if no checkpoint exists (not an error).
func LoadCheckpoint(runDir string) (*Checkpoint, error) {
	path := filepath.Join(runDir, "checkpoint.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil // No checkpoint is fine
	}
	if err != nil {
		return nil, fmt.Errorf("reading checkpoint: %w", err)
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("parsing checkpoint: %w", err)
	}
	return &cp, nil
}

// ClearCheckpoint removes the checkpoint file.
func ClearCheckpoint(runDir string) error {
	path := filepath.Join(runDir, "checkpoint.json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing checkpoint: %w", err)
	}
	return nil
}
