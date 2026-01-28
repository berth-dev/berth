// Package git wraps Git operations used by berth.
// This file handles creating and switching branches.
package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var (
	ErrGitNotFound = errors.New("git not found in PATH")
	ErrGHNotFound  = errors.New("gh CLI not found in PATH; install GitHub CLI first")
	ErrNoChanges   = errors.New("no changes to commit")
	ErrNotARepo    = errors.New("not a git repository")
)

// ensureGit checks that git is available in PATH.
func ensureGit() error {
	_, err := exec.LookPath("git")
	if err != nil {
		return ErrGitNotFound
	}
	return nil
}

// CreateBranch creates a new git branch and switches to it.
// Shells out to: git checkout -b <name>
func CreateBranch(name string) error {
	if err := ensureGit(); err != nil {
		return err
	}
	cmd := exec.Command("git", "checkout", "-b", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout -b %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// SwitchBranch switches to an existing branch.
// Shells out to: git checkout <name>
func SwitchBranch(name string) error {
	if err := ensureGit(); err != nil {
		return err
	}
	cmd := exec.Command("git", "checkout", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// CurrentBranch returns the name of the current git branch.
// Shells out to: git rev-parse --abbrev-ref HEAD
func CurrentBranch() (string, error) {
	if err := ensureGit(); err != nil {
		return "", err
	}
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --abbrev-ref HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// EnsureRepo ensures a git repository exists in the current directory.
// Runs `git init` if .git/ is not present. This is a no-op if already a repo.
func EnsureRepo() error {
	if err := ensureGit(); err != nil {
		return err
	}
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		initCmd := exec.Command("git", "init")
		if out, initErr := initCmd.CombinedOutput(); initErr != nil {
			return fmt.Errorf("git init: %s: %w", strings.TrimSpace(string(out)), initErr)
		}
	}
	return nil
}

// EnsureInitialCommit creates an empty initial commit if the repo has none.
// This is needed because git cannot create branches in a repo with no commits.
func EnsureInitialCommit() error {
	if err := ensureGit(); err != nil {
		return err
	}
	// Check if HEAD exists (i.e., there is at least one commit).
	cmd := exec.Command("git", "rev-parse", "HEAD")
	if err := cmd.Run(); err != nil {
		// No commits — create an empty initial commit.
		commitCmd := exec.Command("git", "commit", "--allow-empty", "-m", "chore: initialize repository")
		if out, commitErr := commitCmd.CombinedOutput(); commitErr != nil {
			return fmt.Errorf("creating initial commit: %s: %w", strings.TrimSpace(string(out)), commitErr)
		}
	}
	return nil
}

// BranchExists checks whether a branch with the given name exists.
// Shells out to: git rev-parse --verify <name>
func BranchExists(name string) bool {
	if err := ensureGit(); err != nil {
		return false
	}
	cmd := exec.Command("git", "rev-parse", "--verify", name)
	return cmd.Run() == nil
}

// CreateWorktree creates a git worktree at path with a new branch based on baseBranch.
// Shells out to: git worktree add -b <branchName> <path> <baseBranch>
func CreateWorktree(path, branchName, baseBranch string) error {
	if err := ensureGit(); err != nil {
		return err
	}
	cmd := exec.Command("git", "worktree", "add", "-b", branchName, path, baseBranch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// RemoveWorktree removes a git worktree at the given path.
// Shells out to: git worktree remove --force <path>
func RemoveWorktree(path string) error {
	if err := ensureGit(); err != nil {
		return err
	}
	cmd := exec.Command("git", "worktree", "remove", "--force", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// MergeWorktreeBranch merges the named branch into the current branch with a merge commit.
// Shells out to: git merge --no-ff <branchName> -m <commitMsg>
func MergeWorktreeBranch(branchName, commitMsg string) error {
	if err := ensureGit(); err != nil {
		return err
	}
	cmd := exec.Command("git", "merge", "--no-ff", branchName, "-m", commitMsg)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git merge: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// AbortMerge aborts an in-progress merge.
// Shells out to: git merge --abort
func AbortMerge() error {
	if err := ensureGit(); err != nil {
		return err
	}
	cmd := exec.Command("git", "merge", "--abort")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git merge --abort: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// DeleteBranch deletes a local branch.
// Shells out to: git branch -D <name>
func DeleteBranch(name string) error {
	if err := ensureGit(); err != nil {
		return err
	}
	cmd := exec.Command("git", "branch", "-D", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git branch -D %s: %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}
