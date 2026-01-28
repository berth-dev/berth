// reconciler.go spawns targeted fix agents to resolve post-merge verification
// failures using KG impact analysis for surgical precision.
package execute

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/graph"
	"github.com/berth-dev/berth/internal/log"
	"github.com/berth-dev/berth/prompts"
)

const maxReconcileAttempts = 2

// reconcilerData holds the template data for the reconciler prompt.
type reconcilerData struct {
	BeadTitle       string
	BeadDescription string
	FailedStep      string
	VerifyOutput    string
	ImpactData      string
}

// Reconcile attempts to fix post-merge verification failures by querying
// KG impact data and spawning a targeted fix agent. Returns true if the
// fix agent successfully resolves the verification failure.
func Reconcile(
	cfg config.Config,
	bead *beads.Bead,
	worktreePath string,
	projectRoot string,
	kgClient *graph.Client,
	logger *log.Logger,
) (bool, error) {
	if logger != nil {
		_ = logger.Append(log.LogEvent{
			Event:  log.EventReconcileStarted,
			BeadID: bead.ID,
			Title:  bead.Title,
		})
	}

	for attempt := 1; attempt <= maxReconcileAttempts; attempt++ {
		// Run verification on trunk to get error details.
		verifyResult, err := RunVerification(cfg, bead, projectRoot)
		if err != nil {
			return false, fmt.Errorf("reconciler verify check: %w", err)
		}
		if verifyResult.Passed {
			// Already passing — nothing to reconcile.
			return true, nil
		}

		// Query KG per-file for impact analysis (same pattern as preEmbedGraphData).
		impactData := buildImpactData(kgClient, bead.Files)

		// Build reconciler prompt from template.
		taskPrompt, tmplErr := buildReconcilerPrompt(bead, verifyResult, impactData)
		if tmplErr != nil {
			return false, fmt.Errorf("building reconciler prompt: %w", tmplErr)
		}

		systemPrompt := prompts.ExecutorSystemPrompt

		// Spawn fix agent on trunk (projectRoot), not worktree.
		output, spawnErr := SpawnClaude(cfg, systemPrompt, taskPrompt, projectRoot, nil)
		if spawnErr != nil {
			fmt.Printf("  Reconcile attempt %d failed (spawn): %v\n", attempt, spawnErr)
			continue
		}
		if output.IsError {
			fmt.Printf("  Reconcile attempt %d failed (claude error): %s\n", attempt, output.Result)
			continue
		}

		// Re-verify after fix.
		reVerify, reErr := RunVerification(cfg, bead, projectRoot)
		if reErr != nil {
			continue
		}
		if reVerify.Passed {
			if logger != nil {
				_ = logger.Append(log.LogEvent{
					Event:   log.EventReconcileCompleted,
					BeadID:  bead.ID,
					Title:   bead.Title,
					Attempt: attempt,
				})
			}
			return true, nil
		}

		fmt.Printf("  Reconcile attempt %d: verification still failing at %q\n", attempt, reVerify.FailedStep)
	}

	if logger != nil {
		_ = logger.Append(log.LogEvent{
			Event:  log.EventReconcileFailed,
			BeadID: bead.ID,
			Title:  bead.Title,
		})
	}

	return false, fmt.Errorf("reconciliation failed after %d attempts for bead %s", maxReconcileAttempts, bead.ID)
}

// buildImpactData queries KG for per-file impact analysis and formats it
// as a string. Follows the same deduplication pattern as preEmbedGraphData
// in loop.go.
func buildImpactData(kgClient *graph.Client, files []string) string {
	if kgClient == nil || len(files) == 0 {
		return "(no impact data available)"
	}

	impact := &graph.ImpactAnalysis{}
	seenDirect := make(map[string]bool)
	seenTransitive := make(map[string]bool)
	seenTests := make(map[string]bool)

	for _, file := range files {
		result, err := kgClient.AnalyzeImpact(file)
		if err != nil || result == nil {
			continue
		}
		for _, d := range result.DirectDependents {
			key := d.File + "|" + d.Kind + "|" + d.Name
			if !seenDirect[key] {
				seenDirect[key] = true
				impact.DirectDependents = append(impact.DirectDependents, d)
			}
		}
		for _, t := range result.TransitiveDependents {
			key := t.File + "|" + t.Via
			if !seenTransitive[key] {
				seenTransitive[key] = true
				impact.TransitiveDependents = append(impact.TransitiveDependents, t)
			}
		}
		for _, s := range result.AffectedTests {
			if !seenTests[s] {
				seenTests[s] = true
				impact.AffectedTests = append(impact.AffectedTests, s)
			}
		}
	}

	var b strings.Builder

	if len(impact.DirectDependents) > 0 {
		b.WriteString("Direct dependents:\n")
		for _, d := range impact.DirectDependents {
			b.WriteString(fmt.Sprintf("  - %s: %s (%s)\n", d.File, d.Name, d.Kind))
		}
	}
	if len(impact.TransitiveDependents) > 0 {
		b.WriteString("Transitive dependents:\n")
		for _, t := range impact.TransitiveDependents {
			b.WriteString(fmt.Sprintf("  - %s (via %s)\n", t.File, t.Via))
		}
	}
	if len(impact.AffectedTests) > 0 {
		b.WriteString("Affected tests:\n")
		for _, s := range impact.AffectedTests {
			b.WriteString(fmt.Sprintf("  - %s\n", s))
		}
	}

	if b.Len() == 0 {
		return "(no impact data available)"
	}

	return b.String()
}

// buildReconcilerPrompt renders the reconciler template with bead and
// verification data.
func buildReconcilerPrompt(bead *beads.Bead, verifyResult *VerifyResult, impactData string) (string, error) {
	tmpl, err := template.New("reconciler").Parse(prompts.ReconcilerTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing reconciler template: %w", err)
	}

	data := reconcilerData{
		BeadTitle:       bead.Title,
		BeadDescription: bead.Description,
		FailedStep:      verifyResult.FailedStep,
		VerifyOutput:    verifyResult.Output,
		ImpactData:      impactData,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing reconciler template: %w", err)
	}

	return buf.String(), nil
}
