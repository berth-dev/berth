// output.go parses Claude JSON output from completed bead executions.
package execute

import (
	"encoding/json"
	"fmt"
)

// ClaudeOutput holds the parsed result from a Claude CLI invocation
// using --output-format json.
type ClaudeOutput struct {
	Result     string  `json:"result"`
	CostUSD    float64 `json:"cost_usd"`
	DurationMS int64   `json:"duration_ms"`
	SessionID  string  `json:"session_id"`
	IsError    bool    `json:"is_error"`
}

// claudeRawOutput is the full JSON envelope returned by Claude CLI
// with --output-format json.
type claudeRawOutput struct {
	Type       string  `json:"type"`
	Subtype    string  `json:"subtype"`
	Result     string  `json:"result"`
	CostUSD    float64 `json:"cost_usd"`
	DurationMS int64   `json:"duration_ms"`
	SessionID  string  `json:"session_id"`
	IsError    bool    `json:"is_error"`
	NumTurns   int     `json:"num_turns"`
}

// ParseClaudeOutput parses the raw JSON bytes from Claude's
// --output-format json response into a ClaudeOutput.
func ParseClaudeOutput(raw []byte) (*ClaudeOutput, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty claude output")
	}

	var rawOut claudeRawOutput
	if err := json.Unmarshal(raw, &rawOut); err != nil {
		return nil, fmt.Errorf("parsing claude output: %w", err)
	}

	// Validate the response type.
	if rawOut.Type != "result" {
		return nil, fmt.Errorf("unexpected claude output type: %q (expected \"result\")", rawOut.Type)
	}

	return &ClaudeOutput{
		Result:     rawOut.Result,
		CostUSD:    rawOut.CostUSD,
		DurationMS: rawOut.DurationMS,
		SessionID:  rawOut.SessionID,
		IsError:    rawOut.IsError,
	}, nil
}
