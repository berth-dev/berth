// conflict.go handles merge conflict resolution via Claude.
package execute

import (
	"context"
	"fmt"
	"strings"

	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/git"
)

// ConflictMergeResult holds the result of a conflict resolution attempt.
type ConflictMergeResult struct {
	Resolved   bool
	FilesFixed []string
	Error      error
}

// RunConflictMerge spawns Claude to resolve merge conflicts.
// If there are no conflicts, it returns Resolved=true immediately.
func RunConflictMerge(
	ctx context.Context,
	conflicts []git.MergeConflict,
	projectRoot string,
) ConflictMergeResult {
	if len(conflicts) == 0 {
		return ConflictMergeResult{Resolved: true}
	}

	prompt := buildConflictPrompt(conflicts)

	// Collect all conflicting files across all conflicts.
	var allFiles []string
	seen := make(map[string]bool)
	for _, c := range conflicts {
		for _, f := range c.Files {
			if !seen[f] {
				seen[f] = true
				allFiles = append(allFiles, f)
			}
		}
	}

	systemPrompt := `You are a merge conflict resolution agent.
Your task is to resolve Git merge conflicts intelligently, preserving the intent of both branches.
Work carefully and verify your changes compile/work before committing.`

	cfg := config.DefaultConfig()

	output, err := SpawnClaude(*cfg, systemPrompt, prompt, projectRoot, &SpawnClaudeOpts{
		WorkDir: projectRoot,
	})
	if err != nil {
		return ConflictMergeResult{
			Resolved: false,
			Error:    fmt.Errorf("spawning conflict resolver: %w", err),
		}
	}

	// Check output for success indicators.
	result := strings.ToLower(output.Result)
	resolved := strings.Contains(result, "resolved") ||
		strings.Contains(result, "committed") ||
		strings.Contains(result, "merge complete")

	if !resolved {
		return ConflictMergeResult{
			Resolved: false,
			Error:    fmt.Errorf("conflict resolution did not complete successfully: %s", output.Result),
		}
	}

	return ConflictMergeResult{
		Resolved:   true,
		FilesFixed: allFiles,
	}
}

// buildConflictPrompt formats the conflict description for Claude.
func buildConflictPrompt(conflicts []git.MergeConflict) string {
	var sb strings.Builder

	sb.WriteString("# Merge Conflict Resolution Task\n\n")
	sb.WriteString("The following merge conflicts need to be resolved:\n\n")

	for i, c := range conflicts {
		sb.WriteString(fmt.Sprintf("## Conflict %d\n", i+1))
		sb.WriteString(fmt.Sprintf("- **Bead ID**: %s\n", c.BeadID))
		sb.WriteString(fmt.Sprintf("- **Branch**: %s\n", c.Branch))
		sb.WriteString(fmt.Sprintf("- **Git Output**:\n```\n%s\n```\n", c.Output))
		sb.WriteString("- **Conflicting Files**:\n")
		for _, f := range c.Files {
			sb.WriteString(fmt.Sprintf("  - %s\n", f))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(`## Instructions

1. Read the conflicting files using the Read tool
2. Understand what each branch was trying to accomplish
3. Merge the changes intelligently, preserving both intents
4. Edit files to remove conflict markers (<<<<<<<, =======, >>>>>>>)
5. Stage the resolved files with git add
6. Run: git add -A && git commit -m "Resolve merge conflicts"

Important:
- Do NOT simply choose one side over the other unless that is clearly correct
- Preserve functionality from both branches when possible
- Ensure the resulting code compiles and is syntactically valid
- After committing, confirm the resolution was successful
`)

	return sb.String()
}
