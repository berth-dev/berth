// prompt.go builds the plan-prompt content for the planning phase.
package plan

import (
	"fmt"
	"strings"

	"github.com/berth-dev/berth/internal/detect"
)

// BuildPlanPrompt constructs the full prompt for Claude to generate an execution
// plan. It incorporates requirements, stack information, Knowledge Graph data,
// accumulated learnings, and optional user feedback from a rejected plan.
func BuildPlanPrompt(requirements *Requirements, stackInfo detect.StackInfo, graphData string, learnings []string, feedback string) string {
	var b strings.Builder

	b.WriteString("# Task: Create an Execution Plan\n\n")

	// Requirements section
	b.WriteString("## Requirements\n\n")
	if requirements.Title != "" {
		b.WriteString(fmt.Sprintf("**Title:** %s\n\n", requirements.Title))
	}
	b.WriteString(requirements.Content)
	b.WriteString("\n\n")

	// Project stack context
	b.WriteString("## Project Stack\n\n")
	if stackInfo.Language != "" {
		b.WriteString(fmt.Sprintf("- Language: %s\n", stackInfo.Language))
	}
	if stackInfo.Framework != "" {
		b.WriteString(fmt.Sprintf("- Framework: %s\n", stackInfo.Framework))
	}
	if stackInfo.PackageManager != "" {
		b.WriteString(fmt.Sprintf("- Package Manager: %s\n", stackInfo.PackageManager))
	}
	if stackInfo.TestCmd != "" {
		b.WriteString(fmt.Sprintf("- Test Command: %s\n", stackInfo.TestCmd))
	}
	if stackInfo.BuildCmd != "" {
		b.WriteString(fmt.Sprintf("- Build Command: %s\n", stackInfo.BuildCmd))
	}
	if stackInfo.LintCmd != "" {
		b.WriteString(fmt.Sprintf("- Lint Command: %s\n", stackInfo.LintCmd))
	}
	b.WriteString("\n")

	// Knowledge Graph data (pre-embedded code context)
	if graphData != "" {
		b.WriteString("## Existing Code Context (from Knowledge Graph)\n\n")
		b.WriteString(graphData)
		b.WriteString("\n\n")
	}

	// Accumulated learnings from previous runs
	if len(learnings) > 0 {
		b.WriteString("## Learnings from Previous Runs\n\n")
		for _, l := range learnings {
			b.WriteString(fmt.Sprintf("- %s\n", l))
		}
		b.WriteString("\n")
	}

	// User feedback from a rejected plan
	if feedback != "" {
		b.WriteString("## User Feedback\n\n")
		b.WriteString("The previous plan was rejected. The user provided this feedback:\n\n")
		b.WriteString(fmt.Sprintf("> %s\n\n", feedback))
		b.WriteString("Please revise the plan to address this feedback.\n\n")
	}

	// Plan phase rules and output format instructions
	b.WriteString(`## PLAN Phase Rules

- Break work into as many beads as needed (no artificial cap)
- Each bead must:
  - Touch <=5 files
  - Have exactly 1 responsibility
  - Be describable in a short paragraph
  - List exact files to modify (from Knowledge Graph)
  - Include context about what already exists in those files
  - Define dependencies on other beads
- If a bead touches >5 files, split it
- Define verify_extra commands per bead (beyond default pipeline)
- Output: plan in structured markdown with bead definitions

## Output Format

You MUST output the plan in this exact structured markdown format.

Start with a top-level heading for the plan title, then optionally a description paragraph.
Then define each bead using this template:

### bt-1: Short title describing the bead
- files: [path/to/file1.ts, path/to/file2.ts]
- context: Description of what already exists in these files and what this bead should do. This should be a short paragraph.
- depends: none
- verify_extra: ["command1", "command2"]

### bt-2: Another bead title
- files: [path/to/file3.ts]
- context: What exists and what to change.
- depends: bt-1
- verify_extra: ["command1"]

Rules for the output:
- Number beads sequentially: bt-1, bt-2, bt-3, etc.
- The "files" field is a bracketed comma-separated list of file paths
- The "context" field is a short paragraph (becomes the bead description)
- The "depends" field is either "none" or a comma-separated list of bead IDs (e.g., "bt-1, bt-2")
- The "verify_extra" field is a JSON array of shell commands to run for verification beyond the default pipeline
- Each bead MUST have all four fields: files, context, depends, verify_extra

Output ONLY the structured plan markdown. Do not include any other text, explanations, or commentary outside the plan structure.
Return the plan as your text response. Do NOT write it to a file.
`)

	return b.String()
}
