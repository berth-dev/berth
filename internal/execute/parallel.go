// parallel.go implements the top-level parallel execution orchestrator.
package execute

import (
	"fmt"
	"os"
	"time"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/coordinator"
	"github.com/berth-dev/berth/internal/git"
	"github.com/berth-dev/berth/internal/graph"
	"github.com/berth-dev/berth/internal/log"
	"github.com/berth-dev/berth/prompts"
)

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
func RunExecuteParallel(cfg config.Config, projectRoot string, runDir string, branchName string, prefetchedBeads []beads.Bead) error {
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
			kgClient.Close()
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
	go coordServer.Start()
	coordServer.StartLockReaper(5 * time.Minute)
	defer coordServer.Stop()

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
		kgClient, logger, systemPrompt,
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
