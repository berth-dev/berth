// clean.go implements the "berth clean" command for manual run directory cleanup.
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/berth-dev/berth/internal/cleanup"
	"github.com/berth-dev/berth/internal/config"
	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove old run directories",
	Long: `Remove old run directories from .berth/runs/.

By default, removes runs older than the configured max_age_days (default 30).
Use --keep to keep only the N most recent runs instead.
Use --dry-run to preview what would be removed.`,
	RunE: runClean,
}

var (
	keepFlag   int
	dryRunFlag bool
)

func init() {
	cleanCmd.Flags().IntVar(&keepFlag, "keep", 0, "Keep only the last N runs (0 = use age-based cleanup)")
	cleanCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Preview what would be removed without deleting")
}

func runClean(cmd *cobra.Command, args []string) error {
	if _, err := os.Stat(".berth"); os.IsNotExist(err) {
		return fmt.Errorf(".berth/ not found. Run 'berth init' first")
	}

	runsDir := filepath.Join(".berth", "runs")

	var pruned []string
	var err error

	if keepFlag > 0 {
		pruned, err = cleanup.PruneKeepRecent(runsDir, keepFlag, dryRunFlag)
	} else {
		cfg, cfgErr := config.ReadConfig(".")
		if cfgErr != nil {
			return fmt.Errorf("reading config: %w", cfgErr)
		}

		maxAge := cfg.Cleanup.MaxAgeDays
		if maxAge <= 0 {
			maxAge = 30
		}
		pruned, err = cleanup.PruneByAge(runsDir, maxAge, dryRunFlag)
	}

	if err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	if len(pruned) == 0 {
		fmt.Println("No runs to clean up.")
		return nil
	}

	verb := "Removed"
	if dryRunFlag {
		verb = "Would remove"
	}

	for _, name := range pruned {
		fmt.Printf("  %s %s\n", verb, name)
	}
	fmt.Printf("%s %d run(s).\n", verb, len(pruned))

	return nil
}
