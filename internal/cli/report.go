// report.go implements the "berth report" command for generating run summaries.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/berth-dev/berth/internal/config"
	berthreport "github.com/berth-dev/berth/internal/report"
	"github.com/spf13/cobra"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Show last run results",
	Long: `Display a detailed report of the most recent completed Berth run,
including all beads, their outcomes, commits, files changed, and learnings.`,
	RunE: runReport,
}

func runReport(cmd *cobra.Command, args []string) error {
	// Get project root.
	projectRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Read config.
	cfg, err := config.ReadConfig(".")
	if err != nil {
		// Use default config if none exists.
		cfg = config.DefaultConfig()
	}

	// Find latest run dir in .berth/runs/.
	runsDir := filepath.Join(".berth", "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil || len(entries) == 0 {
		return fmt.Errorf("No completed runs found. Start one with: berth run")
	}

	// Sort entries by name (timestamps sort lexicographically) and pick latest.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() > entries[j].Name()
	})

	var latestRunDir string
	for _, e := range entries {
		if e.IsDir() {
			latestRunDir = filepath.Join(runsDir, e.Name())
			break
		}
	}
	if latestRunDir == "" {
		return fmt.Errorf("No completed runs found. Start one with: berth run")
	}

	// Try to read existing report file.
	reportPath := filepath.Join(latestRunDir, "report.md")
	if data, err := os.ReadFile(reportPath); err == nil {
		fmt.Print(string(data))
		return nil
	}

	// No existing report; generate one.
	r, err := berthreport.GenerateReport(*cfg, projectRoot, latestRunDir)
	if err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	fmt.Print(berthreport.FormatReport(r))
	return nil
}
