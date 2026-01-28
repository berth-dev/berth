// merge.go implements the serial merge queue for parallel bead execution.
// Only the merge queue goroutine mutates the trunk branch, ensuring no
// concurrent git operations on the main integration branch.
package execute

import (
	"fmt"
	"os"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/git"
	"github.com/berth-dev/berth/internal/graph"
	"github.com/berth-dev/berth/internal/log"
)

// MergeRequest is submitted by a worker goroutine after bead execution.
type MergeRequest struct {
	Bead         *beads.Bead
	WorktreePath string
	BranchName   string
	GraphData    string
	Success      bool
	Error        error
}

// MergeResult is the outcome of processing a merge request.
type MergeResult struct {
	BeadID  string
	Success bool
	Error   error
}

// MergeQueue serializes merges into the trunk branch. Exactly one goroutine
// reads from the requests channel and performs git merge + verification.
type MergeQueue struct {
	cfg         config.Config
	projectRoot string
	trunkBranch string
	kgClient    *graph.Client
	logger      *log.Logger
	worktrees   *WorktreeManager
	systemPrompt string
	requests    chan MergeRequest
	results     chan MergeResult
	done        chan struct{}
}

// NewMergeQueue creates a MergeQueue. Call Start() in a goroutine before
// submitting requests.
func NewMergeQueue(
	cfg config.Config,
	projectRoot string,
	trunkBranch string,
	kgClient *graph.Client,
	logger *log.Logger,
	worktrees *WorktreeManager,
	systemPrompt string,
) *MergeQueue {
	return &MergeQueue{
		cfg:          cfg,
		projectRoot:  projectRoot,
		trunkBranch:  trunkBranch,
		kgClient:     kgClient,
		logger:       logger,
		worktrees:    worktrees,
		systemPrompt: systemPrompt,
		requests:     make(chan MergeRequest, 32),
		results:      make(chan MergeResult, 32),
		done:         make(chan struct{}),
	}
}

// Start begins the merge processor loop. Run in a goroutine.
func (mq *MergeQueue) Start() {
	defer close(mq.done)
	for req := range mq.requests {
		result := mq.processMerge(req)
		mq.results <- result
	}
	close(mq.results)
}

// Submit enqueues a merge request from a worker goroutine.
func (mq *MergeQueue) Submit(req MergeRequest) {
	mq.requests <- req
}

// Results returns the channel of merge results for the scheduler to read.
func (mq *MergeQueue) Results() <-chan MergeResult {
	return mq.results
}

// Close closes the requests channel, signaling no more merges will be submitted.
func (mq *MergeQueue) Close() {
	close(mq.requests)
}

// Wait blocks until the merge processor has finished processing all requests.
func (mq *MergeQueue) Wait() {
	<-mq.done
}

// processMerge handles a single merge request:
// 1. If bead failed execution, return failure
// 2. Switch to trunk, merge worker branch
// 3. On merge conflict, try reconciliation
// 4. Run verification on trunk
// 5. On verify fail, try reconciliation
// 6. On success, run onBeadSuccess, clean up worktree
func (mq *MergeQueue) processMerge(req MergeRequest) MergeResult {
	beadID := req.Bead.ID

	// If the bead failed execution entirely, no merge needed.
	if !req.Success {
		return MergeResult{
			BeadID:  beadID,
			Success: false,
			Error:   fmt.Errorf("bead %s failed execution: %w", beadID, req.Error),
		}
	}

	// Log merge start.
	if mq.logger != nil {
		_ = mq.logger.Append(log.LogEvent{
			Event:     log.EventMergeStarted,
			BeadID:    beadID,
			MergeFrom: req.BranchName,
			MergeTo:   mq.trunkBranch,
		})
	}

	// Switch to trunk branch.
	if err := git.SwitchBranch(mq.trunkBranch); err != nil {
		return MergeResult{
			BeadID:  beadID,
			Success: false,
			Error:   fmt.Errorf("switching to trunk branch: %w", err),
		}
	}

	// Merge worker branch into trunk.
	commitMsg := fmt.Sprintf("merge(berth): integrate bead %s - %s", beadID, req.Bead.Title)
	if mergeErr := git.MergeWorktreeBranch(req.BranchName, commitMsg); mergeErr != nil {
		// Merge conflict — abort. Reconciliation cannot resolve git conflicts
		// (trunk is clean after abort), so skip it and fail directly.
		_ = git.AbortMerge()

		if mq.logger != nil {
			_ = mq.logger.Append(log.LogEvent{
				Event:  log.EventMergeFailed,
				BeadID: beadID,
				Error:  mergeErr.Error(),
			})
		}

		return MergeResult{
			BeadID:  beadID,
			Success: false,
			Error:   fmt.Errorf("merge conflict for bead %s: %w", beadID, mergeErr),
		}
	}

	// Run verification on trunk.
	verifyResult, err := RunVerification(mq.cfg, req.Bead, mq.projectRoot)
	if err != nil {
		return MergeResult{
			BeadID:  beadID,
			Success: false,
			Error:   fmt.Errorf("post-merge verification error for bead %s: %w", beadID, err),
		}
	}

	if !verifyResult.Passed {
		// Verification failed — try reconciliation.
		if mq.logger != nil {
			_ = mq.logger.Append(log.LogEvent{
				Event:  log.EventMergeFailed,
				BeadID: beadID,
				Step:   verifyResult.FailedStep,
				Error:  verifyResult.Output,
			})
		}

		reconciled, reconcileErr := Reconcile(
			mq.cfg, req.Bead, req.WorktreePath, mq.projectRoot,
			mq.kgClient, mq.logger,
		)
		if reconcileErr != nil || !reconciled {
			mergeResult := MergeResult{
				BeadID:  beadID,
				Success: false,
			}
			if reconcileErr != nil {
				mergeResult.Error = fmt.Errorf("post-merge verify failed for bead %s, reconciliation failed: %w", beadID, reconcileErr)
			} else {
				mergeResult.Error = fmt.Errorf("post-merge verify failed for bead %s at %q, reconciliation did not fix", beadID, verifyResult.FailedStep)
			}
			return mergeResult
		}

		// Re-verify after reconciliation.
		reVerify, reErr := RunVerification(mq.cfg, req.Bead, mq.projectRoot)
		if reErr != nil || !reVerify.Passed {
			mergeResult := MergeResult{
				BeadID:  beadID,
				Success: false,
			}
			if reErr != nil {
				mergeResult.Error = fmt.Errorf("post-reconcile verify error for bead %s: %w", beadID, reErr)
			} else {
				mergeResult.Error = fmt.Errorf("post-reconcile verify still failing for bead %s at %q", beadID, reVerify.FailedStep)
			}
			return mergeResult
		}
	}

	// Success: run post-success steps.
	if err := onBeadSuccess(req.Bead, mq.kgClient, mq.projectRoot, mq.logger, mq.systemPrompt); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: post-merge success steps failed for bead %s: %v\n", beadID, err)
	}

	// Clean up worktree.
	if err := mq.worktrees.Remove(beadID); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to remove worktree for bead %s: %v\n", beadID, err)
	}

	// Log merge success.
	if mq.logger != nil {
		_ = mq.logger.Append(log.LogEvent{
			Event:     log.EventMergeCompleted,
			BeadID:    beadID,
			MergeFrom: req.BranchName,
			MergeTo:   mq.trunkBranch,
		})
	}

	return MergeResult{
		BeadID:  beadID,
		Success: true,
	}
}
