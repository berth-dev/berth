// worktree.go manages per-bead git worktrees for parallel execution.
package execute

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/berth-dev/berth/internal/git"
)

// WorktreeManager creates and removes git worktrees for parallel bead execution.
// Each bead gets its own worktree under .berth/worktrees/<beadID>/ with a
// dedicated branch berth/worker/<beadID>.
type WorktreeManager struct {
	projectRoot string
	baseBranch  string
	mu          sync.Mutex
	worktrees   map[string]string // beadID -> worktree path
}

// NewWorktreeManager creates a WorktreeManager rooted at the project directory.
func NewWorktreeManager(projectRoot, baseBranch string) *WorktreeManager {
	return &WorktreeManager{
		projectRoot: projectRoot,
		baseBranch:  baseBranch,
		worktrees:   make(map[string]string),
	}
}

// Create creates a git worktree for the given bead. Returns the absolute
// worktree path. The worktree is created at .berth/worktrees/<beadID>/
// with a new branch berth/worker/<beadID> based on baseBranch.
func (wm *WorktreeManager) Create(beadID string) (string, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if existing, ok := wm.worktrees[beadID]; ok {
		return existing, nil
	}

	path := filepath.Join(wm.projectRoot, ".berth", "worktrees", beadID)
	branch := wm.BranchName(beadID)

	if err := git.CreateWorktree(path, branch, wm.baseBranch); err != nil {
		return "", fmt.Errorf("creating worktree for bead %s: %w", beadID, err)
	}

	wm.worktrees[beadID] = path
	return path, nil
}

// Remove removes the worktree and deletes the branch for the given bead.
func (wm *WorktreeManager) Remove(beadID string) error {
	wm.mu.Lock()
	path, ok := wm.worktrees[beadID]
	if ok {
		delete(wm.worktrees, beadID)
	}
	wm.mu.Unlock()

	if !ok {
		return nil
	}

	if err := git.RemoveWorktree(path); err != nil {
		return fmt.Errorf("removing worktree for bead %s: %w", beadID, err)
	}

	branch := wm.BranchName(beadID)
	if err := git.DeleteBranch(branch); err != nil {
		// Best effort: branch may already be deleted.
		fmt.Printf("Warning: failed to delete branch %s: %v\n", branch, err)
	}

	return nil
}

// Path returns the worktree path for a bead, and whether it exists.
func (wm *WorktreeManager) Path(beadID string) (string, bool) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	path, ok := wm.worktrees[beadID]
	return path, ok
}

// BranchName returns the worker branch name for a bead.
func (wm *WorktreeManager) BranchName(beadID string) string {
	return "berth/worker/" + beadID
}

// CleanupAll removes all worktrees and deletes their branches.
func (wm *WorktreeManager) CleanupAll() {
	wm.mu.Lock()
	ids := make([]string, 0, len(wm.worktrees))
	for id := range wm.worktrees {
		ids = append(ids, id)
	}
	wm.mu.Unlock()

	for _, id := range ids {
		if err := wm.Remove(id); err != nil {
			fmt.Printf("Warning: failed to cleanup worktree for bead %s: %v\n", id, err)
		}
	}
}
