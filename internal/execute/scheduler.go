// scheduler.go implements the dependency-aware parallel bead scheduler.
package execute

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/coordinator"
	"github.com/berth-dev/berth/internal/graph"
	"github.com/berth-dev/berth/internal/log"
	"github.com/berth-dev/berth/prompts"
)

// BeadNode represents a bead in the dependency graph.
type BeadNode struct {
	Bead       *beads.Bead
	DependsOn  []string // bead IDs this node depends on
	DependedBy []string // bead IDs that depend on this node
	Status     string   // "pending"|"running"|"completed"|"failed"|"skipped"
}

// Scheduler manages concurrent bead execution up to MaxParallel,
// launching beads as their dependencies are satisfied.
type Scheduler struct {
	cfg          config.Config
	projectRoot  string
	nodes        map[string]*BeadNode
	orderedIDs   []string // deterministic iteration order (sorted bead IDs)
	mu           sync.Mutex
	maxParallel  int
	running      int
	pool         *ExecutionPool
	worktrees    *WorktreeManager
	mergeQueue   *MergeQueue
	coordServer  *coordinator.Server
	kgClient     *graph.Client
	logger       *log.Logger
	systemPrompt string
	verbose      bool
	wg           sync.WaitGroup
}

// NewScheduler builds a dependency graph from the bead list and returns a
// ready-to-run Scheduler. The dep graph is built from each bead's DependsOn field.
func NewScheduler(
	cfg config.Config,
	projectRoot string,
	allBeads []beads.Bead,
	pool *ExecutionPool,
	worktrees *WorktreeManager,
	mergeQueue *MergeQueue,
	coordServer *coordinator.Server,
	kgClient *graph.Client,
	logger *log.Logger,
	systemPrompt string,
	verbose bool,
) *Scheduler {
	nodes := make(map[string]*BeadNode, len(allBeads))

	// Create nodes.
	for i := range allBeads {
		b := &allBeads[i]
		nodes[b.ID] = &BeadNode{
			Bead:      b,
			DependsOn: b.DependsOn,
			Status:    "pending",
		}
	}

	// Build reverse dependency map.
	for id, node := range nodes {
		for _, depID := range node.DependsOn {
			if depNode, ok := nodes[depID]; ok {
				depNode.DependedBy = append(depNode.DependedBy, id)
			}
		}
	}

	// Build sorted ID list for deterministic launch order.
	orderedIDs := make([]string, 0, len(nodes))
	for id := range nodes {
		orderedIDs = append(orderedIDs, id)
	}
	sort.Strings(orderedIDs)

	maxParallel := cfg.Execution.MaxParallel
	if maxParallel <= 0 {
		maxParallel = 5
	}

	return &Scheduler{
		cfg:          cfg,
		projectRoot:  projectRoot,
		nodes:        nodes,
		orderedIDs:   orderedIDs,
		maxParallel:  maxParallel,
		pool:         pool,
		worktrees:    worktrees,
		mergeQueue:   mergeQueue,
		coordServer:  coordServer,
		kgClient:     kgClient,
		logger:       logger,
		systemPrompt: systemPrompt,
		verbose:      verbose,
	}
}

// Run executes the scheduling loop: launch ready beads, process merge results,
// repeat until all beads are done.
func (s *Scheduler) Run() error {
	s.launchReady()

	for result := range s.mergeQueue.Results() {
		s.mu.Lock()
		node, ok := s.nodes[result.BeadID]
		if ok {
			if result.Success {
				node.Status = "completed"
				s.pool.RecordCompletion()
			} else {
				node.Status = "failed"
				s.pool.RecordStuck()
				s.cascadeFailure(node)
			}
			s.running--
		}
		s.mu.Unlock()

		if result.Error != nil {
			fmt.Fprintf(os.Stderr, "Bead %s merge result: %v\n", result.BeadID, result.Error)
		}

		s.launchReady()

		if s.pool.IsComplete() {
			break
		}
	}

	s.wg.Wait()
	return nil
}

// launchReady finds all unblocked pending beads and launches goroutines
// for them, up to maxParallel concurrent workers. Iterates in sorted ID
// order for deterministic, reproducible scheduling.
func (s *Scheduler) launchReady() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range s.orderedIDs {
		if s.running >= s.maxParallel {
			break
		}
		node := s.nodes[id]
		if node.Status != "pending" {
			continue
		}
		if !s.depsComplete(node) {
			continue
		}

		node.Status = "running"
		s.running++
		s.wg.Add(1)
		go s.executeWorker(node)
	}
}

// depsComplete returns true if all dependencies of the node are completed.
// Must be called with s.mu held.
func (s *Scheduler) depsComplete(node *BeadNode) bool {
	for _, depID := range node.DependsOn {
		depNode, ok := s.nodes[depID]
		if !ok {
			continue
		}
		if depNode.Status != "completed" {
			return false
		}
	}
	return true
}

// cascadeFailure marks all beads that (transitively) depend on the failed
// node as skipped. Must be called with s.mu held.
func (s *Scheduler) cascadeFailure(node *BeadNode) {
	for _, depID := range node.DependedBy {
		depNode, ok := s.nodes[depID]
		if !ok || depNode.Status != "pending" {
			continue
		}
		depNode.Status = "skipped"
		s.pool.RecordSkip()
		// Recurse to skip transitive dependents.
		s.cascadeFailure(depNode)
	}
}

// executeWorker runs a single bead in its own goroutine:
// 1. Create worktree
// 2. Pre-embed graph data
// 3. Generate MCP config
// 4. Run RetryBead
// 5. Submit merge request
func (s *Scheduler) executeWorker(node *BeadNode) {
	defer s.wg.Done()

	bead := node.Bead
	beadID := bead.ID

	// Log worker start.
	if s.logger != nil {
		_ = s.logger.Append(log.LogEvent{
			Event:    log.EventWorkerStarted,
			BeadID:   beadID,
			Title:    bead.Title,
			WorkerID: beadID,
		})
	}

	fmt.Printf("%s %s: %s (parallel worker)\n", s.pool.Progress(), beadID, bead.Title)

	// Load sidecar metadata.
	if meta, err := beads.ReadBeadMeta(s.projectRoot, beadID); err == nil {
		if len(bead.Files) == 0 && len(meta.Files) > 0 {
			bead.Files = meta.Files
		}
		bead.VerifyExtra = meta.VerifyExtra
	}

	// Mark bead as in_progress.
	if err := beads.UpdateStatus(beadID, "in_progress"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to update bead %s status: %v\n", beadID, err)
	}

	// Create worktree.
	worktreePath, err := s.worktrees.Create(beadID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating worktree for bead %s: %v\n", beadID, err)
		s.mergeQueue.Submit(MergeRequest{
			Bead:    bead,
			Success: false,
			Error:   err,
		})
		return
	}

	// Pre-embed graph data.
	graphData := preEmbedGraphData(s.kgClient, bead.Files)

	// Generate MCP config for coordinator bridge.
	mcpConfigPath := filepath.Join(worktreePath, "mcp-config.json")
	mcpConfig := s.buildMCPConfig(beadID)
	if writeErr := os.WriteFile(mcpConfigPath, mcpConfig, 0644); writeErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write MCP config for bead %s: %v\n", beadID, writeErr)
		mcpConfigPath = ""
	}

	// Build spawn opts with parallel system prompt override.
	opts := &SpawnClaudeOpts{
		WorkDir:       worktreePath,
		MCPConfigPath: mcpConfigPath,
		SystemPrompt:  s.systemPrompt + "\n\n" + prompts.ParallelSystemPrompt,
		Verbose:       s.verbose,
	}

	// Run retry loop.
	passed, retryErr := RetryBead(s.cfg, bead, graphData, s.projectRoot, s.logger, s.kgClient, opts)
	if retryErr != nil {
		fmt.Fprintf(os.Stderr, "Error during parallel bead %s execution: %v\n", beadID, retryErr)
	}

	// Log worker completion.
	if s.logger != nil {
		_ = s.logger.Append(log.LogEvent{
			Event:        log.EventWorkerCompleted,
			BeadID:       beadID,
			Title:        bead.Title,
			WorkerID:     beadID,
			WorktreePath: worktreePath,
		})
	}

	// Submit to merge queue.
	s.mergeQueue.Submit(MergeRequest{
		Bead:         bead,
		WorktreePath: worktreePath,
		BranchName:   s.worktrees.BranchName(beadID),
		GraphData:    graphData,
		Success:      passed,
		Error:        retryErr,
	})
}

// buildMCPConfig creates the MCP configuration JSON for the coordinator bridge.
func (s *Scheduler) buildMCPConfig(beadID string) []byte {
	berthBinary, _ := os.Executable()
	if berthBinary == "" {
		berthBinary = "berth"
	}

	config := map[string]any{
		"mcpServers": map[string]any{
			"coordinator": map[string]any{
				"command": berthBinary,
				"args":    []string{"_coordinator-bridge", "--addr", s.coordServer.Addr(), "--bead-id", beadID},
			},
		},
	}

	data, _ := json.Marshal(config)
	return data
}
