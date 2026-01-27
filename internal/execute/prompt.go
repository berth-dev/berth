// prompt.go builds per-bead executor prompts with pre-embedded graph data.
package execute

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/prompts"
)

// CompletedDep represents a completed dependency with its title and summary.
type CompletedDep struct {
	Title   string
	Summary string
}

// promptData holds the template data for executor task prompt rendering.
type promptData struct {
	Title            string
	ID               string
	Priority         string
	Description      string
	Files            []string
	Verify           string
	Done             string
	CompletedDeps    []CompletedDep
	RelevantPatterns string
	LockedDecisions  string
}

// BuildExecutorPrompt renders the executor task template with bead data,
// graph context, and learnings. If attempt > 1, previous error context is
// appended. If diagnosis is non-nil, the diagnostic analysis is included.
func BuildExecutorPrompt(bead *beads.Bead, attempt int, diagnosis *string, graphData string, learnings []string) string {
	data := promptData{
		Title:       bead.Title,
		ID:          bead.ID,
		Priority:    "normal",
		Description: bead.Description,
		Files:       bead.Files,
	}

	tmpl, err := template.New("executor_task").Parse(prompts.ExecutorTaskTemplate)
	if err != nil {
		// Template is embedded at compile time; parse failure is a bug.
		return fmt.Sprintf("ERROR: failed to parse executor task template: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Sprintf("ERROR: failed to execute executor task template: %v", err)
	}

	var sections []string
	sections = append(sections, buf.String())

	// Append graph data if available.
	if graphData != "" {
		sections = append(sections, graphData)
	}

	// Append learnings from previous beads.
	if len(learnings) > 0 {
		var lb strings.Builder
		lb.WriteString("## Accumulated Learnings\n")
		for _, l := range learnings {
			lb.WriteString("- ")
			lb.WriteString(l)
			lb.WriteString("\n")
		}
		sections = append(sections, lb.String())
	}

	// Append retry context for attempts beyond the first.
	if attempt > 1 {
		sections = append(sections, fmt.Sprintf(
			"## Retry Context\nThis is attempt %d. Previous attempts failed verification. "+
				"Review the errors carefully and take a different approach if needed.\n",
			attempt,
		))
	}

	// Append diagnostic analysis if provided.
	if diagnosis != nil && *diagnosis != "" {
		sections = append(sections, fmt.Sprintf(
			"## Diagnostic Analysis\nA diagnostic session analyzed the failures and recommends:\n%s\n",
			*diagnosis,
		))
	}

	return strings.Join(sections, "\n")
}
