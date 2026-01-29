// resume.go implements the "berth resume" command for resuming interrupted runs.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/execute"
	"github.com/berth-dev/berth/internal/git"
	"github.com/berth-dev/berth/internal/log"
	"github.com/berth-dev/berth/internal/report"
	"github.com/spf13/cobra"
)

var resumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume an interrupted run",
	Long: `Resume a previously interrupted berth run. Finds the latest run
directory, restores branch state, handles stuck beads, and resumes
the execution loop.`,
	RunE: runResume,
}

var skipStuckFlag bool

func init() {
	resumeCmd.Flags().BoolVar(&skipStuckFlag, "skip-stuck", false, "Skip stuck beads instead of retrying them")
}

func runResume(cmd *cobra.Command, args []string) error {
	// Validate: .berth/ must exist.
	if _, err := os.Stat(".berth"); os.IsNotExist(err) {
		return fmt.Errorf(".berth/ not found. Run 'berth init' first")
	}

	projectRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Read config.
	cfg, err := config.ReadConfig(".")
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	// Find latest run directory.
	runDir, err := findLatestRunDir()
	if err != nil {
		return fmt.Errorf("finding latest run: %w", err)
	}
	fmt.Printf("Resuming run from: %s\n", runDir)

	// Determine the expected branch name from config.
	branchName := cfg.Execution.BranchPrefix + cfg.Project.Name
	if branchName == cfg.Execution.BranchPrefix {
		// Project name not set; try to detect from current branch.
		current, branchErr := git.CurrentBranch()
		if branchErr != nil {
			return fmt.Errorf("cannot determine branch: %w", branchErr)
		}
		branchName = current
	}

	// Ensure we are on the correct branch.
	currentBranch, err := git.CurrentBranch()
	if err != nil {
		return fmt.Errorf("getting current branch: %w", err)
	}
	if currentBranch != branchName {
		if git.BranchExists(branchName) {
			fmt.Printf("Switching to branch: %s\n", branchName)
			if switchErr := git.SwitchBranch(branchName); switchErr != nil {
				return fmt.Errorf("switching to branch %s: %w", branchName, switchErr)
			}
		} else {
			fmt.Printf("Warning: expected branch %s not found, continuing on %s\n", branchName, currentBranch)
		}
	}

	// Load checkpoint to restore execution state.
	checkpoint, checkpointErr := execute.LoadCheckpoint(runDir)
	if checkpointErr != nil {
		// Checkpoint corrupted: warn user but continue with fresh state.
		fmt.Fprintf(os.Stderr, "Warning: failed to load checkpoint (continuing with fresh state): %v\n", checkpointErr)
		checkpoint = nil
	}

	// Prepare execution state from checkpoint.
	var execState *execute.ExecuteState
	if checkpoint != nil {
		fmt.Printf("Restored checkpoint state: %d completed, %d failed, %d consecutive failures\n",
			len(checkpoint.CompletedBeads), len(checkpoint.FailedBeads), checkpoint.ConsecFailures)
		execState = &execute.ExecuteState{
			RetryCount:     checkpoint.RetryCount,
			ConsecFailures: checkpoint.ConsecFailures,
		}
	}

	// List all beads to handle stuck and in_progress states.
	allBeads, err := beads.List()
	if err != nil {
		return fmt.Errorf("listing beads: %w", err)
	}

	stuckCount := 0
	inProgressCount := 0

	for _, b := range allBeads {
		switch b.Status {
		case "stuck":
			stuckCount++
			if skipStuckFlag {
				// Mark stuck beads as skipped by closing them.
				if closeErr := beads.Close(b.ID, "skipped by resume --skip-stuck"); closeErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to skip bead %s: %v\n", b.ID, closeErr)
				} else {
					fmt.Printf("  Skipped stuck bead: %s (%s)\n", b.ID, b.Title)
				}
			}
		case "in_progress":
			inProgressCount++
			// Reset in_progress beads to pending (open) so they get retried.
			if resetErr := beads.UpdateStatus(b.ID, "open"); resetErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to reset bead %s: %v\n", b.ID, resetErr)
			} else {
				fmt.Printf("  Reset in_progress bead: %s (%s)\n", b.ID, b.Title)
			}
		}
	}

	if stuckCount > 0 && !skipStuckFlag {
		fmt.Printf("\n%d stuck bead(s) found. Use --skip-stuck to skip them.\n", stuckCount)
	}
	if inProgressCount > 0 {
		fmt.Printf("Reset %d in_progress bead(s) to pending.\n", inProgressCount)
	}

	// Create logger.
	logger, err := log.NewLogger(projectRoot)
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}

	// Log resume event.
	if logErr := logger.Append(log.LogEvent{
		Event:  log.EventRunStarted,
		Branch: branchName,
		Reason: "resumed",
	}); logErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to log resume: %v\n", logErr)
	}

	// Resume execution with restored state.
	fmt.Println("\nResuming execution...")
	if execErr := execute.RunExecuteWithState(*cfg, projectRoot, runDir, branchName, Verbose(), execState); execErr != nil {
		fmt.Fprintf(os.Stderr, "Execute phase error: %v\n", execErr)
		// Continue to report phase.
	}

	// Generate report.
	fmt.Println("\nGenerating report...")
	r, err := report.GenerateReport(*cfg, projectRoot, runDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: report generation error: %v\n", err)
	}
	if r != nil {
		fmt.Println()
		fmt.Print(report.FormatReport(r))
	}

	return nil
}

// findLatestRunDir finds the most recent run directory in .berth/runs/.
func findLatestRunDir() (string, error) {
	runsDir := filepath.Join(".berth", "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return "", fmt.Errorf("reading runs directory: %w", err)
	}

	if len(entries) == 0 {
		return "", fmt.Errorf("no runs found in .berth/runs/")
	}

	// Sort entries by name descending (timestamp format ensures lexicographic order).
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() > entries[j].Name()
	})

	// Find the first directory entry.
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(runsDir, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("no run directories found in .berth/runs/")
}
