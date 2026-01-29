// status.go implements the "berth status" command showing current run progress.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/git"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current run progress",
	Long: `Display the status of the current or most recent Berth run,
including all beads and their states.`,
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Find latest run directory in .berth/runs/.
	runsDir := filepath.Join(".berth", "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil || len(entries) == 0 {
		return fmt.Errorf("no runs found; start one with: berth run")
	}

	// Load all beads.
	allBeads, err := beads.List()
	if err != nil {
		return fmt.Errorf("failed to list beads: %w", err)
	}
	if len(allBeads) == 0 {
		return fmt.Errorf("no runs found; start one with: berth run")
	}

	// Get current branch (best-effort).
	branch, branchErr := git.CurrentBranch()

	fmt.Println("Berth Status")
	if branchErr == nil && branch != "" {
		fmt.Printf("Branch: %s\n", branch)
	}
	fmt.Println()

	doneCount := 0
	total := len(allBeads)

	for _, b := range allBeads {
		status := normalizeStatus(b.Status)
		extra := formatBeadExtra(b)

		fmt.Printf("  %-6s  %-13s  %s", b.ID, status, b.Title)
		if extra != "" {
			fmt.Printf("  %s", extra)
		}
		fmt.Println()

		if b.Status == "done" {
			doneCount++
		}
	}

	fmt.Println()
	fmt.Printf("Progress: %d/%d beads complete\n", doneCount, total)

	return nil
}

// normalizeStatus maps internal bead statuses to display-friendly labels.
func normalizeStatus(status string) string {
	switch status {
	case "done":
		return "done"
	case "in_progress":
		return "in_progress"
	case "open":
		return "pending"
	case "stuck":
		return "stuck"
	case "skipped":
		return "skipped"
	default:
		return status
	}
}

// formatBeadExtra returns additional context for a bead's display line.
func formatBeadExtra(b beads.Bead) string {
	var parts []string

	if b.Status == "in_progress" {
		parts = append(parts, "[attempt 1]")
	}

	if len(b.DependsOn) > 0 && (b.Status == "open" || b.Status == "pending") {
		parts = append(parts, fmt.Sprintf("[blocked by %s]", strings.Join(b.DependsOn, ", ")))
	}

	return strings.Join(parts, " ")
}
