// parser.go parses plan.md output into bead structs.
package plan

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/tui"
)

// Plan represents a parsed execution plan consisting of multiple bead specifications.
type Plan struct {
	Title       string
	Description string
	Beads       []BeadSpec
	RawOutput   string // Original Claude output for "view details"
}

// BeadSpec defines a single bead (unit of work) within a plan.
type BeadSpec struct {
	ID          string
	Title       string
	Description string   // from the "context" field in the plan
	Files       []string
	DependsOn   []string
	VerifyExtra []string
}

// ParsePlan parses Claude's structured markdown plan output into a Plan struct.
// It extracts the plan title from the first heading, then parses each bead
// definition (### bt-N: Title) with its fields: files, context, depends, verify_extra.
// Returns an error if no beads are found.
func ParsePlan(output string) (*Plan, error) {
	plan := &Plan{
		RawOutput: output,
	}

	lines := strings.Split(output, "\n")

	// Extract title from first heading (# or ##) before any bead definitions
	var descLines []string
	titleFound := false
	beadStarted := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if this line starts a bead definition
		if isBeadHeading(trimmed) {
			beadStarted = true
			break
		}

		// Look for the plan title (first # heading)
		if !titleFound && strings.HasPrefix(trimmed, "# ") {
			plan.Title = strings.TrimPrefix(trimmed, "# ")
			titleFound = true
			continue
		}

		// Collect description lines between title and first bead
		if titleFound && trimmed != "" {
			descLines = append(descLines, trimmed)
		}
	}

	// Fallback: use first non-empty line as title
	if !titleFound {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				plan.Title = trimmed
				break
			}
		}
	}

	plan.Description = strings.Join(descLines, "\n")

	// Parse bead definitions
	if beadStarted {
		plan.Beads = parseBeads(lines)
	}

	if len(plan.Beads) == 0 {
		return nil, fmt.Errorf("no beads found in plan output")
	}

	return plan, nil
}

// isBeadHeading returns true if the line matches the pattern "### bt-N: Title".
func isBeadHeading(line string) bool {
	return strings.HasPrefix(line, "### bt-") || strings.HasPrefix(line, "###bt-")
}

// parseBeads extracts all BeadSpec definitions from the markdown lines.
func parseBeads(lines []string) []BeadSpec {
	var beads []BeadSpec
	var current *BeadSpec

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if isBeadHeading(trimmed) {
			// Save previous bead
			if current != nil {
				beads = append(beads, *current)
			}

			// Parse heading: "### bt-1: Title" or "###bt-1: Title"
			heading := strings.TrimPrefix(trimmed, "###")
			heading = strings.TrimSpace(heading)

			id, title := parseBeadHeading(heading)
			current = &BeadSpec{
				ID:    id,
				Title: title,
			}
			continue
		}

		// Parse fields within a bead
		if current != nil {
			parseBeadField(current, trimmed)
		}
	}

	// Save the last bead
	if current != nil {
		beads = append(beads, *current)
	}

	return beads
}

// parseBeadHeading extracts the bead ID and title from a heading like "bt-1: Title".
func parseBeadHeading(heading string) (string, string) {
	// Split on first ": " to separate ID from title
	parts := strings.SplitN(heading, ":", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	// Fallback: entire heading as both ID and title
	return strings.TrimSpace(heading), strings.TrimSpace(heading)
}

// parseBeadField parses a single field line within a bead definition.
func parseBeadField(bead *BeadSpec, line string) {
	// Match "- files:", "- context:", "- depends:", "- verify_extra:"
	if val, ok := extractField(line, "files"); ok {
		bead.Files = parseFilesList(val)
		return
	}
	if val, ok := extractField(line, "context"); ok {
		bead.Description = val
		return
	}
	if val, ok := extractField(line, "depends"); ok {
		bead.DependsOn = parseDependsList(val)
		return
	}
	if val, ok := extractField(line, "verify_extra"); ok {
		bead.VerifyExtra = parseVerifyExtra(val)
		return
	}
}

// extractField checks if the line matches "- fieldName: value" and returns the value.
func extractField(line, fieldName string) (string, bool) {
	prefix := fmt.Sprintf("- %s:", fieldName)
	if strings.HasPrefix(line, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(line, prefix)), true
	}

	// Also handle without leading dash (just "fieldName:")
	prefix2 := fmt.Sprintf("%s:", fieldName)
	if strings.HasPrefix(line, prefix2) {
		return strings.TrimSpace(strings.TrimPrefix(line, prefix2)), true
	}

	return "", false
}

// parseFilesList parses a bracketed comma-separated file list.
// Input: "[src/stores/auth.ts, src/components/Login.tsx]" or "src/stores/auth.ts, src/components/Login.tsx"
func parseFilesList(val string) []string {
	// Remove brackets
	val = strings.TrimPrefix(val, "[")
	val = strings.TrimSuffix(val, "]")

	// Try JSON array first
	var jsonFiles []string
	if err := json.Unmarshal([]byte("["+val+"]"), &jsonFiles); err == nil && len(jsonFiles) > 0 {
		// Check if the JSON parse produced reasonable results (not just a single
		// string that happened to parse as JSON)
		allValid := true
		for _, f := range jsonFiles {
			if strings.Contains(f, ",") {
				allValid = false
				break
			}
		}
		if allValid {
			return trimAll(jsonFiles)
		}
	}

	// Fallback: split by comma
	parts := strings.Split(val, ",")
	return trimAll(parts)
}

// parseDependsList parses a dependency list.
// Input: "none" -> empty, "bt-1, bt-2" -> ["bt-1", "bt-2"]
// Also handles bracketed form: "[bt-1, bt-2]" -> ["bt-1", "bt-2"]
func parseDependsList(val string) []string {
	lower := strings.ToLower(strings.TrimSpace(val))
	if lower == "none" || lower == "" || lower == "[]" || lower == "n/a" {
		return nil
	}

	// Remove brackets (handles single or double: "[bt-1, bt-2]" or "[[bt-1, bt-2]]")
	val = strings.TrimPrefix(val, "[")
	val = strings.TrimSuffix(val, "]")

	parts := strings.Split(val, ",")
	var deps []string
	for _, p := range parts {
		dep := strings.TrimSpace(p)
		if dep != "" {
			deps = append(deps, dep)
		}
	}
	return deps
}

// parseVerifyExtra parses verify_extra commands.
// Input: '["pnpm test -- --grep auth", "pnpm lint"]' -> ["pnpm test -- --grep auth", "pnpm lint"]
// Also handles comma-separated without JSON: "pnpm test, pnpm lint"
func parseVerifyExtra(val string) []string {
	val = strings.TrimSpace(val)

	// Try JSON array parse
	var cmds []string
	if err := json.Unmarshal([]byte(val), &cmds); err == nil {
		return trimAll(cmds)
	}

	// Try with brackets stripped and re-wrapped
	stripped := strings.TrimPrefix(val, "[")
	stripped = strings.TrimSuffix(stripped, "]")
	if err := json.Unmarshal([]byte("["+stripped+"]"), &cmds); err == nil && len(cmds) > 0 {
		return trimAll(cmds)
	}

	// Fallback: treat as empty or single command
	if val == "none" || val == "[]" || val == "" {
		return nil
	}

	// Split by comma as last resort, but only if no quotes present
	if !strings.Contains(val, "\"") {
		parts := strings.Split(stripped, ",")
		return trimAll(parts)
	}

	// Single command fallback
	return []string{stripped}
}

// trimAll trims whitespace from all strings in a slice and removes empty entries.
func trimAll(ss []string) []string {
	var result []string
	for _, s := range ss {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// ConvertToTUIPlan converts a plan.Plan to a tui.Plan for display in the TUI.
func ConvertToTUIPlan(p *Plan) *tui.Plan {
	tuiBeads := make([]tui.BeadSpec, len(p.Beads))
	for i, spec := range p.Beads {
		tuiBeads[i] = tui.BeadSpec{
			ID:          spec.ID,
			Title:       spec.Title,
			Description: spec.Description,
			Files:       spec.Files,
			DependsOn:   spec.DependsOn,
			VerifyExtra: spec.VerifyExtra,
		}
	}
	return &tui.Plan{
		Title:       p.Title,
		Description: p.Description,
		Beads:       tuiBeads,
		RawOutput:   p.RawOutput,
	}
}

// ConvertToExecutionBeads converts plan.BeadSpecs to beads.Bead slice for use
// with ComputeGroups and bead execution. Status is initialized to "open".
func ConvertToExecutionBeads(specs []BeadSpec) []beads.Bead {
	result := make([]beads.Bead, len(specs))
	for i, spec := range specs {
		result[i] = beads.Bead{
			ID:          spec.ID,
			Title:       spec.Title,
			Description: spec.Description,
			Status:      "open",
			DependsOn:   spec.DependsOn,
			Files:       spec.Files,
			VerifyExtra: spec.VerifyExtra,
		}
	}
	return result
}
