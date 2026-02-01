// verify.go runs the verification pipeline after bead execution.
package execute

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/config"
)

// VerifyResult holds the outcome of a verification pipeline run.
type VerifyResult struct {
	Passed     bool
	FailedStep string
	Output     string // Output from the failed step (empty if passed)
	AllOutput  string // All verification output combined
}

// RunVerification executes the verification pipeline commands in order.
// It combines the default pipeline from config.VerifyPipeline with any
// per-bead verify_extra commands. Execution stops on the first failure.
// Pass an empty workDir to run in the current directory.
func RunVerification(cfg config.Config, bead *beads.Bead, workDir string) (*VerifyResult, error) {
	pipeline := buildPipeline(cfg, bead)
	if len(pipeline) == 0 {
		return &VerifyResult{
			Passed:    true,
			AllOutput: "(no verification commands configured)",
		}, nil
	}

	var allOutput strings.Builder

	for _, step := range pipeline {
		stepOutput, err := runStep(step, workDir)

		allOutput.WriteString(fmt.Sprintf("=== %s ===\n", step))
		allOutput.WriteString(stepOutput)
		allOutput.WriteString("\n")

		if err != nil {
			return &VerifyResult{
				Passed:     false,
				FailedStep: step,
				Output:     stepOutput,
				AllOutput:  allOutput.String(),
			}, nil
		}
	}

	return &VerifyResult{
		Passed:    true,
		AllOutput: allOutput.String(),
	}, nil
}

// buildPipeline combines the default verify pipeline with any per-bead
// extra verification commands, and optionally the security scan command.
// The default pipeline runs first, followed by bead-specific extras, and
// finally the security scan (if configured).
func buildPipeline(cfg config.Config, bead *beads.Bead) []string {
	pipeline := make([]string, 0, len(cfg.VerifyPipeline))
	pipeline = append(pipeline, cfg.VerifyPipeline...)

	if len(bead.VerifyExtra) > 0 {
		pipeline = append(pipeline, bead.VerifyExtra...)
	}

	// Add security scan if configured (runs last, after lint/test)
	if cfg.Verify.Security != "" {
		pipeline = append(pipeline, cfg.Verify.Security)
	}

	return pipeline
}

// runStep executes a single shell command and returns the combined
// stdout+stderr output. Returns a non-nil error if the command exits
// with a non-zero status. If workDir is non-empty, the command runs
// in that directory.
func runStep(command string, workDir string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	if workDir != "" {
		cmd.Dir = workDir
	}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	output := buf.String()

	return output, err
}
