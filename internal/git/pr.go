// pr.go creates pull requests via the gh CLI.
package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// ensureGH checks that the GitHub CLI (gh) is available in PATH.
func ensureGH() error {
	_, err := exec.LookPath("gh")
	if err != nil {
		return ErrGHNotFound
	}
	return nil
}

// CreatePR creates a GitHub pull request using the gh CLI.
// Returns the PR URL on success.
func CreatePR(title, body, base string) (string, error) {
	if err := ensureGH(); err != nil {
		return "", err
	}

	cmd := exec.Command("gh", "pr", "create",
		"--title", title,
		"--body", body,
		"--base", base,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh pr create: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return strings.TrimSpace(string(out)), nil
}

// PRExists checks if there is already an open PR for the current branch.
// Returns true if a PR exists (regardless of state).
func PRExists() (bool, error) {
	if err := ensureGH(); err != nil {
		return false, err
	}

	cmd := exec.Command("gh", "pr", "view", "--json", "state")
	if err := cmd.Run(); err != nil {
		// gh returns an error when no PR exists for the current branch.
		return false, nil
	}
	return true, nil
}
