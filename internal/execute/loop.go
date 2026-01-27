// Package execute implements Phase 3: the main execution loop that processes beads.
// This file contains the top-level execution orchestrator.
package execute

import (
	"fmt"
	"os"
	"path/filepath"

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

// RunExecute is the main execution entry point. It creates a feature branch,
// starts the KG MCP server, and processes beads one at a time through the
// retry loop until all beads are completed, stuck, or skipped.
func RunExecute(cfg config.Config, projectRoot string, runDir string, branchName string) error {
	// 1. Create a git branch for this execution run.
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
		passed, retryErr := RetryBead(cfg, task, graphData, projectRoot, logger, kgClient)
		if retryErr != nil {
			fmt.Fprintf(os.Stderr, "Error during bead %s execution: %v\n", task.ID, retryErr)
		}

		if passed {
			// Bead succeeded: commit, close, record learning, reindex.
			if err := onBeadSuccess(task, kgClient, projectRoot, logger, systemPrompt); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: post-success steps failed for bead %s: %v\n", task.ID, err)
			}
			pool.RecordCompletion()
		} else {
			// Bead failed all retries: enter stuck handling.
			// HandleStuck is defined in stuck.go (same package, implemented by another agent).
			action, stuckErr := HandleStuck(cfg, task, nil, "", graphData, projectRoot)
			if stuckErr != nil {
				fmt.Fprintf(os.Stderr, "Error handling stuck bead %s: %v\n", task.ID, stuckErr)
			}

			switch action.Action {
			case stuckActionSkip:
				pool.RecordSkip()
			case stuckActionAbort:
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
			case stuckActionHint:
				// Hint succeeded verification; treat as completed.
				if err := onBeadSuccess(task, kgClient, projectRoot, logger, systemPrompt); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: post-hint steps failed for bead %s: %v\n", task.ID, err)
				}
				pool.RecordCompletion()
			default:
				pool.RecordStuck()
			}
		}

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

	fmt.Printf("Execution complete: %d completed, %d stuck, %d skipped out of %d total\n",
		pool.Completed, pool.Stuck, pool.Skipped, pool.Total)

	return nil
}

// onBeadSuccess handles post-success steps: commit, close bead, append learning,
// reindex changed files, and log completion.
func onBeadSuccess(task *beads.Bead, kgClient *graph.Client, projectRoot string, logger *log.Logger, systemPrompt string) error {
	// Commit changes for this bead.
	if err := git.CommitBead(task.ID, task.Title); err != nil {
		return fmt.Errorf("committing bead %s: %w", task.ID, err)
	}

	// Close the bead.
	if err := beads.Close(task.ID, task.Title); err != nil {
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

// readSystemPrompt reads the system prompt from .berth/CLAUDE.md.
// Returns the file content or an error if the file cannot be read.
func readSystemPrompt(projectRoot string) (string, error) {
	path := filepath.Join(projectRoot, ".berth", "CLAUDE.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading system prompt: %w", err)
	}
	return string(data), nil
}

// preEmbedGraphData queries the KG client for data about the bead's files
// and formats it as a markdown section. Returns an empty string if KG is
// unavailable or has no data.
func preEmbedGraphData(kgClient *graph.Client, files []string) string {
	if kgClient == nil || len(files) == 0 {
		return ""
	}

	// Query understanding for each file via the MCP client and collect results.
	var graphFiles []graph.FileGraphData
	for _, file := range files {
		understanding, err := kgClient.UnderstandFile(file)
		if err != nil {
			// Skip files that fail to query.
			continue
		}
		if understanding != nil {
			graphFiles = append(graphFiles, graph.FileGraphData{
				Path:    understanding.File,
				Exports: understanding.Exports,
				Importers: understanding.Importers,
			})
		}
	}

	if len(graphFiles) == 0 {
		return ""
	}

	data := &graph.GraphData{Files: graphFiles}
	return graph.FormatGraphData(data)
}
