package execute

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/berth-dev/berth/internal/beads"
)

// Integration tests for the execution loop components working together.
// Run with: go test -v ./internal/execute/... -run Integration

// TestIntegrationCheckpointSaveLoadRoundTrip tests that checkpoint state is
// correctly saved and loaded, verifying all fields survive the round-trip.
func TestIntegrationCheckpointSaveLoadRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a checkpoint with all fields populated.
	original := &Checkpoint{
		RunID:          "integration-test-run",
		CurrentBeadID:  "bt-5",
		CompletedBeads: []string{"bt-1", "bt-2", "bt-3"},
		FailedBeads:    []string{"bt-4"},
		RetryCount:     map[string]int{"bt-4": 3, "bt-5": 1},
		ConsecFailures: 2,
		LastError:      "verification failed: tests did not pass",
	}

	// Save checkpoint.
	if err := SaveCheckpoint(tmpDir, original); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	// Verify file was created.
	checkpointPath := filepath.Join(tmpDir, "checkpoint.json")
	if _, err := os.Stat(checkpointPath); os.IsNotExist(err) {
		t.Fatal("checkpoint.json was not created")
	}

	// Load checkpoint and verify all fields.
	loaded, err := LoadCheckpoint(tmpDir)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadCheckpoint returned nil")
	}

	// Verify all fields match.
	if loaded.RunID != original.RunID {
		t.Errorf("RunID = %q, want %q", loaded.RunID, original.RunID)
	}
	if loaded.CurrentBeadID != original.CurrentBeadID {
		t.Errorf("CurrentBeadID = %q, want %q", loaded.CurrentBeadID, original.CurrentBeadID)
	}
	if len(loaded.CompletedBeads) != len(original.CompletedBeads) {
		t.Errorf("CompletedBeads length = %d, want %d", len(loaded.CompletedBeads), len(original.CompletedBeads))
	}
	for i, bead := range loaded.CompletedBeads {
		if bead != original.CompletedBeads[i] {
			t.Errorf("CompletedBeads[%d] = %q, want %q", i, bead, original.CompletedBeads[i])
		}
	}
	if len(loaded.FailedBeads) != len(original.FailedBeads) {
		t.Errorf("FailedBeads length = %d, want %d", len(loaded.FailedBeads), len(original.FailedBeads))
	}
	if loaded.RetryCount["bt-4"] != original.RetryCount["bt-4"] {
		t.Errorf("RetryCount[bt-4] = %d, want %d", loaded.RetryCount["bt-4"], original.RetryCount["bt-4"])
	}
	if loaded.RetryCount["bt-5"] != original.RetryCount["bt-5"] {
		t.Errorf("RetryCount[bt-5] = %d, want %d", loaded.RetryCount["bt-5"], original.RetryCount["bt-5"])
	}
	if loaded.ConsecFailures != original.ConsecFailures {
		t.Errorf("ConsecFailures = %d, want %d", loaded.ConsecFailures, original.ConsecFailures)
	}
	if loaded.LastError != original.LastError {
		t.Errorf("LastError = %q, want %q", loaded.LastError, original.LastError)
	}
	if loaded.Timestamp.IsZero() {
		t.Error("Timestamp should be set after SaveCheckpoint")
	}
}

// TestIntegrationCheckpointPersistsAcrossResume simulates the resume scenario:
// save checkpoint, create ExecuteState from it, and verify state is correctly
// transferred as it would be during a berth resume operation.
func TestIntegrationCheckpointPersistsAcrossResume(t *testing.T) {
	tmpDir := t.TempDir()

	// Simulate execution state at time of interruption.
	checkpoint := &Checkpoint{
		RunID:          "resume-test-run",
		CurrentBeadID:  "bt-3",
		CompletedBeads: []string{"bt-1", "bt-2"},
		FailedBeads:    []string{},
		RetryCount:     map[string]int{"bt-3": 2},
		ConsecFailures: 1,
		LastError:      "last attempt failed",
	}

	// Save checkpoint (simulating interruption).
	if err := SaveCheckpoint(tmpDir, checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	// Load checkpoint (simulating resume start).
	loaded, err := LoadCheckpoint(tmpDir)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadCheckpoint returned nil")
	}

	// Create ExecuteState from checkpoint (as resume.go does).
	execState := &ExecuteState{
		RetryCount:     loaded.RetryCount,
		ConsecFailures: loaded.ConsecFailures,
	}

	// Verify ExecuteState has correct values.
	if execState.RetryCount["bt-3"] != 2 {
		t.Errorf("ExecuteState.RetryCount[bt-3] = %d, want 2", execState.RetryCount["bt-3"])
	}
	if execState.ConsecFailures != 1 {
		t.Errorf("ExecuteState.ConsecFailures = %d, want 1", execState.ConsecFailures)
	}

	// Verify circuit breaker can be initialized from ExecuteState.
	breaker := NewCircuitBreaker(3)
	breaker.SetConsecutiveFailures(execState.ConsecFailures)

	if breaker.GetConsecutiveFailures() != 1 {
		t.Errorf("CircuitBreaker.ConsecutiveFailures = %d, want 1", breaker.GetConsecutiveFailures())
	}
	if breaker.ShouldPause() {
		t.Error("CircuitBreaker should not be paused with 1 failure (threshold 3)")
	}
}

// TestIntegrationCircuitBreakerTriggersAtThreshold tests that the circuit
// breaker correctly pauses execution after reaching the configured threshold.
func TestIntegrationCircuitBreakerTriggersAtThreshold(t *testing.T) {
	threshold := 3
	breaker := NewCircuitBreaker(threshold)

	// Simulate consecutive failures.
	for i := 0; i < threshold-1; i++ {
		breaker.RecordFailure()
		if breaker.ShouldPause() {
			t.Errorf("ShouldPause should be false after %d failures (threshold %d)", i+1, threshold)
		}
	}

	// Record the final failure that triggers the breaker.
	breaker.RecordFailure()
	if !breaker.ShouldPause() {
		t.Errorf("ShouldPause should be true after %d failures", threshold)
	}

	// Verify the consecutive failures count.
	if breaker.GetConsecutiveFailures() != threshold {
		t.Errorf("ConsecutiveFailures = %d, want %d", breaker.GetConsecutiveFailures(), threshold)
	}
}

// TestIntegrationCircuitBreakerResetsOnSuccess tests that the circuit breaker
// correctly resets its failure count when a success is recorded.
func TestIntegrationCircuitBreakerResetsOnSuccess(t *testing.T) {
	breaker := NewCircuitBreaker(3)

	// Accumulate some failures (but not enough to trigger).
	breaker.RecordFailure()
	breaker.RecordFailure()

	if breaker.GetConsecutiveFailures() != 2 {
		t.Errorf("ConsecutiveFailures = %d, want 2", breaker.GetConsecutiveFailures())
	}

	// Record a success - should reset the counter.
	breaker.RecordSuccess()

	if breaker.GetConsecutiveFailures() != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0 after success", breaker.GetConsecutiveFailures())
	}
	if breaker.ShouldPause() {
		t.Error("ShouldPause should be false after success")
	}

	// Verify we need to accumulate failures from scratch.
	breaker.RecordFailure()
	if breaker.ShouldPause() {
		t.Error("ShouldPause should be false after only 1 failure post-reset")
	}
}

// TestIntegrationCircuitBreakerWithCheckpointRestore tests the full workflow
// of saving circuit breaker state via checkpoint, restoring it, and verifying
// the breaker behaves correctly after restore.
func TestIntegrationCircuitBreakerWithCheckpointRestore(t *testing.T) {
	tmpDir := t.TempDir()

	// Simulate a run with 2 consecutive failures.
	originalBreaker := NewCircuitBreaker(3)
	originalBreaker.RecordFailure()
	originalBreaker.RecordFailure()

	// Save checkpoint with breaker state.
	checkpoint := &Checkpoint{
		RunID:          "breaker-restore-test",
		CurrentBeadID:  "bt-3",
		CompletedBeads: []string{},
		FailedBeads:    []string{"bt-1", "bt-2"},
		RetryCount:     map[string]int{},
		ConsecFailures: originalBreaker.GetConsecutiveFailures(),
		LastError:      "second failure",
	}
	if err := SaveCheckpoint(tmpDir, checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	// Load checkpoint.
	loaded, err := LoadCheckpoint(tmpDir)
	if err != nil {
		t.Fatalf("LoadCheckpoint failed: %v", err)
	}

	// Create new breaker and restore state.
	restoredBreaker := NewCircuitBreaker(3)
	restoredBreaker.SetConsecutiveFailures(loaded.ConsecFailures)

	// Verify state was restored.
	if restoredBreaker.GetConsecutiveFailures() != 2 {
		t.Errorf("Restored ConsecutiveFailures = %d, want 2", restoredBreaker.GetConsecutiveFailures())
	}
	if restoredBreaker.ShouldPause() {
		t.Error("Restored breaker should not be paused (2 < 3)")
	}

	// One more failure should trigger the breaker.
	restoredBreaker.RecordFailure()
	if !restoredBreaker.ShouldPause() {
		t.Error("Breaker should be paused after 3rd failure")
	}
}

// TestIntegrationCloseReasonsAreCaptured tests that the ExtractSummary function
// correctly extracts close reasons from Claude output.
func TestIntegrationCloseReasonsAreCaptured(t *testing.T) {
	tests := []struct {
		name          string
		claudeOutput  string
		beadTitle     string
		wantContains  string
		wantMaxLength int
	}{
		{
			name:         "empty output falls back to title",
			claudeOutput: "",
			beadTitle:    "Add user authentication",
			wantContains: "Add user authentication",
		},
		{
			name:         "whitespace-only output falls back to title",
			claudeOutput: "   \n\t  ",
			beadTitle:    "Fix login bug",
			wantContains: "Fix login bug",
		},
		{
			name:         "short output is preserved",
			claudeOutput: "Successfully implemented JWT authentication with refresh tokens",
			beadTitle:    "Add auth",
			wantContains: "Successfully implemented JWT authentication",
		},
		{
			name:          "long output is truncated",
			claudeOutput:  string(make([]byte, 1000)), // 1000 characters
			beadTitle:     "Big task",
			wantMaxLength: 503, // maxSummaryLength (500) + "..."
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := beads.ExtractSummary(tc.claudeOutput, tc.beadTitle)

			if tc.wantContains != "" {
				if len(result) == 0 {
					t.Error("ExtractSummary returned empty string")
				}
				// Check contains for non-truncation tests
				if tc.wantMaxLength == 0 && !contains(result, tc.wantContains) {
					t.Errorf("ExtractSummary = %q, want to contain %q", result, tc.wantContains)
				}
			}

			if tc.wantMaxLength > 0 && len(result) > tc.wantMaxLength {
				t.Errorf("ExtractSummary length = %d, want <= %d", len(result), tc.wantMaxLength)
			}
		})
	}
}

// TestIntegrationExecuteStateInitialization tests that ExecuteState is
// correctly initialized from checkpoint data.
func TestIntegrationExecuteStateInitialization(t *testing.T) {
	checkpoint := &Checkpoint{
		RunID:          "state-init-test",
		CurrentBeadID:  "bt-5",
		CompletedBeads: []string{"bt-1", "bt-2", "bt-3", "bt-4"},
		FailedBeads:    []string{},
		RetryCount:     map[string]int{"bt-2": 1, "bt-3": 2, "bt-5": 3},
		ConsecFailures: 0,
		LastError:      "",
	}

	// Initialize ExecuteState as loop.go does.
	state := &ExecuteState{
		RetryCount:     checkpoint.RetryCount,
		ConsecFailures: checkpoint.ConsecFailures,
	}

	// Verify retry counts.
	if state.RetryCount["bt-2"] != 1 {
		t.Errorf("RetryCount[bt-2] = %d, want 1", state.RetryCount["bt-2"])
	}
	if state.RetryCount["bt-3"] != 2 {
		t.Errorf("RetryCount[bt-3] = %d, want 2", state.RetryCount["bt-3"])
	}
	if state.RetryCount["bt-5"] != 3 {
		t.Errorf("RetryCount[bt-5] = %d, want 3", state.RetryCount["bt-5"])
	}
	if state.ConsecFailures != 0 {
		t.Errorf("ConsecFailures = %d, want 0", state.ConsecFailures)
	}
}

// TestIntegrationCircuitBreakerConcurrentAccess tests that the circuit breaker
// is thread-safe when accessed concurrently.
func TestIntegrationCircuitBreakerConcurrentAccess(t *testing.T) {
	breaker := NewCircuitBreaker(100)
	var wg sync.WaitGroup

	// Spawn 50 goroutines recording failures.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			breaker.RecordFailure()
		}()
	}

	// Spawn 25 goroutines checking state.
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = breaker.ShouldPause()
			_ = breaker.GetConsecutiveFailures()
		}()
	}

	wg.Wait()

	// Should have exactly 50 failures recorded.
	if breaker.GetConsecutiveFailures() != 50 {
		t.Errorf("ConsecutiveFailures = %d, want 50", breaker.GetConsecutiveFailures())
	}
}

// TestIntegrationCheckpointClearedOnSuccess verifies that ClearCheckpoint
// removes the checkpoint file.
func TestIntegrationCheckpointClearedOnSuccess(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a checkpoint.
	checkpoint := &Checkpoint{
		RunID:          "clear-test",
		CompletedBeads: []string{"bt-1", "bt-2", "bt-3"},
		ConsecFailures: 0,
	}
	if err := SaveCheckpoint(tmpDir, checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint failed: %v", err)
	}

	// Verify it exists.
	checkpointPath := filepath.Join(tmpDir, "checkpoint.json")
	if _, err := os.Stat(checkpointPath); os.IsNotExist(err) {
		t.Fatal("checkpoint.json was not created")
	}

	// Clear it (as would happen on successful run completion).
	if err := ClearCheckpoint(tmpDir); err != nil {
		t.Fatalf("ClearCheckpoint failed: %v", err)
	}

	// Verify it's gone.
	if _, err := os.Stat(checkpointPath); !os.IsNotExist(err) {
		t.Error("checkpoint.json should be deleted after ClearCheckpoint")
	}

	// LoadCheckpoint should return nil, nil (not an error).
	loaded, err := LoadCheckpoint(tmpDir)
	if err != nil {
		t.Errorf("LoadCheckpoint after clear returned error: %v", err)
	}
	if loaded != nil {
		t.Error("LoadCheckpoint after clear should return nil")
	}
}

// contains checks if s contains substr.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
