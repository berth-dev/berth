// Package git wraps Git operations used by berth.
// This file handles worktree operations for parallel bead execution.
package git

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// worktreeDir is the directory under the project root where worktrees are stored.
const worktreeDir = ".berth/worktrees"

// MergeConflict is returned when a merge operation encounters conflicts.
type MergeConflict struct {
	BeadID string
	Output string
	Branch string
	Files  []string
}

// Error implements the error interface.
func (e *MergeConflict) Error() string {
	return fmt.Sprintf("merge conflict in bead %s on branch %s: %d conflicting files", e.BeadID, e.Branch, len(e.Files))
}

// CreateWorktreeForBead creates a git worktree for a bead at .berth/worktrees/<beadID>.
// It creates a new branch berth/worker/<beadID> based on HEAD.
// Returns the worktree path.
func CreateWorktreeForBead(projectRoot, beadID string) (string, error) {
	if err := ensureGit(); err != nil {
		return "", err
	}

	wtPath := filepath.Join(projectRoot, worktreeDir, beadID)
	branchName := fmt.Sprintf("berth/worker/%s", beadID)

	// Create worktree directory if needed.
	wtDir := filepath.Dir(wtPath)
	if err := os.MkdirAll(wtDir, 0755); err != nil {
		return "", fmt.Errorf("creating worktree directory: %w", err)
	}

	// Run: git worktree add -b {branchName} {wtPath} HEAD
	cmd := exec.Command("git", "worktree", "add", "-b", branchName, wtPath, "HEAD")
	cmd.Dir = projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return wtPath, nil
}

// RemoveWorktreeForBead removes a git worktree for a bead.
// It removes both the worktree and the associated branch.
func RemoveWorktreeForBead(projectRoot, beadID string) error {
	if err := ensureGit(); err != nil {
		return err
	}

	wtPath := filepath.Join(projectRoot, worktreeDir, beadID)
	branchName := fmt.Sprintf("berth/worker/%s", beadID)

	// Run: git worktree remove --force {wtPath}
	cmd := exec.Command("git", "worktree", "remove", "--force", wtPath)
	cmd.Dir = projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Run: git branch -D {branchName} (ignore error if branch doesn't exist)
	branchCmd := exec.Command("git", "branch", "-D", branchName)
	branchCmd.Dir = projectRoot
	_ = branchCmd.Run() // Ignore error if branch doesn't exist

	return nil
}

// MergeWorktreeForBead merges the bead's worktree branch into the target branch.
// Returns a MergeConflict error if conflicts are detected.
func MergeWorktreeForBead(projectRoot, beadID, targetBranch string) error {
	if err := ensureGit(); err != nil {
		return err
	}

	branchName := fmt.Sprintf("berth/worker/%s", beadID)

	// Run: git merge --no-ff -m "Merge bead {beadID}" {branchName}
	commitMsg := fmt.Sprintf("Merge bead %s", beadID)
	cmd := exec.Command("git", "merge", "--no-ff", "-m", commitMsg, branchName)
	cmd.Dir = projectRoot
	out, err := cmd.CombinedOutput()
	outStr := string(out)

	if err != nil {
		// Check for conflicts in output.
		if strings.Contains(outStr, "CONFLICT") || strings.Contains(outStr, "Automatic merge failed") {
			conflictFiles := parseConflictFiles(projectRoot)
			return &MergeConflict{
				BeadID: beadID,
				Output: strings.TrimSpace(outStr),
				Branch: branchName,
				Files:  conflictFiles,
			}
		}
		return fmt.Errorf("git merge: %s: %w", strings.TrimSpace(outStr), err)
	}

	return nil
}

// parseConflictFiles extracts the list of conflicting files from git status.
func parseConflictFiles(projectRoot string) []string {
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var files []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		if f := strings.TrimSpace(scanner.Text()); f != "" {
			files = append(files, f)
		}
	}
	return files
}

// ListWorktrees returns the paths of all berth worktrees in the project.
func ListWorktrees(projectRoot string) ([]string, error) {
	if err := ensureGit(); err != nil {
		return nil, err
	}

	// Run: git worktree list --porcelain
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	var worktrees []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "worktree ") {
			path := strings.TrimPrefix(line, "worktree ")
			// Filter to only berth worktrees (containing worktreeDir).
			if strings.Contains(path, worktreeDir) {
				worktrees = append(worktrees, path)
			}
		}
	}

	return worktrees, nil
}

// CleanupWorktrees removes all berth worktrees in the project.
// Logs warnings for failures but does not return an error.
func CleanupWorktrees(projectRoot string) error {
	worktrees, err := ListWorktrees(projectRoot)
	if err != nil {
		return err
	}

	for _, wt := range worktrees {
		// Extract beadID from worktree path.
		beadID := filepath.Base(wt)
		if err := RemoveWorktreeForBead(projectRoot, beadID); err != nil {
			// Log warning but continue (as per spec: "log warnings for failures").
			fmt.Fprintf(os.Stderr, "warning: failed to remove worktree %s: %v\n", wt, err)
		}
	}

	// Remove worktree directory.
	wtDir := filepath.Join(projectRoot, worktreeDir)
	if err := os.RemoveAll(wtDir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to remove worktree directory %s: %v\n", wtDir, err)
	}

	return nil
}
