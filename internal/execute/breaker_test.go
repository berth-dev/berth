package execute

import (
	"sync"
	"testing"
)

func TestCircuitBreakerTriggered(t *testing.T) {
	cb := NewCircuitBreaker(3)
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.ShouldPause() {
		t.Error("ShouldPause should be false after 2 failures (threshold is 3)")
	}
	cb.RecordFailure()
	if !cb.ShouldPause() {
		t.Error("ShouldPause should be true after 3 failures")
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	cb := NewCircuitBreaker(3)
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess()
	cb.RecordFailure()
	if cb.ShouldPause() {
		t.Error("ShouldPause should be false after success reset")
	}
}

func TestCircuitBreakerCustomThreshold(t *testing.T) {
	cb := NewCircuitBreaker(5)
	for i := 0; i < 4; i++ {
		cb.RecordFailure()
		if cb.ShouldPause() {
			t.Errorf("ShouldPause should be false after %d failures (threshold is 5)", i+1)
		}
	}
	cb.RecordFailure()
	if !cb.ShouldPause() {
		t.Error("ShouldPause should be true after 5 failures")
	}
}

func TestCircuitBreakerConcurrent(t *testing.T) {
	cb := NewCircuitBreaker(100)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cb.RecordFailure()
		}()
	}
	wg.Wait()
	// Should not panic, and count should be 50
	if cb.GetConsecutiveFailures() != 50 {
		t.Errorf("ConsecutiveFailures = %d, want 50", cb.GetConsecutiveFailures())
	}
}

func TestCircuitBreakerDefaultThreshold(t *testing.T) {
	// Test that invalid threshold defaults to 3
	cb := NewCircuitBreaker(0)
	if cb.Threshold != 3 {
		t.Errorf("Threshold = %d, want 3 for zero input", cb.Threshold)
	}

	cb = NewCircuitBreaker(-1)
	if cb.Threshold != 3 {
		t.Errorf("Threshold = %d, want 3 for negative input", cb.Threshold)
	}
}

func TestCircuitBreakerResetMethod(t *testing.T) {
	cb := NewCircuitBreaker(3)
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	if !cb.ShouldPause() {
		t.Error("ShouldPause should be true after 3 failures")
	}

	cb.Reset()
	if cb.ShouldPause() {
		t.Error("ShouldPause should be false after Reset")
	}
	if cb.GetConsecutiveFailures() != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0 after Reset", cb.GetConsecutiveFailures())
	}
}

func TestCircuitBreakerSetConsecutiveFailures(t *testing.T) {
	cb := NewCircuitBreaker(3)

	// Set below threshold
	cb.SetConsecutiveFailures(2)
	if cb.GetConsecutiveFailures() != 2 {
		t.Errorf("ConsecutiveFailures = %d, want 2", cb.GetConsecutiveFailures())
	}
	if cb.ShouldPause() {
		t.Error("ShouldPause should be false when count below threshold")
	}

	// Set at threshold
	cb.SetConsecutiveFailures(3)
	if !cb.ShouldPause() {
		t.Error("ShouldPause should be true when count equals threshold")
	}

	// Set above threshold
	cb.SetConsecutiveFailures(5)
	if !cb.ShouldPause() {
		t.Error("ShouldPause should be true when count above threshold")
	}

	// Set back below threshold
	cb.SetConsecutiveFailures(1)
	if cb.ShouldPause() {
		t.Error("ShouldPause should be false after setting count below threshold")
	}
}

func TestCircuitBreakerSuccessResetsPaused(t *testing.T) {
	cb := NewCircuitBreaker(2)
	cb.RecordFailure()
	cb.RecordFailure()
	if !cb.ShouldPause() {
		t.Error("ShouldPause should be true after reaching threshold")
	}

	cb.RecordSuccess()
	if cb.ShouldPause() {
		t.Error("ShouldPause should be false after RecordSuccess")
	}
	if cb.GetConsecutiveFailures() != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0 after RecordSuccess", cb.GetConsecutiveFailures())
	}
}
