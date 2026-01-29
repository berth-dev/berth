// Package execute implements Phase 3: the main execution loop that processes beads.
// This file contains the top-level execution orchestrator.
package execute

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/config"
	berthcontext "github.com/berth-dev/berth/internal/context"
	"github.com/berth-dev/berth/internal/git"
	"github.com/berth-dev/berth/internal/graph"
	"github.com/berth-dev/berth/internal/log"
	"github.com/berth-dev/berth/prompts"
)

// StuckAction constants for the Action field in the StuckAction struct
// defined in stuck.go.
const (
	stuckActionHint   = "hint"
	stuckActionRescue = "rescue"
	stuckActionSkip   = "skip"
	stuckActionAbort  = "abort"
)

// ExecuteState holds checkpoint-related state for the execution loop.
// This is used to restore state on resume.
type ExecuteState struct {
	RetryCount     map[string]int // per-bead retry counts
	ConsecFailures int            // consecutive failures for circuit breaker
}

// RunExecute is the main execution entry point. It creates a feature branch,
// starts the KG MCP server, and processes beads one at a time through the
// retry loop until all beads are completed, stuck, or skipped.
// If parallel mode is active, delegates to RunExecuteParallel.
func RunExecute(cfg config.Config, projectRoot string, runDir string, branchName string, verbose bool) error {
	return RunExecuteWithState(cfg, projectRoot, runDir, branchName, verbose, nil)
}

// RunExecuteWithState is the main execution entry point that accepts optional
// restored state from a checkpoint. Used by resume to restore execution state.
func RunExecuteWithState(cfg config.Config, projectRoot string, runDir string, branchName string, verbose bool, state *ExecuteState) error {
	// Check if parallel execution is appropriate.
	allBeadsList, err := beads.List()
	if err != nil {
		return fmt.Errorf("listing beads for mode check: %w", err)
	}
	if ShouldRunParallel(cfg, allBeadsList) {
		fmt.Println("Parallel mode enabled")
		return RunExecuteParallel(cfg, projectRoot, runDir, branchName, allBeadsList, verbose)
	}

	// 1. Create a git branch for this execution run.
	// If the repo has no commits, create an initial empty commit first
	// so we have something to branch from.
	if err := git.EnsureInitialCommit(); err != nil {
		return fmt.Errorf("ensuring initial commit: %w", err)
	}
	if err := git.CreateBranch(branchName); err != nil {
		// Branch may already exist; try switching to it.
		if switchErr := git.SwitchBranch(branchName); switchErr != nil {
			return fmt.Errorf("creating or switching to branch %s: %w", branchName, err)
		}
	}

	// 2. Read the system prompt from .berth/CLAUDE.md.
	systemPrompt, err := readSystemPrompt(projectRoot)
	if err != nil {
		// Fall back to the embedded default executor system prompt.
		systemPrompt = prompts.ExecutorSystemPrompt
	}

	// 3. Start or ensure KG MCP is alive.
	var kgClient *graph.Client
	if cfg.KnowledgeGraph.Enabled != "never" {
		kgClient, err = graph.EnsureMCPAlive(projectRoot, cfg.KnowledgeGraph, nil)
		if err != nil {
			// KG is best-effort; log the error but continue without it.
			fmt.Fprintf(os.Stderr, "Warning: KG MCP unavailable: %v\n", err)
			kgClient = nil
		}
	}
	// Ensure the KG client is cleaned up on exit.
	// Use a closure so the defer evaluates kgClient at function-exit time,
	// not at defer-registration time. This handles reassignment inside the loop.
	defer func() {
		if kgClient != nil {
			kgClient.Close()
		}
	}()

	// 4. Count total beads for progress tracking.
	allBeads, err := beads.List()
	if err != nil {
		return fmt.Errorf("listing beads: %w", err)
	}
	pool := NewExecutionPool(len(allBeads))

	// 4a. Initialize checkpoint tracking state.
	// If we have restored state from a checkpoint, use it; otherwise start fresh.
	retryCount := make(map[string]int)
	completedBeads := []string{}
	failedBeads := []string{}

	// 4b. Initialize circuit breaker with threshold from config.
	breaker := NewCircuitBreaker(cfg.Execution.CircuitBreakerThreshold)
	if state != nil {
		if state.RetryCount != nil {
			retryCount = state.RetryCount
		}
		// Restore circuit breaker state from checkpoint.
		breaker.SetConsecutiveFailures(state.ConsecFailures)
	}

	// 5. Print header.
	fmt.Printf("Executing %d beads on branch %s\n", pool.Total, branchName)

	// 6. Create logger.
	logger, err := log.NewLogger(projectRoot)
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}

	// 7. Log run_started.
	if logErr := logger.Append(log.LogEvent{
		Event:  log.EventRunStarted,
		Branch: branchName,
		Beads:  pool.Total,
	}); logErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to log run_started: %v\n", logErr)
	}

	// 8. Main loop: process beads one at a time.
	for {
		task, err := beads.Ready()
		if err != nil {
			return fmt.Errorf("getting next ready bead: %w", err)
		}
		if task == nil {
			// No more unblocked beads; we are done.
			break
		}

		// Load sidecar metadata (files, verify_extra) from the plan phase.
		if meta, err := beads.ReadBeadMeta(projectRoot, task.ID); err == nil {
			if len(task.Files) == 0 && len(meta.Files) > 0 {
				task.Files = meta.Files
			}
			task.VerifyExtra = meta.VerifyExtra
		}

		// Ensure KG MCP is alive for this bead.
		if cfg.KnowledgeGraph.Enabled != "never" {
			kgClient, err = graph.EnsureMCPAlive(projectRoot, cfg.KnowledgeGraph, kgClient)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: KG MCP unavailable for bead %s: %v\n", task.ID, err)
				kgClient = nil
			}
		}

		// Mark bead as in_progress.
		if err := beads.UpdateStatus(task.ID, "in_progress"); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to update bead %s status: %v\n", task.ID, err)
		}

		// Log task_started.
		if logErr := logger.Append(log.LogEvent{
			Event:  log.EventTaskStarted,
			BeadID: task.ID,
			Title:  task.Title,
		}); logErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to log task_started: %v\n", logErr)
		}

		// Print progress.
		fmt.Printf("%s %s: %s (attempt 1)...\n", pool.Progress(), task.ID, task.Title)

		// Pre-embed graph data for this bead's files.
		graphData := preEmbedGraphData(kgClient, task.Files)

		// Execute with retry logic.
		// RetryBead is defined in retry.go (same package, implemented by another agent).
		opts := &SpawnClaudeOpts{Verbose: verbose}
		beadResult, retryErr := RetryBead(cfg, task, graphData, projectRoot, logger, kgClient, opts)
		if retryErr != nil {
			fmt.Fprintf(os.Stderr, "Error during bead %s execution: %v\n", task.ID, retryErr)
		}

		// Extract summary from Claude's output for close reason.
		var claudeOutput string
		if beadResult != nil {
			claudeOutput = beadResult.ClaudeOutput
		}
		closeReason := beads.ExtractSummary(claudeOutput, task.Title)

		var lastError string
		if beadResult != nil && beadResult.Passed {
			// Bead succeeded: commit, close, record learning, reindex.
			if err := onBeadSuccess(task, kgClient, projectRoot, logger, systemPrompt, closeReason); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: post-success steps failed for bead %s: %v\n", task.ID, err)
			}
			pool.RecordCompletion()
			completedBeads = append(completedBeads, task.ID)
			breaker.RecordSuccess() // Reset consecutive failures on success
		} else {
			// Bead failed all retries: enter stuck handling.
			// HandleStuck is defined in stuck.go (same package, implemented by another agent).
			action, stuckErr := HandleStuck(cfg, task, nil, "", graphData, projectRoot)
			if stuckErr != nil {
				fmt.Fprintf(os.Stderr, "Error handling stuck bead %s: %v\n", task.ID, stuckErr)
				lastError = stuckErr.Error()
			}

			switch action.Action {
			case stuckActionSkip:
				pool.RecordSkip()
				failedBeads = append(failedBeads, task.ID)
				breaker.RecordFailure() // Increment consecutive failures
			case stuckActionAbort:
				// Save checkpoint before abort so state can be recovered.
				saveCheckpointState(runDir, branchName, task.ID, completedBeads, failedBeads, retryCount, breaker.GetConsecutiveFailures(), "aborted by user")
				// Abort the entire run.
				if logErr := logger.Append(log.LogEvent{
					Event:  log.EventRunComplete,
					Reason: "aborted",
					Total:  pool.Total,
				}); logErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to log run_complete: %v\n", logErr)
				}
				return fmt.Errorf("run aborted at bead %s", task.ID)
			case stuckActionRescue:
				// Bead was rescued interactively; treat as completed.
				if err := onBeadSuccess(task, kgClient, projectRoot, logger, systemPrompt); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: post-rescue steps failed for bead %s: %v\n", task.ID, err)
				}
				pool.RecordCompletion()
				completedBeads = append(completedBeads, task.ID)
				breaker.RecordSuccess() // Reset consecutive failures on success
			case stuckActionHint:
				// Hint succeeded verification; treat as completed.
				if err := onBeadSuccess(task, kgClient, projectRoot, logger, systemPrompt); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: post-hint steps failed for bead %s: %v\n", task.ID, err)
				}
				pool.RecordCompletion()
				completedBeads = append(completedBeads, task.ID)
				breaker.RecordSuccess() // Reset consecutive failures on success
			default:
				pool.RecordStuck()
				failedBeads = append(failedBeads, task.ID)
				breaker.RecordFailure() // Increment consecutive failures
			}
		}

		// Check if circuit breaker should pause execution.
		if breaker.ShouldPause() {
			// Save checkpoint before pausing.
			saveCheckpointState(runDir, branchName, task.ID, completedBeads, failedBeads, retryCount, breaker.GetConsecutiveFailures(), lastError)

			action, err := handleCircuitBreakerPause(breaker, pool)
			if err != nil {
				return fmt.Errorf("circuit breaker pause error: %w", err)
			}

			switch action {
			case "abort":
				if logErr := logger.Append(log.LogEvent{
					Event:  log.EventRunComplete,
					Reason: "aborted by circuit breaker",
					Total:  pool.Total,
				}); logErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to log run_complete: %v\n", logErr)
				}
				return fmt.Errorf("run aborted by circuit breaker after %d consecutive failures", cfg.Execution.CircuitBreakerThreshold)
			case "skip":
				// Reset breaker and continue with remaining beads.
				breaker.Reset()
				fmt.Println("Circuit breaker reset. Continuing with remaining beads...")
			case "retry":
				// Reset breaker; the loop will naturally retry the next ready bead.
				breaker.Reset()
				fmt.Println("Circuit breaker reset. Retrying...")
			}
		}

		// Save checkpoint after each bead completion/failure.
		saveCheckpointState(runDir, branchName, task.ID, completedBeads, failedBeads, retryCount, breaker.GetConsecutiveFailures(), lastError)

		if pool.IsComplete() {
			break
		}
	}

	// 9. Log run_complete.
	if logErr := logger.Append(log.LogEvent{
		Event:     log.EventRunComplete,
		Completed: pool.Completed,
		Stuck:     pool.Stuck,
		Total:     pool.Total,
	}); logErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to log run_complete: %v\n", logErr)
	}

	// 10. Clear checkpoint on successful completion.
	if pool.Stuck == 0 && pool.Skipped == 0 {
		if err := ClearCheckpoint(runDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to clear checkpoint: %v\n", err)
		}
	}

	fmt.Printf("Execution complete: %d completed, %d stuck, %d skipped out of %d total\n",
		pool.Completed, pool.Stuck, pool.Skipped, pool.Total)

	return nil
}

// saveCheckpointState is a helper function that saves checkpoint state.
// Errors are logged but not returned since checkpoint is best-effort.
func saveCheckpointState(runDir, runID, currentBeadID string, completedBeads, failedBeads []string, retryCount map[string]int, consecFailures int, lastError string) {
	cp := &Checkpoint{
		RunID:          runID,
		CurrentBeadID:  currentBeadID,
		CompletedBeads: completedBeads,
		FailedBeads:    failedBeads,
		RetryCount:     retryCount,
		ConsecFailures: consecFailures,
		LastError:      lastError,
	}
	if err := SaveCheckpoint(runDir, cp); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save checkpoint: %v\n", err)
	}
}

// onBeadSuccess handles post-success steps: close bead, append learning,
// reindex changed files, and log completion.
// Note: Claude already commits code changes during bead execution.
// We only commit here if there are leftover unstaged changes (e.g., generated files
// that Claude didn't stage). This avoids duplicate commits per bead.
// If closeReason is empty, falls back to the task title.
func onBeadSuccess(task *beads.Bead, kgClient *graph.Client, projectRoot string, logger *log.Logger, systemPrompt string, closeReason ...string) error {
	// Check for potential code duplication before proceeding (non-blocking warning).
	// This helps prevent recreating existing functionality.
	if kgClient != nil {
		result, err := kgClient.CheckDuplicationFromTitle(task.Title)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: duplication check failed for bead %s: %v\n", task.ID, err)
		} else {
			graph.WarnIfDuplicates(result)
		}
	}

	// Only commit berth/beads metadata — Claude already committed code.
	if err := git.CommitMetadata(task.ID); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to commit metadata for bead %s: %v\n", task.ID, err)
	}

	// Determine close reason: use provided reason or fall back to title.
	reason := task.Title
	if len(closeReason) > 0 && closeReason[0] != "" {
		reason = closeReason[0]
	}

	// Close the bead with reason.
	if err := beads.Close(task.ID, reason); err != nil {
		return fmt.Errorf("closing bead %s: %w", task.ID, err)
	}

	// Append learning.
	if err := berthcontext.AppendLearning(projectRoot, "Completed: "+task.Title); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to append learning for bead %s: %v\n", task.ID, err)
	}

	// Reindex changed files in the KG.
	if kgClient != nil && len(task.Files) > 0 {
		if err := graph.ReindexChanged(kgClient, task.Files); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to reindex after bead %s: %v\n", task.ID, err)
		}
	}

	// Log completion.
	if logErr := logger.Append(log.LogEvent{
		Event:  log.EventTaskCompleted,
		BeadID: task.ID,
		Title:  task.Title,
	}); logErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to log task_completed: %v\n", logErr)
	}

	return nil
}

// readSystemPrompt reads system prompts and combines them.
// Order: root CLAUDE.md (project conventions) + .berth/CLAUDE.md (executor context).
// Returns error only if .berth/CLAUDE.md cannot be read.
func readSystemPrompt(projectRoot string) (string, error) {
	var parts []string

	// 1. Read root CLAUDE.md if it exists (project conventions).
	rootPath := filepath.Join(projectRoot, "CLAUDE.md")
	if rootData, err := os.ReadFile(rootPath); err == nil {
		parts = append(parts, "# Project Conventions\n\n"+string(rootData))
	}

	// 2. Read .berth/CLAUDE.md (executor context).
	berthPath := filepath.Join(projectRoot, ".berth", "CLAUDE.md")
	berthData, err := os.ReadFile(berthPath)
	if err != nil {
		return "", fmt.Errorf("reading system prompt: %w", err)
	}
	parts = append(parts, string(berthData))

	return strings.Join(parts, "\n\n"), nil
}

// preEmbedGraphData queries the KG client for data about the bead's files
// and formats it as a markdown section. Returns an empty string if KG is
// unavailable or has no data.
func preEmbedGraphData(kgClient *graph.Client, files []string) string {
	if kgClient == nil || len(files) == 0 {
		return ""
	}

	var graphFiles []graph.FileGraphData
	for _, file := range files {
		understanding, err := kgClient.UnderstandFile(file)
		if err != nil {
			continue
		}
		if understanding == nil {
			continue
		}

		fgd := graph.FileGraphData{
			Path:       understanding.File,
			Exports:    understanding.Exports,
			Importers:  understanding.Importers,
			Callers:    make(map[string][]graph.CallerResult),
			TypeUsages: make(map[string][]graph.TypeUsageResult),
		}

		// Query callers for each exported function.
		for _, exp := range understanding.Exports {
			if exp.Kind == "function" {
				callers, qErr := kgClient.QueryCallers(exp.Name)
				if qErr == nil && len(callers) > 0 {
					fgd.Callers[exp.Name] = callers
				}
			}
		}

		// Query type usages for each exported type/class/interface/enum.
		for _, exp := range understanding.Exports {
			switch exp.Kind {
			case "type", "class", "interface", "enum":
				usages, qErr := kgClient.QueryTypeUsages(exp.Name)
				if qErr == nil && len(usages) > 0 {
					fgd.TypeUsages[exp.Name] = usages
				}
			}
		}

		graphFiles = append(graphFiles, fgd)
	}

	if len(graphFiles) == 0 {
		return ""
	}

	// Impact analysis for the bead's file set.
	// AnalyzeImpact takes a single file path, so call per-file and merge
	// with deduplication (multiple files may share dependents).
	impact := &graph.ImpactAnalysis{}
	seenDirect := make(map[string]bool)
	seenTransitive := make(map[string]bool)
	seenTests := make(map[string]bool)
	for _, file := range files {
		result, err := kgClient.AnalyzeImpact(file)
		if err != nil || result == nil {
			continue
		}
		for _, d := range result.DirectDependents {
			key := d.File + "|" + d.Kind + "|" + d.Name
			if !seenDirect[key] {
				seenDirect[key] = true
				impact.DirectDependents = append(impact.DirectDependents, d)
			}
		}
		for _, t := range result.TransitiveDependents {
			key := t.File + "|" + t.Via
			if !seenTransitive[key] {
				seenTransitive[key] = true
				impact.TransitiveDependents = append(impact.TransitiveDependents, t)
			}
		}
		for _, s := range result.AffectedTests {
			if !seenTests[s] {
				seenTests[s] = true
				impact.AffectedTests = append(impact.AffectedTests, s)
			}
		}
	}

	// Only include impact if we found any data.
	var impactPtr *graph.ImpactAnalysis
	if len(impact.DirectDependents) > 0 || len(impact.TransitiveDependents) > 0 || len(impact.AffectedTests) > 0 {
		impactPtr = impact
	}

	data := &graph.GraphData{
		Files:  graphFiles,
		Impact: impactPtr,
	}
	return graph.FormatGraphData(data)
}

// handleCircuitBreakerPause presents the user with options when the circuit
// breaker has triggered due to consecutive failures. Returns the user's
// chosen action: "retry", "skip", or "abort".
func handleCircuitBreakerPause(breaker *CircuitBreaker, pool *ExecutionPool) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Printf("Circuit breaker triggered: %d consecutive failures reached threshold.\n", breaker.ConsecutiveFailures)
	fmt.Printf("Progress: %d completed, %d stuck, %d skipped out of %d total\n",
		pool.Completed, pool.Stuck, pool.Skipped, pool.Total)
	fmt.Println()
	fmt.Println("What do you want to do?")
	fmt.Println()
	fmt.Println("  [1] Retry   -- Reset circuit breaker and continue execution")
	fmt.Println("  [2] Skip    -- Skip remaining beads and finish")
	fmt.Println("  [3] Abort   -- Stop the entire run immediately")
	fmt.Println()

	for {
		fmt.Print("Choice: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("reading user choice: %w", err)
		}

		choice := strings.TrimSpace(input)
		switch choice {
		case "1":
			return "retry", nil
		case "2":
			return "skip", nil
		case "3":
			return "abort", nil
		default:
			fmt.Println("Invalid choice. Please enter 1, 2, or 3.")
		}
	}
}
