// Package plan implements Phase 2: spawning Claude to produce an execution plan.
// This file manages the planning Claude invocation and user approval loop.
package plan

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/context"
	"github.com/berth-dev/berth/internal/detect"
)

// Requirements represents the gathered requirements from the understand phase.
// Defined locally for decoupling from the understand package.
type Requirements struct {
	Title   string
	Content string
}

// claudeJSONOutput represents the JSON envelope returned by `claude -p --output-format json`.
type claudeJSONOutput struct {
	Type           string  `json:"type"`
	Subtype        string  `json:"subtype"`
	CostUSD        float64 `json:"cost_usd"`
	DurationMs     int     `json:"duration_ms"`
	DurationAPIMs  int     `json:"duration_api_ms"`
	IsError        bool    `json:"is_error"`
	NumTurns       int     `json:"num_turns"`
	Result         string  `json:"result"`
	SessionID      string  `json:"session_id"`
}

// RunPlan orchestrates the planning phase. It generates a plan prompt, spawns
// Claude to produce a plan, parses the output, and runs an interactive approval
// loop. Returns the approved plan or an error.
func RunPlan(cfg config.Config, requirements *Requirements, graphData string, runDir string, isGreenfield bool) (*Plan, error) {
	stackInfo := detect.StackInfo{
		Language:       cfg.Project.Language,
		Framework:      cfg.Project.Framework,
		PackageManager: cfg.Project.PackageManager,
	}

	learnings := context.ReadLearnings(runDir)

	var feedback string
	reader := bufio.NewReader(os.Stdin)

	for {
		prompt := BuildPlanPrompt(requirements, stackInfo, graphData, learnings, feedback, isGreenfield)

		fmt.Println("Generating plan with Claude...")
		rawOutput, err := spawnClaude(prompt)
		if err != nil {
			return nil, fmt.Errorf("spawning Claude for planning: %w", err)
		}

		plan, err := ParsePlan(rawOutput)
		if err != nil {
			return nil, fmt.Errorf("parsing plan output: %w\n\nClaude's raw response:\n%s", err, rawOutput)
		}

		if err := writePlan(runDir, rawOutput); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to persist plan: %v\n", err)
		}

		choice, err := presentApprovalUI(plan, reader)
		if err != nil {
			return nil, fmt.Errorf("reading user input: %w", err)
		}

		switch choice {
		case "1":
			fmt.Println("Plan approved. Creating beads...")
			return plan, nil
		case "2":
			fmt.Print("What should be changed? > ")
			line, err := reader.ReadString('\n')
			if err != nil {
				return nil, fmt.Errorf("reading feedback: %w", err)
			}
			feedback = strings.TrimSpace(line)
			fmt.Println("Re-planning with your feedback...")
		case "3":
			printPlanDetails(plan)
		default:
			fmt.Println("Invalid choice. Please enter 1, 2, or 3.")
		}
	}
}

// spawnClaude runs `claude -p` with the given prompt and returns the result
// text extracted from Claude's JSON output envelope.
func spawnClaude(prompt string) (string, error) {
	cmd := exec.Command(
		"claude",
		"-p", prompt,
		"--allowedTools", "Read,Grep,Glob",
		"--output-format", "json",
		"--dangerously-skip-permissions",
		"--model", "opus",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("claude command failed: %w: %s", err, output)
	}

	var envelope claudeJSONOutput
	if err := json.Unmarshal(output, &envelope); err != nil {
		return "", fmt.Errorf("parsing Claude JSON output: %w: %s", err, output)
	}

	if envelope.IsError {
		return "", fmt.Errorf("claude returned an error: %s", envelope.Result)
	}

	return envelope.Result, nil
}

// presentApprovalUI displays the plan summary and prompts the user for a choice.
// Returns the user's choice as a string ("1", "2", or "3").
func presentApprovalUI(plan *Plan, reader *bufio.Reader) (string, error) {
	fmt.Println()
	fmt.Println("+---------------------------------------------------------+")
	fmt.Printf("|  Plan: %s (%d beads)%s|\n",
		truncate(plan.Title, 35),
		len(plan.Beads),
		padding(55-len(fmt.Sprintf("  Plan: %s (%d beads)", truncate(plan.Title, 35), len(plan.Beads)))))
	fmt.Println("|                                                         |")

	for _, bead := range plan.Beads {
		deps := "no dep"
		if len(bead.DependsOn) > 0 {
			deps = strings.Join(bead.DependsOn, ", ")
		}
		line := fmt.Sprintf("  %s: %s", bead.ID, truncate(bead.Title, 35))
		depStr := fmt.Sprintf("[%s]", deps)
		spacesNeeded := 55 - len(line) - len(depStr)
		if spacesNeeded < 1 {
			spacesNeeded = 1
		}
		fmt.Printf("|%s%s%s|\n", line, strings.Repeat(" ", spacesNeeded), depStr)
	}

	fmt.Println("|                                                         |")
	fmt.Println("|  [1] Approve -- start execution                         |")
	fmt.Println("|  [2] Reject -- explain what to change (re-plans)        |")
	fmt.Println("|  [3] View details -- show full bead descriptions        |")
	fmt.Println("+---------------------------------------------------------+")
	fmt.Println()
	fmt.Print("Choice [1/2/3]: ")

	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// printPlanDetails prints the full plan output including all bead descriptions.
func printPlanDetails(plan *Plan) {
	fmt.Println()
	fmt.Println("=== Full Plan Details ===")
	fmt.Println()

	if plan.Description != "" {
		fmt.Println(plan.Description)
		fmt.Println()
	}

	for _, bead := range plan.Beads {
		fmt.Printf("### %s: %s\n", bead.ID, bead.Title)
		if bead.Description != "" {
			fmt.Printf("    Context: %s\n", bead.Description)
		}
		if len(bead.Files) > 0 {
			fmt.Printf("    Files: %s\n", strings.Join(bead.Files, ", "))
		}
		if len(bead.DependsOn) > 0 {
			fmt.Printf("    Depends: %s\n", strings.Join(bead.DependsOn, ", "))
		}
		if len(bead.VerifyExtra) > 0 {
			fmt.Printf("    Verify: %s\n", strings.Join(bead.VerifyExtra, ", "))
		}
		fmt.Println()
	}

	if plan.RawOutput != "" {
		fmt.Println("--- Raw Claude Output ---")
		fmt.Println(plan.RawOutput)
	}
	fmt.Println()
}

// truncate shortens s to maxLen characters, appending "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// padding returns a string of spaces to fill remaining width in the UI box.
func padding(n int) string {
	if n < 0 {
		n = 0
	}
	return strings.Repeat(" ", n)
}

// RunPlanNonInteractive generates a plan without the interactive approval loop.
// The TUI handles approval UI separately.
// If feedback is provided, it's incorporated into the prompt for re-planning.
func RunPlanNonInteractive(
	cfg config.Config,
	requirements *Requirements,
	graphData, runDir string,
	isGreenfield bool,
	feedback string,
) (*Plan, error) {
	stackInfo := detect.StackInfo{
		Language:       cfg.Project.Language,
		Framework:      cfg.Project.Framework,
		PackageManager: cfg.Project.PackageManager,
	}

	learnings := context.ReadLearnings(runDir)

	prompt := BuildPlanPrompt(requirements, stackInfo, graphData, learnings, feedback, isGreenfield)

	rawOutput, err := spawnClaude(prompt)
	if err != nil {
		return nil, fmt.Errorf("Claude failed: %w", err)
	}

	plan, err := ParsePlan(rawOutput)
	if err != nil {
		return nil, fmt.Errorf("parse failed: %w", err)
	}

	// Write plan to disk for persistence
	if err := writePlan(runDir, rawOutput); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to persist plan: %v\n", err)
	}

	return plan, nil
}

// writePlan persists the raw plan content to the run directory.
func writePlan(runDir string, content string) error {
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return fmt.Errorf("plan: creating run directory: %w", err)
	}

	path := filepath.Join(runDir, "plan.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("plan: writing plan: %w", err)
	}

	return nil
}
