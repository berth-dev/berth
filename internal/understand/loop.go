// Package understand implements Phase 1: the interview loop that gathers requirements.
// This file drives the interview loop, spawning claude -p per round.
package understand

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/detect"
	"github.com/berth-dev/berth/internal/log"
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
//
// The logger parameter is optional; if provided, approval choices are logged.
func RunUnderstand(cfg config.Config, stackInfo detect.StackInfo, description string, skipUnderstand bool, runDir string, graphSummary string, logger *log.Logger) (*Requirements, error) {
	if skipUnderstand {
		return buildSkipRequirements(description, runDir)
	}

	return runInterviewLoop(cfg, stackInfo, description, runDir, graphSummary, logger)
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
// After requirements are gathered, presents an approval gate with options:
// accept, interview more, or chat about the plan.
func runInterviewLoop(cfg config.Config, stackInfo detect.StackInfo, description string, runDir string, graphSummary string, logger *log.Logger) (*Requirements, error) {
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

		// If Claude signals done, present approval gate before finalizing.
		if resp.Done {
			reqs, err := finalize(resp, runDir)
			if err != nil {
				return nil, err
			}

			// Present approval gate and handle user choice.
			choice := presentApprovalGate(reqs.Content)

			// Log the approval choice for audit.
			logApprovalChoice(logger, choice, reqs.Title)

			switch choice {
			case ApprovalAccept:
				fmt.Println("\nRequirements accepted. Proceeding to planning phase.")
				return reqs, nil

			case ApprovalInterviewMore:
				fmt.Println("\nContinuing interview to gather more requirements...")
				// Add a synthetic answer to indicate we want more questions.
				rounds = append(rounds, Round{
					Questions: []Question{{
						ID:   "continue_interview",
						Text: "User requested to continue interviewing for more requirements",
					}},
					Answers: []Answer{{
						ID:    "continue_interview",
						Value: "Please ask more clarifying questions to refine the requirements",
					}},
				})
				continue

			case ApprovalChat:
				chatChoice, chatMessages := runChatLoop(reqs.Content, stackInfo, graphSummary)

				// If there were chat messages, regenerate requirements with chat content.
				if len(chatMessages) > 0 {
					fmt.Println("\nUpdating requirements with chat discussion...")
					updatedContent, err := regenerateRequirementsWithChat(reqs.Content, chatMessages, stackInfo, graphSummary)
					if err != nil {
						fmt.Printf("  (Warning: could not incorporate chat: %v)\n", err)
					} else {
						reqs.Content = updatedContent
						reqs.Title = extractTitle(updatedContent)
						if err := writeRequirements(runDir, updatedContent); err != nil {
							fmt.Printf("  (Warning: could not update requirements file: %v)\n", err)
						}

						// Show updated requirements summary.
						fmt.Println()
						fmt.Println("=== Updated Requirements Summary ===")
						fmt.Println()
						lines := strings.Split(updatedContent, "\n")
						previewLines := lines
						if len(lines) > 20 {
							previewLines = lines[:20]
							fmt.Println(strings.Join(previewLines, "\n"))
							fmt.Println("  ... (truncated, see full requirements.md)")
						} else {
							fmt.Println(strings.Join(previewLines, "\n"))
						}
					}
				}

				// Log the post-chat choice.
				logApprovalChoice(logger, chatChoice, reqs.Title)
				if chatChoice == ApprovalAccept {
					fmt.Println("\nRequirements accepted. Proceeding to planning phase.")
					return reqs, nil
				}
				// User chose to interview more after chat.
				fmt.Println("\nContinuing interview to gather more requirements...")
				rounds = append(rounds, Round{
					Questions: []Question{{
						ID:   "continue_interview",
						Text: "User discussed plan and requested to continue interviewing",
					}},
					Answers: []Answer{{
						ID:    "continue_interview",
						Value: "Please ask more clarifying questions based on our discussion",
					}},
				})
				continue
			}
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

// ApprovalChoice represents the user's decision after reviewing requirements.
type ApprovalChoice int

const (
	// ApprovalAccept means proceed to planning phase.
	ApprovalAccept ApprovalChoice = iota
	// ApprovalInterviewMore means continue the interview loop.
	ApprovalInterviewMore
	// ApprovalChat means the user wants to discuss the plan before proceeding.
	ApprovalChat
)

// ChatMessage represents a single message in the chat conversation.
type ChatMessage struct {
	Role    string // "user" or "assistant"
	Content string
}

// finalize writes the requirements markdown to disk, presents the approval
// gate, and returns the Requirements struct only if approved.
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

// presentApprovalGate displays the requirements and asks the user to approve,
// continue interviewing, or chat about the plan.
func presentApprovalGate(content string) ApprovalChoice {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("=== Requirements Summary ===")
	fmt.Println()

	// Display a preview of the requirements (first 20 lines or full content).
	lines := strings.Split(content, "\n")
	previewLines := lines
	if len(lines) > 20 {
		previewLines = lines[:20]
		fmt.Println(strings.Join(previewLines, "\n"))
		fmt.Println("  ... (truncated, see full requirements.md)")
	} else {
		fmt.Println(strings.Join(previewLines, "\n"))
	}

	fmt.Println()
	fmt.Println("=== Approval Gate ===")
	fmt.Println()
	fmt.Println("What would you like to do?")
	fmt.Println("  [1] Accept and proceed to planning")
	fmt.Println("  [2] Interview more (continue gathering requirements)")
	fmt.Println("  [3] Chat about the plan (discuss before proceeding)")
	fmt.Print("  > ")

	line, err := reader.ReadString('\n')
	if err != nil {
		// Default to accept on error.
		fmt.Println("  (Read error, defaulting to Accept)")
		return ApprovalAccept
	}

	line = strings.TrimSpace(line)

	switch line {
	case "1":
		return ApprovalAccept
	case "2":
		return ApprovalInterviewMore
	case "3":
		return ApprovalChat
	default:
		// If user enters something else, default to accept.
		fmt.Println("  (Invalid choice, defaulting to Accept)")
		return ApprovalAccept
	}
}

// runChatLoop allows the user to have a conversation about the plan before
// deciding to accept or continue interviewing. It returns both the user's
// choice and the captured chat messages for incorporation into requirements.
func runChatLoop(content string, stackInfo detect.StackInfo, graphSummary string) (ApprovalChoice, []ChatMessage) {
	reader := bufio.NewReader(os.Stdin)
	var messages []ChatMessage

	fmt.Println()
	fmt.Println("=== Chat Mode ===")
	fmt.Println("Ask questions about the requirements or plan. Type 'done' when ready to decide.")
	fmt.Println()

	for {
		fmt.Print("You: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("  (Read error, exiting chat)")
			break
		}

		line = strings.TrimSpace(line)
		if strings.ToLower(line) == "done" {
			break
		}

		if line == "" {
			continue
		}

		// Capture user message.
		messages = append(messages, ChatMessage{Role: "user", Content: line})

		// Build a prompt to answer the user's question.
		prompt := buildChatPrompt(content, line, stackInfo, graphSummary)
		response, err := spawnClaude(prompt)
		if err != nil {
			fmt.Printf("  (Error getting response: %v)\n", err)
			continue
		}

		// Capture assistant response.
		messages = append(messages, ChatMessage{Role: "assistant", Content: response})

		fmt.Println()
		fmt.Printf("Claude: %s\n", response)
		fmt.Println()
	}

	// After chat, present the approval gate again.
	fmt.Println()
	fmt.Println("Chat complete. What would you like to do?")
	fmt.Println("  [1] Accept and proceed to planning")
	fmt.Println("  [2] Interview more (continue gathering requirements)")
	fmt.Print("  > ")

	line, err := reader.ReadString('\n')
	if err != nil {
		return ApprovalAccept, messages
	}

	line = strings.TrimSpace(line)
	if line == "2" {
		return ApprovalInterviewMore, messages
	}

	return ApprovalAccept, messages
}

// regenerateRequirementsWithChat takes the original requirements and chat messages
// and spawns Claude to incorporate the chat discussion into updated requirements.
func regenerateRequirementsWithChat(originalReqs string, chatMessages []ChatMessage, stackInfo detect.StackInfo, graphSummary string) (string, error) {
	prompt := BuildRegeneratePrompt(originalReqs, chatMessages, stackInfo, graphSummary)
	output, err := spawnClaude(prompt)
	if err != nil {
		return "", fmt.Errorf("regenerating requirements: %w", err)
	}

	// The output should be pure markdown, but clean it up just in case.
	output = strings.TrimSpace(output)

	// If Claude wrapped output in markdown fences, strip them.
	if strings.HasPrefix(output, "```markdown") {
		output = strings.TrimPrefix(output, "```markdown")
		output = strings.TrimSuffix(output, "```")
		output = strings.TrimSpace(output)
	} else if strings.HasPrefix(output, "```") {
		output = strings.TrimPrefix(output, "```")
		output = strings.TrimSuffix(output, "```")
		output = strings.TrimSpace(output)
	}

	// Validate that output looks like a requirements doc (has a heading).
	if !strings.Contains(output, "#") {
		return "", fmt.Errorf("regenerated requirements missing expected markdown structure")
	}

	return output, nil
}

// buildChatPrompt creates a prompt for answering questions about the requirements.
func buildChatPrompt(requirements, question string, stackInfo detect.StackInfo, graphSummary string) string {
	var sb strings.Builder

	sb.WriteString("You are helping a developer understand a requirements document.\n\n")

	sb.WriteString("=== Requirements ===\n")
	sb.WriteString(requirements)
	sb.WriteString("\n\n")

	if stackInfo.Language != "" || stackInfo.Framework != "" {
		sb.WriteString("=== Project Context ===\n")
		if stackInfo.Language != "" {
			sb.WriteString("Language: ")
			sb.WriteString(stackInfo.Language)
			sb.WriteString("\n")
		}
		if stackInfo.Framework != "" {
			sb.WriteString("Framework: ")
			sb.WriteString(stackInfo.Framework)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if graphSummary != "" {
		sb.WriteString("=== Codebase Summary ===\n")
		sb.WriteString(graphSummary)
		sb.WriteString("\n\n")
	}

	sb.WriteString("=== User Question ===\n")
	sb.WriteString(question)
	sb.WriteString("\n\n")

	sb.WriteString("Answer concisely and helpfully. Return ONLY plain text, no JSON.")

	return sb.String()
}

// approvalChoiceString returns a human-readable string for the approval choice.
func approvalChoiceString(choice ApprovalChoice) string {
	switch choice {
	case ApprovalAccept:
		return "accept"
	case ApprovalInterviewMore:
		return "interview_more"
	case ApprovalChat:
		return "chat"
	default:
		return "unknown"
	}
}

// logApprovalChoice logs the user's requirements approval choice for audit.
func logApprovalChoice(logger *log.Logger, choice ApprovalChoice, title string) {
	if logger == nil {
		return
	}
	_ = logger.Append(log.LogEvent{
		Event:  log.EventRequirementsApproval,
		Title:  title,
		Choice: approvalChoiceString(choice),
	})
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

	// Find the start of a JSON object.
	start := strings.Index(s, "{")
	if start == -1 {
		return s
	}

	// Use json.Decoder to find the complete JSON object. This correctly
	// handles nested braces inside strings (e.g., markdown with code fences
	// in requirements_md won't cause premature truncation).
	decoder := json.NewDecoder(strings.NewReader(s[start:]))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err == nil {
		return string(raw)
	}

	// Fallback: use simple brace matching (less reliable but handles some
	// edge cases where JSON is malformed but still parseable).
	end := strings.LastIndex(s, "}")
	if end > start {
		return s[start : end+1]
	}

	return s
}
