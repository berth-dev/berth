// display.go handles terminal UI: printing questions and reading user input.
package understand

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// DisplayQuestions renders each question in the terminal and collects answers
// from stdin. Each question shows numbered options, a "(Recommended)" suffix
// where applicable, and an optional "Help me decide" entry. When allow_custom
// is true, the user may type free-form text instead of a number.
//
// Returns one Answer per question in the same order as the input slice.
func DisplayQuestions(questions []Question) []Answer {
	reader := bufio.NewReader(os.Stdin)
	answers := make([]Answer, 0, len(questions))

	for _, q := range questions {
		answer := displayOneQuestion(q, reader)
		answers = append(answers, answer)
	}

	return answers
}

// displayOneQuestion renders a single question and reads one answer.
func displayOneQuestion(q Question, reader *bufio.Reader) Answer {
	fmt.Println()
	fmt.Println(q.Text)

	// Display numbered options.
	for i, opt := range q.Options {
		suffix := ""
		if opt.Recommended {
			suffix = " (Recommended)"
		}
		fmt.Printf("  [%d] %s%s\n", i+1, opt.Label, suffix)
	}

	// Add "Help me decide" as the last option when allowed.
	helpIdx := 0
	if q.AllowHelp {
		helpIdx = len(q.Options) + 1
		fmt.Printf("  [%d] Help me decide\n", helpIdx)
	}

	fmt.Print("  > ")

	line, err := reader.ReadString('\n')
	if err != nil {
		// On EOF or read error, return empty answer.
		return Answer{ID: q.ID, Value: ""}
	}

	line = strings.TrimSpace(line)

	// Check if input is a number selecting an option.
	if num, ok := parseOptionNumber(line); ok {
		// "Help me decide" selection.
		if q.AllowHelp && num == helpIdx {
			return Answer{ID: q.ID, Value: helpMeDecideValue}
		}

		// Valid option number.
		if num >= 1 && num <= len(q.Options) {
			return Answer{ID: q.ID, Value: q.Options[num-1].Label}
		}
	}

	// If allow_custom is true, accept raw text as a custom answer.
	if q.AllowCustom && line != "" {
		return Answer{ID: q.ID, Value: line}
	}

	// Fallback: return the raw input. The loop will treat it as custom text
	// even if allow_custom is false; upstream code can validate further.
	return Answer{ID: q.ID, Value: line}
}

// parseOptionNumber attempts to parse s as a positive integer. Returns the
// number and true on success, or 0 and false otherwise.
func parseOptionNumber(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, false
		}
		n = n*10 + int(ch-'0')
	}
	if n == 0 {
		return 0, false
	}
	return n, true
}

// helpMeDecideValue is the sentinel value returned when the user selects the
// "Help me decide" option. The loop checks for this to trigger an explain call.
const helpMeDecideValue = "__help_me_decide__"
