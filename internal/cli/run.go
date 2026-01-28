// run.go implements the "berth run" command which drives the full
// understand -> plan -> execute -> report pipeline.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/berth-dev/berth/internal/cleanup"
	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/detect"
	"github.com/berth-dev/berth/internal/execute"
	"github.com/berth-dev/berth/internal/log"
	"github.com/berth-dev/berth/internal/plan"
	"github.com/berth-dev/berth/internal/report"
	"github.com/berth-dev/berth/internal/understand"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run [description]",
	Short: "Run a full development task",
	Long: `Run the full berth pipeline: understand requirements, generate a plan,
execute beads, and produce a report. Requires a task description or --prd flag.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRun,
}

var (
	prdFlag            string
	skipUnderstandFlag bool
	skipApproveFlag    bool
	reindexFlag        bool
	branchFlag         string
	parallelFlag       bool
)

func init() {
	runCmd.Flags().StringVar(&prdFlag, "prd", "", "Path to a PRD file (skips UNDERSTAND phase)")
	runCmd.Flags().BoolVar(&skipUnderstandFlag, "skip-understand", false, "Skip the interview loop")
	runCmd.Flags().BoolVar(&skipApproveFlag, "skip-approve", false, "Auto-approve the generated plan")
	runCmd.Flags().BoolVar(&reindexFlag, "reindex", false, "Force full Knowledge Graph reindex")
	runCmd.Flags().StringVar(&branchFlag, "branch", "", "Custom branch name (default: berth/{sanitized-description})")
	runCmd.Flags().BoolVar(&parallelFlag, "parallel", false, "Enable parallel bead execution (not yet implemented)")
}

func runRun(cmd *cobra.Command, args []string) error {
	// Validate: .berth/ must exist.
	if _, err := os.Stat(".berth"); os.IsNotExist(err) {
		return fmt.Errorf(".berth/ not found. Run 'berth init' first")
	}

	// Validate: need description or --prd.
	var description string
	if len(args) > 0 {
		description = args[0]
	}
	if description == "" && prdFlag == "" {
		return fmt.Errorf("provide a task description or use --prd flag")
	}

	// Validate: must be in a git repo.
	if _, err := os.Stat(".git"); os.IsNotExist(err) {
		return fmt.Errorf("not a git repository. Initialize git first")
	}

	if parallelFlag {
		fmt.Println("Warning: --parallel is not yet implemented; running sequentially.")
	}

	// Get project root.
	projectRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Read config.
	cfg, err := config.ReadConfig(".")
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	// Detect stack info.
	stackInfo := detect.DetectStack(projectRoot)

	// Create run directory.
	timestamp := time.Now().Format("20060102-150405")
	runDir := filepath.Join(".berth", "runs", timestamp)
	if mkErr := os.MkdirAll(runDir, 0755); mkErr != nil {
		return fmt.Errorf("creating run directory: %w", mkErr)
	}

	// Auto-prune old run directories.
	if cfg.Cleanup.MaxAgeDays > 0 {
		runsDir := filepath.Join(".berth", "runs")
		pruned, pruneErr := cleanup.PruneByAge(runsDir, cfg.Cleanup.MaxAgeDays, false)
		if pruneErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: cleanup failed: %v\n", pruneErr)
		} else if len(pruned) > 0 {
			fmt.Fprintf(os.Stderr, "Cleaned up %d old run(s)\n", len(pruned))
		}
	}

	// Determine branch name for execute phase.
	branchSuffix := branchFlag
	if branchSuffix == "" {
		if description != "" {
			branchSuffix = sanitizeBranchName(description)
		} else {
			branchSuffix = sanitizeBranchName(filepath.Base(prdFlag))
		}
	}
	branchName := cfg.Execution.BranchPrefix + branchSuffix

	// Create logger.
	logger, err := log.NewLogger(projectRoot)
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}

	fmt.Printf("Starting berth run: %s\n", branchName)
	fmt.Printf("Run directory: %s\n\n", runDir)

	// Phase 1: UNDERSTAND
	var reqs *understand.Requirements
	if prdFlag != "" {
		// Read PRD file directly.
		prdContent, readErr := os.ReadFile(prdFlag)
		if readErr != nil {
			return fmt.Errorf("reading PRD file: %w", readErr)
		}
		reqs = &understand.Requirements{
			Title:   branchName,
			Content: string(prdContent),
		}
		fmt.Println("Phase 1 UNDERSTAND: skipped (using PRD file)")
	} else {
		fmt.Println("Phase 1 UNDERSTAND: gathering requirements...")
		reqs, err = understand.RunUnderstand(
			*cfg,
			stackInfo,
			description,
			skipUnderstandFlag,
			runDir,
			"", // graphSummary - empty for now
		)
		if err != nil {
			return fmt.Errorf("understand phase: %w", err)
		}
		fmt.Printf("Phase 1 UNDERSTAND: complete (%s)\n\n", reqs.Title)
	}

	// Log understand complete.
	if logErr := logger.Append(log.LogEvent{
		Event:        log.EventUnderstandComplete,
		Title:        reqs.Title,
		Requirements: reqs.Content,
	}); logErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to log understand_complete: %v\n", logErr)
	}

	// Phase 2: PLAN
	fmt.Println("Phase 2 PLAN: generating execution plan...")

	// Convert understand.Requirements -> plan.Requirements.
	planReqs := &plan.Requirements{
		Title:   reqs.Title,
		Content: reqs.Content,
	}

	p, err := plan.RunPlan(*cfg, planReqs, "", runDir)
	if err != nil {
		return fmt.Errorf("plan phase: %w", err)
	}

	fmt.Printf("Phase 2 PLAN: approved (%d beads)\n", len(p.Beads))

	// Create beads from the plan.
	if beadErr := plan.CreateBeads(p); beadErr != nil {
		return fmt.Errorf("creating beads: %w", beadErr)
	}
	fmt.Printf("Created %d beads\n\n", len(p.Beads))

	// Log plan approved.
	if logErr := logger.Append(log.LogEvent{
		Event: log.EventPlanApproved,
		Title: p.Title,
		Beads: len(p.Beads),
	}); logErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to log plan_approved: %v\n", logErr)
	}

	// Phase 3: EXECUTE
	fmt.Println("Phase 3 EXECUTE: running beads...")
	if execErr := execute.RunExecute(*cfg, projectRoot, runDir, branchName); execErr != nil {
		fmt.Fprintf(os.Stderr, "Execute phase error: %v\n", execErr)
		// Continue to report phase even if execute had errors.
	}
	fmt.Println()

	// Phase 4: REPORT
	fmt.Println("Phase 4 REPORT: generating summary...")
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

// sanitizeBranchName converts a description into a valid git branch name.
// Lowercase, replace spaces/special chars with hyphens, truncate to 50 chars.
func sanitizeBranchName(s string) string {
	s = strings.ToLower(s)

	// Replace any non-alphanumeric character (except hyphen) with a hyphen.
	re := regexp.MustCompile(`[^a-z0-9-]+`)
	s = re.ReplaceAllString(s, "-")

	// Collapse multiple hyphens.
	re2 := regexp.MustCompile(`-{2,}`)
	s = re2.ReplaceAllString(s, "-")

	// Trim leading/trailing hyphens.
	s = strings.Trim(s, "-")

	// Truncate to 50 characters.
	if len(s) > 50 {
		s = s[:50]
		// Don't end on a hyphen after truncation.
		s = strings.TrimRight(s, "-")
	}

	if s == "" {
		s = "task"
	}

	return s
}
