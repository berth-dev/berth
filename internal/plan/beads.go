// beads.go creates beads from the parsed plan using bd create.
package plan

import (
	"fmt"

	"github.com/berth-dev/berth/internal/beads"
)

// CreateBeads creates beads in the beads system for each bead spec in the plan,
// then wires up dependencies between them. It maps plan IDs (bt-1, bt-2, etc.)
// to the actual bead IDs returned by the beads CLI.
func CreateBeads(plan *Plan) error {
	idMap := make(map[string]string) // plan ID -> actual bead ID

	for _, spec := range plan.Beads {
		actualID, err := beads.Create(spec.Title, spec.Description)
		if err != nil {
			return fmt.Errorf("creating bead %s: %w", spec.ID, err)
		}
		idMap[spec.ID] = actualID
		fmt.Printf("  Created bead %s -> %s\n", spec.ID, actualID)
	}

	for _, spec := range plan.Beads {
		for _, dep := range spec.DependsOn {
			actualChild := idMap[spec.ID]
			actualParent := idMap[dep]
			if actualParent == "" {
				return fmt.Errorf("dependency %s not found for bead %s", dep, spec.ID)
			}
			if err := beads.AddDependency(actualChild, actualParent); err != nil {
				return fmt.Errorf("adding dependency %s -> %s: %w", spec.ID, dep, err)
			}
			fmt.Printf("  Dependency: %s depends on %s\n", spec.ID, dep)
		}
	}

	return nil
}
