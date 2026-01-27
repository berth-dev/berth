// stuck.go handles the pause-with-choices flow (hint/rescue/skip/abort) for stuck beads.
package execute

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/config"
	berthcontext "github.com/berth-dev/berth/internal/context"
	"github.com/berth-dev/berth/prompts"
)

// StuckAction represents the user's chosen resolution for a stuck bead.
type StuckAction struct {
	Action string // "hint", "rescue", "skip", "abort"
	Hint   string // Only populated for "hint" action
}

// HandleStuck pauses execution and presents the user with choices for
// resolving a stuck bead. The menu loops until the user picks skip/abort
// or until a hint/rescue attempt succeeds verification.
func HandleStuck(
	cfg config.Config,
	bead *beads.Bead,
	verifyErrors []string,
	diagnostic string,
	graphData string,
	projectRoot string,
) (StuckAction, error) {
	reader := bufio.NewReader(os.Stdin)

	for {
		printStuckMenu(bead, diagnostic)

		choice, err := readChoice(reader)
		if err != nil {
			return StuckAction{}, fmt.Errorf("reading user choice: %w", err)
		}

		switch choice {
		case "1":
			// Hint: read a one-liner from the user and retry.
			hint, err := readHint(reader)
			if err != nil {
				return StuckAction{}, fmt.Errorf("reading hint: %w", err)
			}

			success, err := retryWithHint(cfg, bead, hint, graphData, projectRoot)
			if err != nil {
				fmt.Printf("  Hint retry error: %v\n", err)
				continue
			}
			if success {
				return StuckAction{Action: "hint", Hint: hint}, nil
			}
			fmt.Println("  Hint retry failed verification. Returning to menu.")
			continue

		case "2":
			// Rescue: open interactive Claude session.
			err := RunRescue(cfg, bead, verifyErrors, diagnostic, graphData, projectRoot)
			if err != nil {
				fmt.Printf("  Rescue session error: %v\n", err)
				continue
			}

			// Check if verification passes after rescue.
			result, err := RunVerification(cfg, bead)
			if err != nil {
				fmt.Printf("  Post-rescue verification error: %v\n", err)
				continue
			}
			if result.Passed {
				return StuckAction{Action: "rescue"}, nil
			}
			fmt.Println("  Rescue session completed but verification still fails.")
			fmt.Printf("  Failed step: %s\n", result.FailedStep)
			continue

		case "3":
			// Skip: mark bead as stuck and move on.
			if err := beads.UpdateStatus(bead.ID, "stuck"); err != nil {
				return StuckAction{}, fmt.Errorf("marking bead %s as stuck: %w", bead.ID, err)
			}
			return StuckAction{Action: "skip"}, nil

		case "4":
			// Abort: stop the entire run.
			return StuckAction{Action: "abort"}, nil

		default:
			fmt.Println("  Invalid choice. Please enter 1, 2, 3, or 4.")
			continue
		}
	}
}

// printStuckMenu displays the stuck bead information and available actions.
func printStuckMenu(bead *beads.Bead, diagnostic string) {
	fmt.Println()
	fmt.Printf("Bead %s stuck: %q\n", bead.ID, bead.Title)
	fmt.Println("  Failed 4 times. Diagnosis:")

	if diagnostic != "" {
		// Indent diagnostic output for readability.
		lines := strings.Split(diagnostic, "\n")
		for _, line := range lines {
			fmt.Printf("  %s\n", line)
		}
	} else {
		fmt.Println("  (no diagnostic available)")
	}

	fmt.Println()
	fmt.Println("  What do you want to do?")
	fmt.Println()
	fmt.Println("  [1] Hint    -- Give the executor a one-liner and retry")
	fmt.Println("  [2] Rescue  -- Open interactive Claude session with full error context")
	fmt.Println("  [3] Skip    -- Continue with unblocked beads, leave", bead.ID, "stuck")
	fmt.Println("  [4] Abort   -- Stop the entire run")
	fmt.Println()
}

// readChoice reads a single-character choice from the user.
func readChoice(reader *bufio.Reader) (string, error) {
	fmt.Print("  Choice: ")

	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(input), nil
}

// readHint prompts the user for a one-liner hint.
func readHint(reader *bufio.Reader) (string, error) {
	fmt.Print("  Enter hint: ")

	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(input), nil
}

// retryWithHint spawns a Claude session with the user's hint appended
// to the executor prompt, then runs verification. Returns true if
// verification passes.
func retryWithHint(
	cfg config.Config,
	bead *beads.Bead,
	hint string,
	graphData string,
	projectRoot string,
) (bool, error) {
	learnings := berthcontext.ReadLearnings(projectRoot)
	systemPrompt := prompts.ExecutorSystemPrompt

	// Build the prompt with the hint as a diagnosis-like addition.
	hintDiagnosis := fmt.Sprintf("User hint: %s", hint)
	taskPrompt := BuildExecutorPrompt(bead, 5, &hintDiagnosis, graphData, learnings)

	output, err := SpawnClaude(cfg, systemPrompt, taskPrompt, projectRoot)
	if err != nil {
		return false, fmt.Errorf("spawning claude with hint: %w", err)
	}

	if output.IsError {
		return false, fmt.Errorf("claude returned error: %s", output.Result)
	}

	result, err := RunVerification(cfg, bead)
	if err != nil {
		return false, fmt.Errorf("verification after hint: %w", err)
	}

	return result.Passed, nil
}
