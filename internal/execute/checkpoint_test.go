package execute

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckpointRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	cp := &Checkpoint{
		RunID:          "test-run",
		CurrentBeadID:  "bt-3",
		CompletedBeads: []string{"bt-1", "bt-2"},
		FailedBeads:    []string{},
		RetryCount:     map[string]int{"bt-3": 2},
		ConsecFailures: 1,
		LastError:      "some error",
	}

	err := SaveCheckpoint(tmpDir, cp)
	if err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	loaded, err := LoadCheckpoint(tmpDir)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadCheckpoint returned nil")
	}

	if loaded.RunID != cp.RunID {
		t.Errorf("RunID = %q, want %q", loaded.RunID, cp.RunID)
	}
	if loaded.CurrentBeadID != cp.CurrentBeadID {
		t.Errorf("CurrentBeadID = %q, want %q", loaded.CurrentBeadID, cp.CurrentBeadID)
	}
	if len(loaded.CompletedBeads) != len(cp.CompletedBeads) {
		t.Errorf("CompletedBeads = %v, want %v", loaded.CompletedBeads, cp.CompletedBeads)
	}
	for i, bead := range loaded.CompletedBeads {
		if bead != cp.CompletedBeads[i] {
			t.Errorf("CompletedBeads[%d] = %q, want %q", i, bead, cp.CompletedBeads[i])
		}
	}
	if loaded.RetryCount["bt-3"] != cp.RetryCount["bt-3"] {
		t.Errorf("RetryCount[bt-3] = %d, want %d", loaded.RetryCount["bt-3"], cp.RetryCount["bt-3"])
	}
	if loaded.ConsecFailures != cp.ConsecFailures {
		t.Errorf("ConsecFailures = %d, want %d", loaded.ConsecFailures, cp.ConsecFailures)
	}
	if loaded.LastError != cp.LastError {
		t.Errorf("LastError = %q, want %q", loaded.LastError, cp.LastError)
	}
	if loaded.Timestamp.IsZero() {
		t.Error("Timestamp should be set after SaveCheckpoint")
	}
}

func TestLoadCheckpointMissing(t *testing.T) {
	tmpDir := t.TempDir()
	cp, err := LoadCheckpoint(tmpDir)
	if err != nil {
		t.Errorf("LoadCheckpoint returned error for missing file: %v", err)
	}
	if cp != nil {
		t.Errorf("LoadCheckpoint returned non-nil for missing file: %+v", cp)
	}
}

func TestLoadCheckpointCorrupted(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "checkpoint.json")
	if err := os.WriteFile(path, []byte("invalid json"), 0644); err != nil {
		t.Fatalf("failed to write corrupted checkpoint: %v", err)
	}

	cp, err := LoadCheckpoint(tmpDir)
	if err == nil {
		t.Error("LoadCheckpoint should return error for corrupted file")
	}
	if cp != nil {
		t.Error("LoadCheckpoint should return nil for corrupted file")
	}
}

func TestClearCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	cp := &Checkpoint{RunID: "test"}
	if err := SaveCheckpoint(tmpDir, cp); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	err := ClearCheckpoint(tmpDir)
	if err != nil {
		t.Fatalf("ClearCheckpoint failed: %v", err)
	}

	loaded, _ := LoadCheckpoint(tmpDir)
	if loaded != nil {
		t.Error("checkpoint should be cleared")
	}
}

func TestClearCheckpointNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	err := ClearCheckpoint(tmpDir)
	if err != nil {
		t.Errorf("ClearCheckpoint should not error for non-existent file: %v", err)
	}
}

func TestCheckpointWithEmptyFields(t *testing.T) {
	tmpDir := t.TempDir()
	cp := &Checkpoint{
		RunID:          "minimal-run",
		CurrentBeadID:  "",
		CompletedBeads: nil,
		FailedBeads:    nil,
		RetryCount:     nil,
		ConsecFailures: 0,
		LastError:      "",
	}

	err := SaveCheckpoint(tmpDir, cp)
	if err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	loaded, err := LoadCheckpoint(tmpDir)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadCheckpoint returned nil")
	}
	if loaded.RunID != cp.RunID {
		t.Errorf("RunID = %q, want %q", loaded.RunID, cp.RunID)
	}
}

func TestCheckpointOverwrite(t *testing.T) {
	tmpDir := t.TempDir()

	// Save first checkpoint
	cp1 := &Checkpoint{
		RunID:          "run-1",
		CurrentBeadID:  "bt-1",
		CompletedBeads: []string{},
		ConsecFailures: 0,
	}
	if err := SaveCheckpoint(tmpDir, cp1); err != nil {
		t.Fatalf("SaveCheckpoint(1) failed: %v", err)
	}

	// Save second checkpoint (should overwrite)
	cp2 := &Checkpoint{
		RunID:          "run-1",
		CurrentBeadID:  "bt-2",
		CompletedBeads: []string{"bt-1"},
		ConsecFailures: 1,
	}
	if err := SaveCheckpoint(tmpDir, cp2); err != nil {
		t.Fatalf("SaveCheckpoint(2) failed: %v", err)
	}

	// Load and verify it's the second checkpoint
	loaded, err := LoadCheckpoint(tmpDir)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}
	if loaded.CurrentBeadID != "bt-2" {
		t.Errorf("CurrentBeadID = %q, want %q", loaded.CurrentBeadID, "bt-2")
	}
	if len(loaded.CompletedBeads) != 1 || loaded.CompletedBeads[0] != "bt-1" {
		t.Errorf("CompletedBeads = %v, want [bt-1]", loaded.CompletedBeads)
	}
	if loaded.ConsecFailures != 1 {
		t.Errorf("ConsecFailures = %d, want 1", loaded.ConsecFailures)
	}
}
