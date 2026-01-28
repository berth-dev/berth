// prompt.go builds the plan-prompt content for the planning phase.
package plan

import (
	"fmt"
	"strings"

	"github.com/berth-dev/berth/internal/detect"
)

// scaffoldCmds maps framework names to their official scaffolding commands.
// Empty string means no scaffolding tool is available.
var scaffoldCmds = map[string]string{
	"next":    "npx create-next-app@latest . --yes --ts --eslint --tailwind --app --src-dir --use-npm --disable-git",
	"react":   "npx create-react-app . --template typescript",
	"vue":     "npm create vue@latest . -- --typescript --jsx --router --pinia",
	"svelte":  "npx sv create . --template minimal --types ts",
	"express": "",
	"node":    "",
}

func buildScaffoldingSection(framework string, isGreenfield bool) string {
	if !isGreenfield {
		return ""
	}
	cmd, ok := scaffoldCmds[framework]
	if !ok || cmd == "" {
		return ""
	}

	return fmt.Sprintf(`## Scaffolding Rules (CRITICAL for new projects)

The FIRST bead MUST use the official scaffolding tool to initialize the project.
Do NOT hand-write package.json, tsconfig.json, or config files.

Scaffold command: %s

Rules for the scaffold bead:
- Run the scaffold command above via Bash — it is fully non-interactive
- After scaffolding, install any additional dependencies via npm/pnpm
- The files: field should list only files you MODIFY after scaffolding (e.g., add better-sqlite3 to package.json)
- Set verify_extra: ["test -f package.json", "npx tsc --noEmit"] to confirm success
- ALL subsequent beads MUST use the directory structure created by the scaffolding tool

`, cmd)
}

// frameworkPractices maps framework names to their convention guidelines.
var frameworkPractices = map[string]string{
	"next": `## Next.js Conventions (MUST follow)
- Use src/ directory: src/app/ for routes, src/components/ for components, src/lib/ for utilities
- next.config.ts (TypeScript) with serverExternalPackages for Node-only deps like better-sqlite3
- Server Components by default; add "use client" only for interactive components
- API routes in src/app/api/
- Include ESLint configuration (created by scaffolding tool)
- Separate types into src/lib/types.ts`,

	"react": `## React Conventions
- Use src/ directory for all source code
- Components in src/components/, hooks in src/hooks/, utils in src/utils/
- Functional components with hooks only`,

	"vue": `## Vue Conventions
- Use src/ directory with src/components/, src/views/, src/composables/
- Use Composition API with script setup`,
}

func buildFrameworkPractices(framework string) string {
	practices, ok := frameworkPractices[framework]
	if !ok {
		return ""
	}
	return practices + "\n\n"
}

// BuildPlanPrompt constructs the full prompt for Claude to generate an execution
// plan. It incorporates requirements, stack information, Knowledge Graph data,
// accumulated learnings, and optional user feedback from a rejected plan.
func BuildPlanPrompt(requirements *Requirements, stackInfo detect.StackInfo, graphData string, learnings []string, feedback string, isGreenfield bool) string {
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

	// Scaffolding section for greenfield projects.
	scaffolding := buildScaffoldingSection(stackInfo.Framework, isGreenfield)
	if scaffolding != "" {
		b.WriteString(scaffolding)
	}

	// Framework-specific best practices.
	practices := buildFrameworkPractices(stackInfo.Framework)
	if practices != "" {
		b.WriteString(practices)
	}

	// Live version data from npm registry.
	versionData := DiscoverVersions(stackInfo.Framework)
	if versionData != "" {
		b.WriteString(versionData)
	}

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
