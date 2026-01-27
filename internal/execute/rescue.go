// rescue.go opens an interactive Claude session for manually resolving stuck beads.
package execute

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/config"
)

// RunRescue opens an interactive Claude session with full terminal access.
// The session is pre-loaded with context about the stuck bead, including
// error outputs, diagnostic analysis, and Knowledge Graph data. The user
// interacts directly with Claude to resolve the issue. On exit, the
// caller should run the verification pipeline.
func RunRescue(
	cfg config.Config,
	bead *beads.Bead,
	verifyErrors []string,
	diagnostic string,
	graphData string,
	projectRoot string,
) error {
	rescueContext := buildRescueContext(bead, verifyErrors, diagnostic, graphData)

	cmd := exec.Command(
		"claude",
		"--append-system-prompt", rescueContext,
		"--dangerously-skip-permissions",
	)
	cmd.Dir = projectRoot
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("\n--- Rescue session for bead %s: %q ---\n", bead.ID, bead.Title)
	fmt.Println("An interactive Claude session is opening with full error context.")
	fmt.Println("Type 'exit' or press Ctrl+C to end the session.")
	fmt.Println()

	if err := cmd.Run(); err != nil {
		// A non-zero exit from an interactive session is not necessarily
		// an error (user may have pressed Ctrl+C). We still return nil
		// so the caller can run verification.
		return nil
	}

	return nil
}

// buildRescueContext assembles the append-system-prompt content for the
// rescue session. It includes the bead description, all error outputs,
// the diagnostic analysis, and any Knowledge Graph context.
func buildRescueContext(bead *beads.Bead, errors []string, diagnostic string, graphData string) string {
	var b strings.Builder

	b.WriteString("## Rescue Session: ")
	b.WriteString(bead.Title)
	b.WriteString("\n")
	b.WriteString("You are rescuing a stuck bead. Here's what happened:\n\n")

	b.WriteString("### Bead Description\n")
	b.WriteString(bead.Description)
	b.WriteString("\n\n")

	b.WriteString("### Previous Errors\n")
	if len(errors) == 0 {
		b.WriteString("(no errors captured)\n")
	} else {
		for i, errOutput := range errors {
			b.WriteString(fmt.Sprintf("#### Attempt %d\n", i+1))
			b.WriteString(errOutput)
			b.WriteString("\n\n")
		}
	}

	if diagnostic != "" {
		b.WriteString("### Diagnostic Analysis\n")
		b.WriteString(diagnostic)
		b.WriteString("\n\n")
	}

	if graphData != "" {
		b.WriteString("### Code Context\n")
		b.WriteString(graphData)
		b.WriteString("\n\n")
	}

	b.WriteString("Help the user fix this. The verification pipeline must pass.\n")

	return b.String()
}
