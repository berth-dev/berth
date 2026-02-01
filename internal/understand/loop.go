// Package understand implements Phase 1: the interview loop that gathers requirements.
// This file drives the interview loop, spawning claude -p per round.
package understand

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/detect"
)

// maxRounds is a safety cap to prevent infinite interview loops.
const maxRounds = 10

// Question represents a single question posed to the user during the
// interview. Each question has numbered options and optional flags for custom
// input and the "Help me decide" explanation feature.
type Question struct {
	ID          string   `json:"id"`
	Text        string   `json:"text"`
	Options     []Option `json:"options"`
	AllowCustom bool     `json:"allow_custom"`
	AllowHelp   bool     `json:"allow_help"`
}

// Option is one selectable choice within a Question.
type Option struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Recommended bool   `json:"recommended,omitempty"`
}

// Round stores the questions and answers for a single interview round.
type Round struct {
	Questions []Question
	Answers   []Answer
}

// Answer holds the user's response to a single Question.
type Answer struct {
	ID    string
	Value string
}

// UnderstandResponse is the JSON schema that Claude returns each round.
// When Done is true, RequirementsMD contains the final requirements document.
// When Done is false, Questions contains the next set of questions.
type UnderstandResponse struct {
	Done           bool       `json:"done"`
	Context        string     `json:"context,omitempty"`
	Questions      []Question `json:"questions,omitempty"`
	RequirementsMD string     `json:"requirements_md,omitempty"`
}

// Requirements is the output of the understand phase: a title and the full
// markdown content of the requirements document.
type Requirements struct {
	Title   string
	Content string // The full markdown content
}

// RunUnderstand drives the interview loop to gather requirements from the user.
//
// If skipUnderstand is true, the description is used directly without any
// interview rounds. Otherwise, Claude is spawned once per round to generate
// questions, and the loop continues until Claude signals done or the safety
// cap is reached.
//
// runDir is the path to the current run directory (e.g. .berth/runs/<id>)
// where requirements.md will be written.
func RunUnderstand(cfg config.Config, stackInfo detect.StackInfo, description string, skipUnderstand bool, runDir string, graphSummary string) (*Requirements, error) {
	if skipUnderstand {
		return buildSkipRequirements(description, runDir)
	}

	return runInterviewLoop(cfg, stackInfo, description, runDir, graphSummary)
}

// buildSkipRequirements creates a Requirements directly from the raw
// description, skipping the interview entirely.
func buildSkipRequirements(description string, runDir string) (*Requirements, error) {
	title := extractTitle(description)
	content := fmt.Sprintf("# Requirements: %s\n\n## Description\n%s\n", title, description)

	if err := writeRequirements(runDir, content); err != nil {
		return nil, err
	}

	return &Requirements{
		Title:   title,
		Content: content,
	}, nil
}

// runInterviewLoop is the core loop that spawns Claude once per round.
func runInterviewLoop(cfg config.Config, stackInfo detect.StackInfo, description string, runDir string, graphSummary string) (*Requirements, error) {
	var rounds []Round

	for round := 1; round <= maxRounds; round++ {
		fmt.Printf("\n--- Interview Round %d ---\n", round)

		// Build the prompt with accumulated history.
		prompt := BuildUnderstandPrompt(round, rounds, stackInfo, graphSummary, description)

		// Spawn Claude to generate questions or final requirements.
		output, err := spawnClaude(prompt)
		if err != nil {
			return nil, fmt.Errorf("understand round %d: %w", round, err)
		}

		// Parse Claude's response. The output might contain markdown fences
		// or leading/trailing whitespace; try to extract valid JSON.
		cleaned := cleanJSONOutput(output)

		var resp UnderstandResponse
		if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
			return nil, fmt.Errorf("understand round %d: parsing response: %w\nRaw output:\n%s", round, err, output)
		}

		// If Claude signals done, write the requirements and return.
		if resp.Done {
			return finalize(resp, runDir)
		}

		// Not done: display questions and collect answers.
		if len(resp.Questions) == 0 {
			return nil, fmt.Errorf("understand round %d: claude returned done=false but no questions", round)
		}

		if resp.Context != "" {
			fmt.Printf("\nContext: %s\n", resp.Context)
		}

		answers := displayAndCollectAnswers(resp.Questions, stackInfo, graphSummary)

		rounds = append(rounds, Round{
			Questions: resp.Questions,
			Answers:   answers,
		})
	}

	return nil, fmt.Errorf("understand: reached maximum rounds (%d) without completion", maxRounds)
}

// displayAndCollectAnswers shows questions to the user, handles "Help me
// decide" requests, and returns the final answers.
func displayAndCollectAnswers(questions []Question, stackInfo detect.StackInfo, graphSummary string) []Answer {
	answers := DisplayQuestions(questions)

	// Post-process: handle "Help me decide" selections.
	for i, a := range answers {
		if a.Value != helpMeDecideValue {
			continue
		}

		// Find the matching question.
		var q Question
		for _, question := range questions {
			if question.ID == a.ID {
				q = question
				break
			}
		}

		explanation, err := RunExplain(q, stackInfo, graphSummary)
		if err != nil {
			fmt.Printf("\n  (Could not get explanation: %v)\n", err)
		} else {
			fmt.Printf("\n  Explanation: %s\n", explanation)
		}

		// Re-prompt for this specific question after showing the explanation.
		fmt.Printf("\nNow choose for: %s\n", q.Text)
		reAnswers := DisplayQuestions([]Question{q})
		if len(reAnswers) > 0 {
			answers[i] = reAnswers[0]
		}
	}

	return answers
}

// finalize writes the requirements markdown to disk and returns the
// Requirements struct.
func finalize(resp UnderstandResponse, runDir string) (*Requirements, error) {
	content := resp.RequirementsMD
	if content == "" {
		return nil, fmt.Errorf("understand: claude signaled done but requirements_md is empty")
	}

	title := extractTitle(content)

	if err := writeRequirements(runDir, content); err != nil {
		return nil, err
	}

	fmt.Printf("\nRequirements written to %s\n", filepath.Join(runDir, "requirements.md"))

	return &Requirements{
		Title:   title,
		Content: content,
	}, nil
}

// writeRequirements creates the run directory if needed and writes the
// requirements markdown file.
func writeRequirements(runDir string, content string) error {
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return fmt.Errorf("understand: creating run directory: %w", err)
	}

	path := filepath.Join(runDir, "requirements.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("understand: writing requirements: %w", err)
	}

	return nil
}

// extractTitle attempts to pull a title from a markdown requirements document.
// It looks for the first H1 heading. Falls back to "Untitled Task".
func extractTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			title := strings.TrimPrefix(trimmed, "# ")
			// Strip common prefixes like "Requirements: ".
			title = strings.TrimPrefix(title, "Requirements: ")
			title = strings.TrimPrefix(title, "Requirements:")
			return strings.TrimSpace(title)
		}
	}
	return "Untitled Task"
}

// cleanJSONOutput extracts JSON from Claude's output, handling cases where
// the model includes explanatory text before/after the JSON or wraps it in
// markdown code fences.
func cleanJSONOutput(s string) string {
	s = strings.TrimSpace(s)

	// Try to extract JSON from markdown code fences first.
	if idx := strings.Index(s, "```json"); idx != -1 {
		s = s[idx+7:] // Skip "```json"
		if endIdx := strings.Index(s, "```"); endIdx != -1 {
			s = s[:endIdx]
		}
		return strings.TrimSpace(s)
	}
	if idx := strings.Index(s, "```"); idx != -1 {
		s = s[idx+3:] // Skip "```"
		// Skip optional language identifier on same line.
		if nlIdx := strings.Index(s, "\n"); nlIdx != -1 && nlIdx < 20 {
			s = s[nlIdx+1:]
		}
		if endIdx := strings.Index(s, "```"); endIdx != -1 {
			s = s[:endIdx]
		}
		return strings.TrimSpace(s)
	}

	// No code fence found. Try to find JSON object boundaries.
	// Look for '{"' to avoid matching braces in prose like "{see below}".
	start := strings.Index(s, `{"`)
	if start == -1 {
		// Fallback to any '{' if no '{"' found (e.g., empty object or array).
		start = strings.Index(s, "{")
	}
	end := strings.LastIndex(s, "}")
	if start != -1 && end != -1 && end > start {
		return s[start : end+1]
	}

	return strings.TrimSpace(s)
}
