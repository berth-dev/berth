// groups.go computes parallel execution groups from bead dependencies.
package execute

import (
	"sort"

	"github.com/berth-dev/berth/internal/beads"
)

// ExecutionGroup represents a set of beads that can be executed together.
// All beads in a group have no dependencies on each other.
type ExecutionGroup struct {
	Index    int      // group index, 0-based
	BeadIDs  []string // beads in this group
	Parallel bool     // true if len(BeadIDs) > 1
}

// ComputeGroups computes parallel execution groups from bead dependencies.
// It uses topological sort with level grouping (Kahn's algorithm) to group
// beads by "level" - all beads at the same level have no dependencies on
// each other and can be executed in parallel.
// Returns groups in execution order (level 0 first, then level 1, etc.).
func ComputeGroups(allBeads []beads.Bead) []ExecutionGroup {
	if len(allBeads) == 0 {
		return nil
	}

	// Build bead ID set for filtering valid dependencies.
	beadSet := make(map[string]bool, len(allBeads))
	for _, b := range allBeads {
		beadSet[b.ID] = true
	}

	// Build dependency graph: inDegree tracks how many unresolved dependencies each bead has.
	inDegree := make(map[string]int, len(allBeads))
	// rdeps maps bead ID to the list of beads that depend on it.
	rdeps := make(map[string][]string, len(allBeads))

	for _, b := range allBeads {
		inDegree[b.ID] = 0
	}

	for _, b := range allBeads {
		for _, dep := range b.DependsOn {
			// Only count dependencies that exist in our bead set.
			if beadSet[dep] {
				inDegree[b.ID]++
				rdeps[dep] = append(rdeps[dep], b.ID)
			}
		}
	}

	// Track remaining beads to process.
	remaining := make(map[string]bool, len(allBeads))
	for _, b := range allBeads {
		remaining[b.ID] = true
	}

	var groups []ExecutionGroup
	level := 0

	// Process beads level by level using Kahn's algorithm.
	for len(remaining) > 0 {
		// Find all beads with inDegree == 0 (no unresolved dependencies).
		var ready []string
		for id := range remaining {
			if inDegree[id] == 0 {
				ready = append(ready, id)
			}
		}

		// If no beads are ready but some remain, there's a cycle.
		// Include all remaining beads in final group to avoid infinite loop.
		if len(ready) == 0 {
			for id := range remaining {
				ready = append(ready, id)
			}
		}

		// Sort for deterministic ordering.
		sort.Strings(ready)

		// Create execution group for this level.
		group := ExecutionGroup{
			Index:    level,
			BeadIDs:  ready,
			Parallel: len(ready) > 1,
		}
		groups = append(groups, group)

		// Remove processed beads from remaining and update inDegree for dependents.
		for _, id := range ready {
			delete(remaining, id)
			// Decrement inDegree for all beads that depend on this one.
			for _, dependent := range rdeps[id] {
				if remaining[dependent] {
					inDegree[dependent]--
				}
			}
		}

		level++
	}

	return groups
}

// GetBeadByID finds a bead by ID in the given slice.
// Returns nil if not found.
func GetBeadByID(allBeads []beads.Bead, id string) *beads.Bead {
	for i := range allBeads {
		if allBeads[i].ID == id {
			return &allBeads[i]
		}
	}
	return nil
}
