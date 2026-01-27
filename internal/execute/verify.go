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
func RunVerification(cfg config.Config, bead *beads.Bead) (*VerifyResult, error) {
	pipeline := buildPipeline(cfg, bead)
	if len(pipeline) == 0 {
		return &VerifyResult{
			Passed:    true,
			AllOutput: "(no verification commands configured)",
		}, nil
	}

	var allOutput strings.Builder

	for _, step := range pipeline {
		stepOutput, err := runStep(step)

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
// extra verification commands. The default pipeline runs first, followed
// by bead-specific extras.
func buildPipeline(cfg config.Config, bead *beads.Bead) []string {
	pipeline := make([]string, 0, len(cfg.VerifyPipeline))
	pipeline = append(pipeline, cfg.VerifyPipeline...)

	// The Bead struct uses the Files field; there is no verify_extra
	// field on the struct currently. If bead-specific commands are
	// added later, they would be appended here.
	_ = bead

	return pipeline
}

// runStep executes a single shell command and returns the combined
// stdout+stderr output. Returns a non-nil error if the command exits
// with a non-zero status.
func runStep(command string) (string, error) {
	cmd := exec.Command("sh", "-c", command)

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	output := buf.String()

	return output, err
}
