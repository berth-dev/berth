// prompt.go builds the understand prompt for each interview round.
package understand

import (
	"fmt"
	"strings"

	"github.com/berth-dev/berth/internal/detect"
)

// BuildUnderstandPrompt constructs the prompt sent to Claude for a given
// interview round. The first round includes the task description, stack info,
// and Knowledge Graph summary. Subsequent rounds accumulate all previous Q&A
// history so Claude has full context when generating follow-up questions.
func BuildUnderstandPrompt(round int, previousRounds []Round, stackInfo detect.StackInfo, graphSummary string, description string) string {
	var sb strings.Builder

	// System-level instructions.
	sb.WriteString(systemInstructions)
	sb.WriteString("\n\n")

	// Project context.
	sb.WriteString("## Project Stack\n")
	sb.WriteString(formatStackInfo(stackInfo))
	sb.WriteString("\n\n")

	// Knowledge Graph summary (pre-embedded codebase context).
	if graphSummary != "" {
		sb.WriteString("## Codebase Context (Knowledge Graph)\n")
		sb.WriteString(graphSummary)
		sb.WriteString("\n\n")
	}

	// Task description.
	sb.WriteString("## Task Description\n")
	sb.WriteString(description)
	sb.WriteString("\n\n")

	// Previous rounds (accumulated Q&A history).
	if len(previousRounds) > 0 {
		sb.WriteString("## Previous Interview Rounds\n")
		for i, r := range previousRounds {
			sb.WriteString(fmt.Sprintf("### Round %d\n", i+1))
			for _, q := range r.Questions {
				sb.WriteString(fmt.Sprintf("**Q (%s):** %s\n", q.ID, q.Text))
				// Find the corresponding answer.
				for _, a := range r.Answers {
					if a.ID == q.ID {
						sb.WriteString(fmt.Sprintf("**A:** %s\n", a.Value))
						break
					}
				}
			}
			sb.WriteString("\n")
		}
	}

	// Current round instruction.
	sb.WriteString(fmt.Sprintf("## Current Round: %d\n", round))
	if round == 1 {
		sb.WriteString("This is the first interview round. Ask the most important clarifying questions about the task.\n")
	} else {
		sb.WriteString("Based on the answers so far, either ask follow-up questions or signal that you have enough information.\n")
	}
	sb.WriteString("\n")

	// Output format instructions.
	sb.WriteString(outputInstructions)

	return sb.String()
}

// formatStackInfo produces a human-readable summary of the detected stack.
func formatStackInfo(s detect.StackInfo) string {
	if s.Language == "" && s.Framework == "" {
		return "No stack detected (greenfield project)"
	}

	var parts []string
	if s.Language != "" {
		parts = append(parts, fmt.Sprintf("Language: %s", s.Language))
	}
	if s.Framework != "" {
		parts = append(parts, fmt.Sprintf("Framework: %s", s.Framework))
	}
	if s.PackageManager != "" {
		parts = append(parts, fmt.Sprintf("Package manager: %s", s.PackageManager))
	}
	if s.TestCmd != "" {
		parts = append(parts, fmt.Sprintf("Test command: %s", s.TestCmd))
	}
	if s.BuildCmd != "" {
		parts = append(parts, fmt.Sprintf("Build command: %s", s.BuildCmd))
	}
	if s.LintCmd != "" {
		parts = append(parts, fmt.Sprintf("Lint command: %s", s.LintCmd))
	}
	return strings.Join(parts, "\n")
}

// systemInstructions tells Claude how to behave during the understand phase.
const systemInstructions = `You are Berth's requirements interviewer. Your goal is to gather enough information to write a clear, complete requirements document for a coding task.

Rules:
1. Ask focused, specific questions — avoid vague or open-ended queries
2. Provide numbered options for each question when possible, marking one as recommended
3. Limit each round to 1-4 questions to avoid overwhelming the user
4. Use codebase context (Knowledge Graph data) to make questions grounded in reality
5. When you have enough information to write requirements, signal done=true
6. If the task is simple and the description is clear, it is OK to signal done after 1-2 rounds
7. Never ask about things already answered in previous rounds
8. Frame questions around decisions, not information gathering — the user expects actionable choices
9. When recommending an option, base it on the codebase context and common best practices`

// outputInstructions describes the expected JSON output format.
const outputInstructions = `## Output Format

You MUST respond with valid JSON and nothing else. No markdown fences, no explanation outside the JSON.

If you need more information, respond with:
{
  "done": false,
  "context": "Brief summary of what you understand so far about the task and codebase",
  "questions": [
    {
      "id": "q1",
      "text": "Your question text",
      "options": [
        {"key": "1", "label": "Option description", "recommended": true},
        {"key": "2", "label": "Another option"}
      ],
      "allow_custom": true,
      "allow_help": true
    }
  ]
}

If you have enough information, respond with:
{
  "done": true,
  "requirements_md": "# Requirements: <title>\n\n## Decisions\n- Decision 1...\n\n## Scope\n- ...\n\n## Out of Scope\n- ...\n\n## Technical Approach\n- ...\n\n## Acceptance Criteria\n- ..."
}

IMPORTANT: The "requirements_md" field must be a complete markdown document suitable for driving a planning phase. Include all decisions made during the interview.`

// BuildRegeneratePrompt creates a prompt to incorporate chat discussion into requirements.
func BuildRegeneratePrompt(originalReqs string, chatMessages []ChatMessage, stackInfo detect.StackInfo, graphSummary string) string {
	var sb strings.Builder

	sb.WriteString("You are updating a requirements document based on a follow-up chat discussion.\n\n")

	sb.WriteString("## Original Requirements\n")
	sb.WriteString(originalReqs)
	sb.WriteString("\n\n")

	sb.WriteString("## Chat Discussion\n")
	for _, msg := range chatMessages {
		if msg.Role == "user" {
			sb.WriteString("User: ")
		} else {
			sb.WriteString("Assistant: ")
		}
		sb.WriteString(msg.Content)
		sb.WriteString("\n\n")
	}

	if stackInfo.Language != "" || stackInfo.Framework != "" {
		sb.WriteString("## Project Context\n")
		sb.WriteString(formatStackInfo(stackInfo))
		sb.WriteString("\n\n")
	}

	if graphSummary != "" {
		sb.WriteString("## Codebase Context\n")
		sb.WriteString(graphSummary)
		sb.WriteString("\n\n")
	}

	sb.WriteString(`## Task
Incorporate any new features, changes, or clarifications from the chat into the requirements document.

Rules:
1. Preserve the original structure (Overview/Decisions, Scope, Out of Scope, Technical Approach, Acceptance Criteria)
2. Add new items to the appropriate sections based on the chat discussion
3. Do NOT remove anything from the original unless the chat explicitly requested removal
4. If the chat discussed implementation details, add them to Technical Approach
5. If new features were discussed, add them to Scope and Acceptance Criteria
6. Keep the same formatting style as the original document

Output ONLY the updated requirements markdown, no explanation or commentary.`)

	return sb.String()
}
