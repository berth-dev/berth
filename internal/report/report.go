// Package report implements Phase 4: generating run summaries after execution completes.
package report

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/config"
	berthcontext "github.com/berth-dev/berth/internal/context"
	"github.com/berth-dev/berth/internal/git"
	"github.com/berth-dev/berth/internal/log"
)

// Report holds the aggregated statistics and metadata for a completed berth run.
type Report struct {
	Feature      string
	Branch       string
	TotalBeads   int
	Completed    int
	Stuck        int
	Skipped      int
	Commits      []string
	FilesChanged string // git diff --stat output
	Learnings    int
	Duration     time.Duration
	CostUSD      float64
}

// GenerateReport gathers all run data and produces a Report.
// It reads bead statuses, git information, log events, and learnings to
// build a comprehensive summary of the run. Non-critical failures (e.g.,
// git commands failing) are tolerated and result in empty values rather
// than errors.
func GenerateReport(cfg config.Config, projectRoot string, runDir string) (*Report, error) {
	r := &Report{
		Feature: cfg.Project.Name,
	}

	// Collect bead statistics.
	allBeads, err := beads.List()
	if err != nil {
		// Non-fatal: we proceed with zero counts.
		allBeads = nil
	}
	r.TotalBeads = len(allBeads)
	for _, b := range allBeads {
		switch b.Status {
		case "done":
			r.Completed++
		case "stuck":
			r.Stuck++
		case "open":
			r.Skipped++
		}
	}

	// Get current branch (best-effort).
	branch, err := git.CurrentBranch()
	if err == nil {
		r.Branch = branch
	}

	// Detect the base branch (e.g., "main" or "master").
	baseBranch := detectBaseBranch(projectRoot)

	// Get git log: commits since branch diverged from base.
	r.Commits = gitLogOneline(projectRoot, baseBranch)

	// Get git diff --stat output.
	r.FilesChanged = gitDiffStat(projectRoot, baseBranch)

	// Read learnings count.
	learnings := berthcontext.ReadLearnings(projectRoot)
	r.Learnings = len(learnings)

	// Calculate duration and cost from log events.
	logger, err := log.NewLogger(projectRoot)
	if err == nil {
		events, readErr := logger.ReadAll()
		if readErr == nil && len(events) > 0 {
			r.Duration = computeDuration(events)
			r.CostUSD = computeCost(events)
		}
	}

	// Write the report to the run directory.
	if writeErr := WriteReport(runDir, r); writeErr != nil {
		return r, fmt.Errorf("writing report: %w", writeErr)
	}

	return r, nil
}

// FormatReport produces a terminal-friendly, human-readable summary string.
func FormatReport(r *Report) string {
	var b strings.Builder

	b.WriteString("========================================\n")
	b.WriteString("  Berth Run Report\n")
	b.WriteString("========================================\n")
	b.WriteString("\n")

	if r.Feature != "" {
		fmt.Fprintf(&b, "Feature:     %s\n", r.Feature)
	}
	if r.Branch != "" {
		fmt.Fprintf(&b, "Branch:      %s\n", r.Branch)
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "Beads:       %d total\n", r.TotalBeads)
	fmt.Fprintf(&b, "  Completed: %d\n", r.Completed)
	fmt.Fprintf(&b, "  Stuck:     %d\n", r.Stuck)
	fmt.Fprintf(&b, "  Skipped:   %d\n", r.Skipped)
	b.WriteString("\n")

	if len(r.Commits) > 0 {
		b.WriteString("Commits:\n")
		for _, c := range r.Commits {
			fmt.Fprintf(&b, "  - %s\n", c)
		}
		b.WriteString("\n")
	}

	if r.FilesChanged != "" {
		b.WriteString("Files Changed:\n")
		for _, line := range strings.Split(r.FilesChanged, "\n") {
			if strings.TrimSpace(line) != "" {
				fmt.Fprintf(&b, "  %s\n", line)
			}
		}
		b.WriteString("\n")
	}

	if r.Learnings > 0 {
		fmt.Fprintf(&b, "Learnings:   %d new entries\n", r.Learnings)
		b.WriteString("\n")
	}

	if r.Duration > 0 {
		fmt.Fprintf(&b, "Duration:    %s\n", formatDuration(r.Duration))
	}

	if r.CostUSD > 0 {
		fmt.Fprintf(&b, "Cost:        $%.2f\n", r.CostUSD)
	}

	b.WriteString("========================================\n")

	return b.String()
}

// WriteReport writes the formatted report to {runDir}/report.md.
// Creates the run directory if it does not exist.
func WriteReport(runDir string, report *Report) error {
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return fmt.Errorf("creating run directory: %w", err)
	}

	content := FormatReport(report)
	path := filepath.Join(runDir, "report.md")

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing report file: %w", err)
	}

	return nil
}

// detectBaseBranch returns the default branch name by inspecting
// refs/remotes/origin/HEAD. Falls back to "main" on any failure.
func detectBaseBranch(projectRoot string) string {
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return "main"
	}
	// Output is like "refs/remotes/origin/main\n"
	ref := strings.TrimSpace(string(out))
	parts := strings.Split(ref, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return "main"
}

// gitLogOneline returns commit messages from baseBranch..HEAD as a string slice.
// Returns an empty slice on any failure.
func gitLogOneline(projectRoot string, baseBranch string) []string {
	cmd := exec.Command("git", "log", "--oneline", baseBranch+"..HEAD")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil
	}

	return strings.Split(raw, "\n")
}

// gitDiffStat returns the output of git diff --stat baseBranch..HEAD.
// Returns an empty string on any failure.
func gitDiffStat(projectRoot string, baseBranch string) string {
	cmd := exec.Command("git", "diff", "--stat", baseBranch+"..HEAD")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(out))
}

// computeDuration calculates the total run duration from log events.
// It looks for the first run_started event and uses either the last
// run_complete event or the last event's timestamp as the end point.
func computeDuration(events []log.LogEvent) time.Duration {
	if len(events) == 0 {
		return 0
	}

	var start time.Time
	var end time.Time

	for _, e := range events {
		if e.Event == log.EventRunStarted && start.IsZero() {
			start = e.Time
		}
		// Track the latest event time as a fallback end.
		if !e.Time.IsZero() {
			end = e.Time
		}
		// Prefer an explicit run_complete event as the end.
		if e.Event == log.EventRunComplete {
			end = e.Time
		}
	}

	if start.IsZero() || end.IsZero() {
		return 0
	}

	d := end.Sub(start)
	if d < 0 {
		return 0
	}

	return d
}

// computeCost sums the CostUSD field across all log events.
func computeCost(events []log.LogEvent) float64 {
	var total float64
	for _, e := range events {
		total += e.CostUSD
	}
	return total
}

// formatDuration produces a human-readable duration string such as "5m 32s"
// or "1h 12m 5s". Sub-second durations are shown as "< 1s".
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "< 1s"
	}

	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	switch {
	case h > 0:
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	case m > 0:
		return fmt.Sprintf("%dm %ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}
