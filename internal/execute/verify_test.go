package execute

import (
	"testing"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/config"
)

func TestBuildPipelineWithSecurity(t *testing.T) {
	cfg := config.Config{
		VerifyPipeline: []string{"go test ./...", "golangci-lint run"},
		Verify: config.VerifyConfig{
			Security: "gosec ./...",
		},
	}
	bead := &beads.Bead{}

	pipeline := buildPipeline(cfg, bead)

	if len(pipeline) != 3 {
		t.Errorf("expected 3 steps in pipeline, got %d", len(pipeline))
	}
	// Security scan should be last
	if pipeline[2] != "gosec ./..." {
		t.Errorf("expected security scan as last step, got %s", pipeline[2])
	}
}

func TestBuildPipelineWithoutSecurity(t *testing.T) {
	cfg := config.Config{
		VerifyPipeline: []string{"go test ./...", "golangci-lint run"},
		Verify: config.VerifyConfig{
			Security: "", // not configured
		},
	}
	bead := &beads.Bead{}

	pipeline := buildPipeline(cfg, bead)

	if len(pipeline) != 2 {
		t.Errorf("expected 2 steps in pipeline (no security), got %d", len(pipeline))
	}
}

func TestBuildPipelineSecurityAfterBeadExtras(t *testing.T) {
	cfg := config.Config{
		VerifyPipeline: []string{"go test ./..."},
		Verify: config.VerifyConfig{
			Security: "gosec ./...",
		},
	}
	bead := &beads.Bead{
		VerifyExtra: []string{"go vet ./..."},
	}

	pipeline := buildPipeline(cfg, bead)

	if len(pipeline) != 3 {
		t.Errorf("expected 3 steps in pipeline, got %d", len(pipeline))
	}
	// Order should be: default pipeline, bead extras, security
	expected := []string{"go test ./...", "go vet ./...", "gosec ./..."}
	for i, step := range pipeline {
		if step != expected[i] {
			t.Errorf("pipeline[%d] = %s, want %s", i, step, expected[i])
		}
	}
}

func TestRunVerificationSecurityFailure(t *testing.T) {
	cfg := config.Config{
		VerifyPipeline: []string{"true"}, // always passes
		Verify: config.VerifyConfig{
			Security: "false", // always fails
		},
	}
	bead := &beads.Bead{}

	result, err := RunVerification(cfg, bead, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Passed {
		t.Error("expected verification to fail due to security scan")
	}
	if result.FailedStep != "false" {
		t.Errorf("expected FailedStep to be 'false', got %s", result.FailedStep)
	}
}

func TestRunVerificationSecuritySuccess(t *testing.T) {
	cfg := config.Config{
		VerifyPipeline: []string{"true"},
		Verify: config.VerifyConfig{
			Security: "true", // always passes
		},
	}
	bead := &beads.Bead{}

	result, err := RunVerification(cfg, bead, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected verification to pass, but failed at step: %s", result.FailedStep)
	}
}

func TestRunVerificationEmptySecuritySkipped(t *testing.T) {
	cfg := config.Config{
		VerifyPipeline: []string{"echo hello"},
		Verify: config.VerifyConfig{
			Security: "", // empty, should be skipped
		},
	}
	bead := &beads.Bead{}

	result, err := RunVerification(cfg, bead, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Passed {
		t.Errorf("expected verification to pass, but failed at step: %s", result.FailedStep)
	}
}
