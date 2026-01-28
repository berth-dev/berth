// explain.go implements the "Help me decide" side-call for ambiguous decisions.
package understand

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/berth-dev/berth/internal/detect"
)

// claudeOutputJSON is the envelope Claude returns with --output-format json.
type claudeOutputJSON struct {
	Type       string  `json:"type"`
	Subtype    string  `json:"subtype"`
	Result     string  `json:"result"`
	IsError    bool    `json:"is_error"`
	CostUSD    float64 `json:"cost_usd"`
	DurationMs int64   `json:"duration_ms"`
}

// RunExplain spawns a separate Claude process to explain the tradeoffs between
// the options of a question. It returns a short explanation recommending one
// option. This is invoked when the user selects "Help me decide" during the
// interview loop.
func RunExplain(question Question, stackInfo detect.StackInfo, graphSummary string) (string, error) {
	prompt := buildExplainPrompt(question, stackInfo, graphSummary)

	output, err := spawnClaude(prompt)
	if err != nil {
		return "", fmt.Errorf("explain: spawn claude: %w", err)
	}

	return output, nil
}

// buildExplainPrompt constructs the prompt for the explain side-call.
func buildExplainPrompt(q Question, stackInfo detect.StackInfo, graphSummary string) string {
	var sb strings.Builder

	sb.WriteString("Explain the tradeoffs between these options")
	if stackInfo.Language != "" || stackInfo.Framework != "" {
		sb.WriteString(" for a ")
		if stackInfo.Framework != "" {
			sb.WriteString(stackInfo.Framework)
			sb.WriteString(" ")
		}
		if stackInfo.Language != "" {
			sb.WriteString(stackInfo.Language)
		}
		sb.WriteString(" project")
	}
	sb.WriteString(":\n\n")

	sb.WriteString("Question: ")
	sb.WriteString(q.Text)
	sb.WriteString("\n\n")

	sb.WriteString("Options:\n")
	for _, opt := range q.Options {
		sb.WriteString("- ")
		sb.WriteString(opt.Label)
		if opt.Recommended {
			sb.WriteString(" (currently recommended)")
		}
		sb.WriteString("\n")
	}

	if graphSummary != "" {
		sb.WriteString("\nCodebase context:\n")
		sb.WriteString(graphSummary)
		sb.WriteString("\n")
	}

	sb.WriteString("\nKeep it to 2-3 sentences. Recommend one option and briefly explain why.")
	sb.WriteString("\nReturn ONLY a plain text explanation, no JSON.")

	return sb.String()
}

// spawnClaude runs `claude -p <prompt> --output-format json --dangerously-skip-permissions`
// and returns the result text from the JSON output envelope.
func spawnClaude(prompt string) (string, error) {
	cmd := exec.Command(
		"claude",
		"-p", prompt,
		"--allowedTools", "Read,Grep,Glob",
		"--output-format", "json",
		"--dangerously-skip-permissions",
	)

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("claude exited %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return "", fmt.Errorf("running claude: %w", err)
	}

	var envelope claudeOutputJSON
	if err := json.Unmarshal(out, &envelope); err != nil {
		return "", fmt.Errorf("parsing claude output: %w", err)
	}

	if envelope.IsError {
		return "", fmt.Errorf("claude returned error: %s", envelope.Result)
	}

	return strings.TrimSpace(envelope.Result), nil
}
