// retry.go implements the 3+1 retry strategy with diagnostic escalation.
package execute

import (
	"fmt"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/config"
	berthcontext "github.com/berth-dev/berth/internal/context"
	"github.com/berth-dev/berth/internal/graph"
	"github.com/berth-dev/berth/internal/log"
	"github.com/berth-dev/berth/prompts"
)

// maxBlindRetries is the number of retries before escalating to diagnostic.
const maxBlindRetries = 3

// BeadResult contains the outcome of a bead execution attempt.
type BeadResult struct {
	Passed       bool   // Whether verification passed
	ClaudeOutput string // Claude's output text (for close reason)
}

// RetryBead implements the "3+1" retry strategy for a single bead:
//
//  1. Attempts 1-3 (blind retries): build prompt, spawn Claude, verify.
//     If verification passes, return true immediately.
//     If it fails, collect the error and continue.
//
//  2. Attempt 4 (diagnostic retry): run a diagnostic analysis on the
//     accumulated errors, then retry with the diagnosis included in the
//     prompt. If verification passes, return true. Otherwise return false,
//     signaling the bead is stuck and the caller should handle escalation.
//
// Returns BeadResult with the outcome and Claude's output text for close reasons.
func RetryBead(
	cfg config.Config,
	bead *beads.Bead,
	graphData string,
	projectRoot string,
	logger *log.Logger,
	kgClient *graph.Client,
	opts *SpawnClaudeOpts,
) (*BeadResult, error) {
	learnings := berthcontext.ReadLearnings(projectRoot)
	systemPrompt := prompts.ExecutorSystemPrompt
	if opts != nil && opts.SystemPrompt != "" {
		systemPrompt = opts.SystemPrompt
	}

	var collectedErrors []string

	// Phase 1: blind retries (attempts 1-3).
	for attempt := 1; attempt <= maxBlindRetries; attempt++ {
		taskPrompt := BuildExecutorPrompt(bead, attempt, nil, graphData, learnings)

		output, err := SpawnClaude(cfg, systemPrompt, taskPrompt, projectRoot, opts)
		if err != nil {
			collectedErrors = append(collectedErrors, fmt.Sprintf("spawn error (attempt %d): %v", attempt, err))
			logRetry(logger, bead, attempt, fmt.Sprintf("spawn error: %v", err))
			continue
		}

		if output.IsError {
			collectedErrors = append(collectedErrors, fmt.Sprintf("claude error (attempt %d): %s", attempt, output.Result))
			logRetry(logger, bead, attempt, output.Result)
			continue
		}

		workDir := ""
		if opts != nil {
			workDir = opts.WorkDir
		}
		result, err := RunVerification(cfg, bead, workDir)
		if err != nil {
			collectedErrors = append(collectedErrors, fmt.Sprintf("verify error (attempt %d): %v", attempt, err))
			logRetry(logger, bead, attempt, fmt.Sprintf("verify error: %v", err))
			continue
		}

		if result.Passed {
			logVerifyPassed(logger, bead, attempt)
			return &BeadResult{Passed: true, ClaudeOutput: output.Result}, nil
		}

		// Verification failed: collect the error output.
		errMsg := fmt.Sprintf("verify failed at '%s' (attempt %d):\n%s", result.FailedStep, attempt, result.Output)
		collectedErrors = append(collectedErrors, errMsg)
		logVerifyFailed(logger, bead, attempt, result.FailedStep, result.Output)
	}

	// Phase 2: diagnostic retry (attempt 4).
	logDiagnosing(logger, bead)

	diagnosis, err := RunDiagnostic(cfg, bead, collectedErrors, projectRoot)
	if err != nil {
		return &BeadResult{Passed: false}, fmt.Errorf("diagnostic failed for bead %s: %w", bead.ID, err)
	}

	taskPrompt := BuildExecutorPrompt(bead, maxBlindRetries+1, &diagnosis, graphData, learnings)

	output, err := SpawnClaude(cfg, systemPrompt, taskPrompt, projectRoot, opts)
	if err != nil {
		return &BeadResult{Passed: false}, fmt.Errorf("diagnostic spawn failed for bead %s: %w", bead.ID, err)
	}

	if output.IsError {
		return &BeadResult{Passed: false, ClaudeOutput: output.Result}, nil
	}

	workDir := ""
	if opts != nil {
		workDir = opts.WorkDir
	}
	result, err := RunVerification(cfg, bead, workDir)
	if err != nil {
		return &BeadResult{Passed: false, ClaudeOutput: output.Result}, fmt.Errorf("post-diagnostic verify failed for bead %s: %w", bead.ID, err)
	}

	if result.Passed {
		logVerifyPassed(logger, bead, maxBlindRetries+1)
		return &BeadResult{Passed: true, ClaudeOutput: output.Result}, nil
	}

	logVerifyFailed(logger, bead, maxBlindRetries+1, result.FailedStep, result.Output)
	return &BeadResult{Passed: false, ClaudeOutput: output.Result}, nil
}

// logRetry logs a task_retry event.
func logRetry(logger *log.Logger, bead *beads.Bead, attempt int, reason string) {
	if logger == nil {
		return
	}
	_ = logger.Append(log.LogEvent{
		Event:   log.EventTaskRetry,
		BeadID:  bead.ID,
		Title:   bead.Title,
		Attempt: attempt,
		Reason:  reason,
	})
}

// logVerifyPassed logs a verify_passed event.
func logVerifyPassed(logger *log.Logger, bead *beads.Bead, attempt int) {
	if logger == nil {
		return
	}
	_ = logger.Append(log.LogEvent{
		Event:   log.EventVerifyPassed,
		BeadID:  bead.ID,
		Title:   bead.Title,
		Attempt: attempt,
	})
}

// logVerifyFailed logs a verify_failed event.
func logVerifyFailed(logger *log.Logger, bead *beads.Bead, attempt int, step string, output string) {
	if logger == nil {
		return
	}
	_ = logger.Append(log.LogEvent{
		Event:   log.EventVerifyFailed,
		BeadID:  bead.ID,
		Title:   bead.Title,
		Attempt: attempt,
		Step:    step,
		Error:   output,
	})
}

// logDiagnosing logs a task_diagnosing event. We use the Data field
// since there is no dedicated event constant for diagnosing.
func logDiagnosing(logger *log.Logger, bead *beads.Bead) {
	if logger == nil {
		return
	}
	_ = logger.Append(log.LogEvent{
		Event:  log.EventTaskRetry,
		BeadID: bead.ID,
		Title:  bead.Title,
		Data:   map[string]interface{}{"phase": "diagnosing"},
	})
}
