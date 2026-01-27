// commit.go stages and commits changes per bead.
package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// CommitBead stages all changed files and creates a commit tied to a bead.
// Commit message format: "feat(berth): <message>\n\n[berth:<beadID>]"
// Returns nil if there are no changes to commit.
func CommitBead(beadID, message string) error {
	if err := ensureGit(); err != nil {
		return err
	}

	has, err := HasChanges()
	if err != nil {
		return err
	}
	if !has {
		return nil
	}

	addCmd := exec.Command("git", "add", "-A")
	if out, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add -A: %s: %w", strings.TrimSpace(string(out)), err)
	}

	commitMsg := fmt.Sprintf("feat(berth): %s\n\n[berth:%s]", message, beadID)
	commitCmd := exec.Command("git", "commit", "-m", commitMsg)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

// CommitFiles stages specific files and creates a commit.
func CommitFiles(files []string, message string) error {
	if err := ensureGit(); err != nil {
		return err
	}

	args := append([]string{"add"}, files...)
	addCmd := exec.Command("git", args...)
	if out, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %s: %w", strings.TrimSpace(string(out)), err)
	}

	commitCmd := exec.Command("git", "commit", "-m", message)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return nil
}

// HasChanges returns true if the working tree has uncommitted changes.
// Shells out to: git status --porcelain
func HasChanges() (bool, error) {
	if err := ensureGit(); err != nil {
		return false, err
	}
	cmd := exec.Command("git", "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status --porcelain: %w", err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}
