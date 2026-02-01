// parallel.go implements the top-level parallel execution orchestrator.
package execute

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/coordinator"
	"github.com/berth-dev/berth/internal/git"
	"github.com/berth-dev/berth/internal/graph"
	"github.com/berth-dev/berth/internal/log"
	"github.com/berth-dev/berth/prompts"
)

// OutputEvent represents a streaming event from parallel bead execution.
type OutputEvent struct {
	Type     string // "output", "complete", "error", "token_update"
	BeadID   string
	Content  string
	Tokens   int
	IsStderr bool
}

// ParallelResult contains the outcome of a single bead's parallel execution.
type ParallelResult struct {
	BeadID       string
	Passed       bool
	ClaudeOutput string
	Error        error
	WorktreePath string
}

// ShouldRunParallel determines whether to use parallel execution based on
// config mode and bead topology.
func ShouldRunParallel(cfg config.Config, allBeads []beads.Bead) bool {
	mode := cfg.Execution.ParallelMode
	switch mode {
	case "always":
		return true
	case "never":
		return false
	case "auto", "":
		// Auto: need enough beads and at least 2 independent roots.
		threshold := cfg.Execution.ParallelThreshold
		if threshold <= 0 {
			threshold = 4
		}
		if len(allBeads) < threshold {
			return false
		}
		return countRootBeads(allBeads) >= 2
	default:
		return false
	}
}

// countRootBeads returns the number of beads with no dependencies.
func countRootBeads(allBeads []beads.Bead) int {
	count := 0
	for _, b := range allBeads {
		if len(b.DependsOn) == 0 {
			count++
		}
	}
	return count
}

// RunExecuteParallel is the parallel execution entry point. It sets up the
// coordinator server, worktree manager, merge queue, and scheduler, then
// runs all beads concurrently up to MaxParallel. prefetchedBeads is the
// bead list already fetched by RunExecute to avoid a redundant bd list call.
func RunExecuteParallel(cfg config.Config, projectRoot string, runDir string, branchName string, prefetchedBeads []beads.Bead, verbose bool) error {
	// 1. Create a git branch for this execution run.
	if err := git.EnsureInitialCommit(); err != nil {
		return fmt.Errorf("ensuring initial commit: %w", err)
	}
	if err := git.CreateBranch(branchName); err != nil {
		if switchErr := git.SwitchBranch(branchName); switchErr != nil {
			return fmt.Errorf("creating or switching to branch %s: %w", branchName, err)
		}
	}

	// 2. Read system prompt.
	systemPrompt, err := readSystemPrompt(projectRoot)
	if err != nil {
		systemPrompt = prompts.ExecutorSystemPrompt
	}

	// 3. Start KG MCP.
	var kgClient *graph.Client
	if cfg.KnowledgeGraph.Enabled != "never" {
		kgClient, err = graph.EnsureMCPAlive(projectRoot, cfg.KnowledgeGraph, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: KG MCP unavailable: %v\n", err)
			kgClient = nil
		}
	}
	defer func() {
		if kgClient != nil {
			_ = kgClient.Close()
		}
	}()

	// 4. Use pre-fetched beads list.
	allBeads := prefetchedBeads
	pool := NewExecutionPool(len(allBeads))

	fmt.Printf("Executing %d beads in parallel (max %d) on branch %s\n",
		pool.Total, cfg.Execution.MaxParallel, branchName)

	// 5. Create logger.
	logger, err := log.NewLogger(projectRoot)
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}

	if logErr := logger.Append(log.LogEvent{
		Event:  log.EventRunStarted,
		Branch: branchName,
		Beads:  pool.Total,
		Data:   map[string]interface{}{"mode": "parallel"},
	}); logErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to log run_started: %v\n", logErr)
	}

	// 6. Start coordinator HTTP server.
	coordServer, err := coordinator.NewServer()
	if err != nil {
		return fmt.Errorf("starting coordinator server: %w", err)
	}
	go func() { _ = coordServer.Start() }()
	coordServer.StartLockReaper(5 * time.Minute)
	defer func() { _ = coordServer.Stop() }()

	fmt.Printf("Coordinator server running on %s\n", coordServer.Addr())

	// 7. Create worktree manager.
	worktrees := NewWorktreeManager(projectRoot, branchName)
	defer worktrees.CleanupAll()

	// 8. Create merge queue.
	mergeQueue := NewMergeQueue(cfg, projectRoot, branchName, kgClient, logger, worktrees, systemPrompt)
	go mergeQueue.Start()

	// 9. Create scheduler and run.
	scheduler := NewScheduler(
		cfg, projectRoot, allBeads, pool,
		worktrees, mergeQueue, coordServer,
		kgClient, logger, systemPrompt, verbose,
	)

	if err := scheduler.Run(); err != nil {
		mergeQueue.Close()
		mergeQueue.Wait()
		return fmt.Errorf("scheduler error: %w", err)
	}

	// 10. Close merge queue and wait for completion.
	mergeQueue.Close()
	mergeQueue.Wait()

	// 11. Log run complete.
	if logErr := logger.Append(log.LogEvent{
		Event:     log.EventRunComplete,
		Completed: pool.Completed,
		Stuck:     pool.Stuck,
		Total:     pool.Total,
	}); logErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to log run_complete: %v\n", logErr)
	}

	fmt.Printf("Parallel execution complete: %d completed, %d stuck, %d skipped out of %d total\n",
		pool.Completed, pool.Stuck, pool.Skipped, pool.Total)

	return nil
}

// RunParallel executes all beads in an ExecutionGroup concurrently.
// Each bead runs in its own worktree. Results are collected and returned.
// The outputChan receives streaming events during execution if non-nil.
func RunParallel(
	ctx context.Context,
	group ExecutionGroup,
	projectRoot string,
	cfg *config.Config,
	kgClient *graph.Client,
	systemPrompt string,
	outputChan chan<- OutputEvent,
) []ParallelResult {
	if len(group.BeadIDs) == 0 {
		return nil
	}

	// Fetch all beads once to look up by ID.
	allBeads, err := beads.List()
	if err != nil {
		// Return error results for all beads if we can't list them.
		results := make([]ParallelResult, len(group.BeadIDs))
		for i, beadID := range group.BeadIDs {
			results[i] = ParallelResult{
				BeadID: beadID,
				Passed: false,
				Error:  fmt.Errorf("failed to list beads: %w", err),
			}
		}
		return results
	}

	var wg sync.WaitGroup
	resultsChan := make(chan ParallelResult, len(group.BeadIDs))

	// Create a logger for this execution.
	logger, logErr := log.NewLogger(projectRoot)
	if logErr != nil {
		logger = nil
	}

	for _, beadID := range group.BeadIDs {
		beadID := beadID // capture for goroutine

		wg.Add(1)
		go func() {
			defer wg.Done()

			// Check for context cancellation.
			select {
			case <-ctx.Done():
				resultsChan <- ParallelResult{
					BeadID: beadID,
					Passed: false,
					Error:  ctx.Err(),
				}
				return
			default:
			}

			// Create worktree for this bead.
			worktreePath, wtErr := git.CreateWorktreeForBead(projectRoot, beadID)
			if wtErr != nil {
				if outputChan != nil {
					outputChan <- OutputEvent{
						Type:    "error",
						BeadID:  beadID,
						Content: fmt.Sprintf("failed to create worktree: %v", wtErr),
					}
				}
				resultsChan <- ParallelResult{
					BeadID: beadID,
					Passed: false,
					Error:  fmt.Errorf("creating worktree: %w", wtErr),
				}
				return
			}

			// Find the bead in the list.
			bead := GetBeadByID(allBeads, beadID)
			if bead == nil {
				if outputChan != nil {
					outputChan <- OutputEvent{
						Type:    "error",
						BeadID:  beadID,
						Content: "bead not found",
					}
				}
				resultsChan <- ParallelResult{
					BeadID:       beadID,
					Passed:       false,
					Error:        fmt.Errorf("bead %s not found", beadID),
					WorktreePath: worktreePath,
				}
				return
			}

			// Load sidecar metadata.
			if meta, metaErr := beads.ReadBeadMeta(projectRoot, beadID); metaErr == nil {
				if len(bead.Files) == 0 && len(meta.Files) > 0 {
					bead.Files = meta.Files
				}
				bead.VerifyExtra = meta.VerifyExtra
			}

			// Pre-embed graph data for this bead's files.
			graphData := preEmbedGraphData(kgClient, bead.Files)

			// Build spawn opts with worktree as WorkDir.
			opts := &SpawnClaudeOpts{
				WorkDir:      worktreePath,
				SystemPrompt: systemPrompt,
			}

			// Send output event indicating start.
			if outputChan != nil {
				outputChan <- OutputEvent{
					Type:    "output",
					BeadID:  beadID,
					Content: fmt.Sprintf("Starting bead execution in worktree: %s", worktreePath),
				}
			}

			// Call RetryBead with worktree as WorkDir.
			beadResult, retryErr := RetryBead(*cfg, bead, graphData, projectRoot, logger, kgClient, opts)

			// Determine outcome.
			passed := beadResult != nil && beadResult.Passed
			var claudeOutput string
			if beadResult != nil {
				claudeOutput = beadResult.ClaudeOutput
			}

			// Send completion event.
			if outputChan != nil {
				eventType := "complete"
				if !passed {
					eventType = "error"
				}
				outputChan <- OutputEvent{
					Type:    eventType,
					BeadID:  beadID,
					Content: claudeOutput,
				}
			}

			resultsChan <- ParallelResult{
				BeadID:       beadID,
				Passed:       passed,
				ClaudeOutput: claudeOutput,
				Error:        retryErr,
				WorktreePath: worktreePath,
			}
		}()
	}

	// Wait for all goroutines to complete.
	wg.Wait()
	close(resultsChan)

	// Collect results.
	results := make([]ParallelResult, 0, len(group.BeadIDs))
	for result := range resultsChan {
		results = append(results, result)
	}

	return results
}

// MergeParallelResults merges successful bead worktrees into the target branch.
// Returns a slice of merge conflicts encountered during merging.
func MergeParallelResults(
	projectRoot string,
	targetBranch string,
	results []ParallelResult,
) ([]git.MergeConflict, error) {
	var conflicts []git.MergeConflict

	// Switch to target branch first.
	if err := git.SwitchBranch(targetBranch); err != nil {
		return nil, fmt.Errorf("switching to target branch %s: %w", targetBranch, err)
	}

	for _, result := range results {
		// Skip failed beads.
		if !result.Passed {
			continue
		}

		// Merge the worktree branch into target.
		if err := git.MergeWorktreeForBead(projectRoot, result.BeadID, targetBranch); err != nil {
			// Check if it's a merge conflict error.
			var mergeConflict *git.MergeConflict
			if errors.As(err, &mergeConflict) {
				conflicts = append(conflicts, *mergeConflict)
				// Abort the merge to clean up state before continuing.
				_ = git.AbortMerge()
				continue
			}
			// Other merge error - append as conflict with error info.
			conflicts = append(conflicts, git.MergeConflict{
				BeadID: result.BeadID,
				Output: err.Error(),
			})
			_ = git.AbortMerge()
			continue
		}

		// Remove worktree after successful merge.
		if err := git.RemoveWorktreeForBead(projectRoot, result.BeadID); err != nil {
			// Log warning but continue - worktree cleanup is best effort.
			fmt.Fprintf(os.Stderr, "Warning: failed to remove worktree for bead %s: %v\n", result.BeadID, err)
		}
	}

	return conflicts, nil
}
