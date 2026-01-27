// diagnostic.go spawns a diagnostic Claude session for analyzing failures.
package execute

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"text/template"
	"time"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/prompts"
)

// diagnosticData holds the template data for the diagnostic prompt.
type diagnosticData struct {
	Title       string
	ID          string
	Description string
	Error1      string
	Error2      string
	Error3      string
}

// RunDiagnostic spawns a non-interactive Claude session to analyze why a
// bead has failed 3 consecutive times. It renders the diagnostic template
// with the bead metadata and error outputs, then parses Claude's JSON
// response to extract the diagnosis.
func RunDiagnostic(cfg config.Config, bead *beads.Bead, errors []string, projectRoot string) (string, error) {
	prompt, err := buildDiagnosticPrompt(bead, errors)
	if err != nil {
		return "", fmt.Errorf("building diagnostic prompt: %w", err)
	}

	raw, err := spawnDiagnosticClaude(cfg, prompt, projectRoot)
	if err != nil {
		return "", fmt.Errorf("spawning diagnostic claude: %w", err)
	}

	// Parse Claude's wrapper output to extract the result field.
	output, err := ParseClaudeOutput(raw)
	if err != nil {
		// If parsing fails, return the raw output as the diagnosis.
		return string(raw), nil
	}

	if output.IsError {
		return "", fmt.Errorf("diagnostic claude returned error: %s", output.Result)
	}

	return output.Result, nil
}

// buildDiagnosticPrompt renders the diagnostic template with bead data
// and error outputs from the 3 failed attempts.
func buildDiagnosticPrompt(bead *beads.Bead, errors []string) (string, error) {
	tmpl, err := template.New("diagnostic").Parse(prompts.DiagnosticTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing diagnostic template: %w", err)
	}

	// Ensure we have 3 error strings, padding with empty if fewer.
	padded := make([]string, 3)
	for i := 0; i < 3; i++ {
		if i < len(errors) {
			padded[i] = errors[i]
		} else {
			padded[i] = "(no output captured)"
		}
	}

	data := diagnosticData{
		Title:       bead.Title,
		ID:          bead.ID,
		Description: bead.Description,
		Error1:      padded[0],
		Error2:      padded[1],
		Error3:      padded[2],
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing diagnostic template: %w", err)
	}

	return buf.String(), nil
}

// spawnDiagnosticClaude runs `claude -p` with the diagnostic prompt and
// returns the raw output bytes. It enforces a timeout derived from
// cfg.Execution.TimeoutPerBead, matching the pattern in spawner.go.
func spawnDiagnosticClaude(cfg config.Config, prompt string, projectRoot string) ([]byte, error) {
	timeout := time.Duration(cfg.Execution.TimeoutPerBead) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "claude", "-p", prompt, "--output-format", "json", "--dangerously-skip-permissions")
	cmd.Dir = projectRoot

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("claude diagnostic timed out after %s: %w", timeout, ctx.Err())
		}
		return nil, fmt.Errorf("claude diagnostic exited with error: %w: %s", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// DiagnosticResult represents the parsed JSON output from a diagnostic
// Claude session.
type DiagnosticResult struct {
	RootCause         string   `json:"rootCause"`
	Fix               string   `json:"fix"`
	Misconceptions    []string `json:"misconceptions"`
	AdditionalContext string   `json:"additionalContext"`
}

// ParseDiagnosticResult parses the diagnostic JSON string into a
// structured result. Returns an error if the JSON is malformed.
func ParseDiagnosticResult(raw string) (*DiagnosticResult, error) {
	var result DiagnosticResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("parsing diagnostic result: %w", err)
	}
	return &result, nil
}
