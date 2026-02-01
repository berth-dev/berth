// pr.go implements the "berth pr" command for creating pull requests via gh CLI.
package cli

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/git"
	"github.com/spf13/cobra"
)

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Create PR from current run branch",
	Long: `Create a GitHub pull request from the current run's branch.
Auto-generates PR title and body from the run's bead summaries.`,
	RunE: runPR,
}

func init() {
	prCmd.Flags().String("base", "main", "Base branch for the PR")
	prCmd.Flags().Bool("draft", false, "Create as draft PR")
	prCmd.Flags().String("title", "", "Override PR title")
}

func runPR(cmd *cobra.Command, args []string) error {
	// Get current branch and verify we're not on main/master.
	branch, err := git.CurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}
	if branch == "main" || branch == "master" {
		return fmt.Errorf("cannot create PR from main branch")
	}

	// Check if PR already exists.
	exists, err := git.PRExists()
	if err != nil {
		if errors.Is(err, git.ErrGHNotFound) {
			return fmt.Errorf("GitHub CLI (gh) not found. Install it from: https://cli.github.com/")
		}
		return fmt.Errorf("failed to check existing PR: %w", err)
	}
	if exists {
		fmt.Println("A pull request already exists for this branch.")
		return nil
	}

	// Push branch to remote.
	pushCmd := exec.Command("git", "push", "-u", "origin", branch)
	if out, pushErr := pushCmd.CombinedOutput(); pushErr != nil {
		output := strings.TrimSpace(string(out))
		if strings.Contains(output, "No commits") || strings.Contains(output, "no commits") {
			return fmt.Errorf("no commits on this branch")
		}
		return fmt.Errorf("failed to push branch: %s: %w", output, pushErr)
	}

	// Determine PR title.
	title, _ := cmd.Flags().GetString("title")
	if title == "" {
		title = "feat: berth run"
	}

	// Generate PR body from beads.
	body := generatePRBody()

	// Get flags.
	base, _ := cmd.Flags().GetString("base")
	draft, _ := cmd.Flags().GetBool("draft")

	// Create the PR.
	if draft {
		// Shell out to gh directly for draft support.
		prURL, err := createDraftPR(title, body, base)
		if err != nil {
			return handlePRError(err)
		}
		fmt.Println(prURL)
	} else {
		prURL, err := git.CreatePR(title, body, base)
		if err != nil {
			return handlePRError(err)
		}
		fmt.Println(prURL)
	}

	return nil
}

// generatePRBody creates a PR description from the current beads list.
func generatePRBody() string {
	var b strings.Builder
	b.WriteString("## Berth Run Summary\n\n")

	allBeads, err := beads.List()
	if err != nil || len(allBeads) == 0 {
		b.WriteString("_No beads found._\n")
		return b.String()
	}

	b.WriteString("### Beads\n\n")
	for _, bead := range allBeads {
		icon := statusIcon(bead.Status)
		fmt.Fprintf(&b, "- %s **%s**: %s\n", icon, bead.ID, bead.Title)
	}
	b.WriteString("\n---\n_Created by [berth](https://github.com/berth-dev/berth)_\n")

	return b.String()
}

// statusIcon returns a text indicator for a bead status.
func statusIcon(status string) string {
	switch status {
	case "done":
		return "[x]"
	case "in_progress":
		return "[~]"
	case "stuck":
		return "[!]"
	case "open":
		return "[ ]"
	default:
		return "[-]"
	}
}

// createDraftPR creates a draft PR by shelling out to gh directly.
func createDraftPR(title, body, base string) (string, error) {
	cmd := exec.Command("gh", "pr", "create",
		"--title", title,
		"--body", body,
		"--base", base,
		"--draft",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh pr create --draft: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// handlePRError provides user-friendly messages for common PR creation failures.
func handlePRError(err error) error {
	msg := err.Error()
	if errors.Is(err, git.ErrGHNotFound) {
		return fmt.Errorf("GitHub CLI (gh) not found. Install it from: https://cli.github.com/")
	}
	if strings.Contains(msg, "not logged") || strings.Contains(msg, "auth") {
		return fmt.Errorf("not authenticated with GitHub; run: gh auth login")
	}
	return fmt.Errorf("failed to create PR: %w", err)
}
