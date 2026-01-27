// add.go implements the "berth add" command for adding tasks with
// optional --priority and --depends flags.
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add [task description]",
	Short: "Add a task to the current run",
	Long: `Inject a new bead mid-run. The task is picked up by the
execute loop on the next 'bd ready' call.`,
	Args: cobra.ExactArgs(1),
	RunE: runAdd,
}

func init() {
	addCmd.Flags().String("priority", "normal", "Task priority: high, normal, low")
	addCmd.Flags().String("depends", "", "Bead ID this task depends on")
}

func runAdd(cmd *cobra.Command, args []string) error {
	// Validate active run: check .berth/runs/ for a directory.
	runsDir := filepath.Join(".berth", "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil || len(entries) == 0 {
		return fmt.Errorf("No active run. Start one with: berth run")
	}

	description := args[0]

	// Create bead with title=description for mid-run adds.
	beadID, err := beads.Create(description, description)
	if err != nil {
		return fmt.Errorf("failed to create bead: %w", err)
	}

	// If --depends flag is set, add the dependency.
	depends, _ := cmd.Flags().GetString("depends")
	if depends != "" {
		if err := beads.AddDependency(beadID, depends); err != nil {
			return fmt.Errorf("failed to add dependency: %w", err)
		}
	}

	fmt.Printf("Added bead %s: %s\n", beadID, description)
	return nil
}
