// Package context manages persistent context files for Claude sessions.
// This file generates and updates .berth/CLAUDE.md.
package context

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/detect"
)

// claudeMDTemplate is the Go text/template for the generated CLAUDE.md file.
const claudeMDTemplate = `# Berth Executor Context

## Project
- Name: {{.ProjectName}}
- Stack: {{.Stack}}
- Package Manager: {{.PackageManager}}
- Test Command: {{.TestCommand}}

## Your Role
You are a Berth executor. You receive ONE bead (task) at a time.
- Implement the bead as described
- Query the Knowledge Graph MCP to understand code relationships
- Use existing types, functions, and patterns - don't create duplicates
- Run verification when done
- Report what you learned

## Coding Patterns
{{range .Patterns}}- {{.}}
{{end}}
## Verification Pipeline
After implementing, run in order:
{{range $i, $cmd := .VerifyCommands}}{{$i | inc}}. {{$cmd}}
{{end}}All must pass before committing.

## Accumulated Learnings
{{range .Learnings}}- {{.}}
{{end}}
## File Editing Rules
- **Always prefer Edit over Write for existing files.** Only use Write for new files. This prevents accidentally overwriting existing content and ensures Claude makes surgical changes.
- ALWAYS use Edit (str_replace) for modifying existing files
- NEVER rewrite entire files when changing a few lines
{{if .KGTools}}
## Knowledge Graph MCP Tools Available
You have access to a Knowledge Graph MCP with these tools:
- get_callers(name) -- WHO calls this function? USE BEFORE modifying a function signature.
- get_callees(name) -- WHAT does this function call? USE to understand implementation.
- get_dependents(name) -- WHAT breaks if this changes? USE BEFORE deleting or renaming.
- get_exports(file) -- List exports from a file. USE to avoid creating duplicates.
- get_importers(file) -- WHO imports from this file? USE to assess blast radius.
- get_type_usages(type) -- Where is this type used? USE BEFORE adding fields or new types.

MANDATORY QUERIES:
- Before creating a new function/type: get_exports on the target file
- Before changing a function signature: get_callers on that function
- Before modifying a file: get_importers on that file

Note: Most structural context is PRE-EMBEDDED in your prompt by Berth.
Use these tools for AD-HOC queries when you discover something unexpected.
{{end}}
## Rules
- NEVER create new types if one already exists (query Knowledge Graph)
- NEVER modify function signatures without checking callers (query Knowledge Graph)
- Commit with conventional format: feat(berth): description
- Report learnings at end of execution
`

// claudeMDData holds the template data for CLAUDE.md generation.
type claudeMDData struct {
	ProjectName    string
	Stack          string
	PackageManager string
	TestCommand    string
	Patterns       []string
	VerifyCommands []string
	Learnings      []string
	KGTools        []string
}

// GenerateCLAUDEMD generates the content for .berth/CLAUDE.md using the
// provided configuration, stack information, accumulated learnings, and
// available Knowledge Graph tools.
func GenerateCLAUDEMD(cfg config.Config, stackInfo detect.StackInfo, learnings []string, kgTools []string) string {
	funcMap := template.FuncMap{
		"inc": func(i int) int { return i + 1 },
	}

	tmpl, err := template.New("claude_md").Funcs(funcMap).Parse(claudeMDTemplate)
	if err != nil {
		// Template is a compile-time constant; a parse error is a bug.
		panic(fmt.Sprintf("parsing CLAUDE.md template: %v", err))
	}

	data := claudeMDData{
		ProjectName:    cfg.Project.Name,
		Stack:          stackInfo.Language + "/" + stackInfo.Framework,
		PackageManager: stackInfo.PackageManager,
		TestCommand:    stackInfo.TestCmd,
		Patterns:       derivePatterns(stackInfo),
		VerifyCommands: cfg.VerifyPipeline,
		Learnings:      learnings,
		KGTools:        kgTools,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("executing CLAUDE.md template: %v", err))
	}

	return buf.String()
}

// WriteCLAUDEMD writes content to {dir}/.berth/CLAUDE.md, creating the
// .berth/ directory if it does not exist.
func WriteCLAUDEMD(dir string, content string) error {
	berthDir := filepath.Join(dir, ".berth")
	if err := os.MkdirAll(berthDir, 0755); err != nil {
		return fmt.Errorf("creating .berth directory: %w", err)
	}

	path := filepath.Join(berthDir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing CLAUDE.md: %w", err)
	}

	return nil
}

// derivePatterns returns coding pattern guidelines based on the detected stack.
func derivePatterns(stackInfo detect.StackInfo) []string {
	lang := strings.ToLower(stackInfo.Language)

	switch lang {
	case "go":
		return []string{
			"Use standard library idioms",
			"Error returns, not exceptions",
			"Accept interfaces, return structs",
			"Table-driven tests",
		}
	case "typescript", "javascript":
		return []string{
			"Use async/await over raw promises",
			"Prefer const over let; avoid var",
			"Use strict TypeScript types, avoid any",
			"Write unit tests with describe/it blocks",
		}
	case "python":
		return []string{
			"Follow PEP 8 style guidelines",
			"Use type hints for function signatures",
			"Prefer list comprehensions where readable",
			"Write tests with pytest conventions",
		}
	case "rust":
		return []string{
			"Use Result and Option types for error handling",
			"Prefer references over cloning",
			"Write idiomatic Rust with pattern matching",
			"Use cargo test for testing",
		}
	case "java":
		return []string{
			"Follow standard Java naming conventions",
			"Use dependency injection where appropriate",
			"Prefer composition over inheritance",
			"Write JUnit tests",
		}
	default:
		return []string{
			"Follow project conventions",
			"Write clear, maintainable code",
			"Include tests for new functionality",
		}
	}
}
