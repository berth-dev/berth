// Package commands provides Bubble Tea commands for TUI operations.
package commands

import (
	tea "charm.land/bubbletea/v2"

	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/execute"
	"github.com/berth-dev/berth/internal/plan"
	"github.com/berth-dev/berth/internal/tui"
	"github.com/berth-dev/berth/internal/understand"
)

// GeneratePlanCmd generates a plan from requirements.
// It spawns Claude to create an execution plan based on the gathered requirements,
// then computes execution groups for parallel bead execution.
// Returns PlanGeneratedMsg with the plan and groups, or PlanErrorMsg on failure.
func GeneratePlanCmd(
	cfg config.Config,
	requirements *understand.Requirements,
	graphSummary, runDir string,
	isGreenfield bool,
) tea.Cmd {
	return func() tea.Msg {
		planResult, err := plan.RunPlanNonInteractive(
			cfg,
			&plan.Requirements{Title: requirements.Title, Content: requirements.Content},
			graphSummary,
			runDir,
			isGreenfield,
			"", // no feedback for initial generation
		)
		if err != nil {
			return tui.PlanErrorMsg{Err: err}
		}

		tuiPlan := plan.ConvertToTUIPlan(planResult)
		executionBeads := plan.ConvertToExecutionBeads(planResult.Beads)
		groups := execute.ComputeGroups(executionBeads)
		tuiGroups := convertGroups(groups)

		return tui.PlanGeneratedMsg{Plan: tuiPlan, Groups: tuiGroups}
	}
}

// RegeneratePlanCmd regenerates plan with user feedback.
// It spawns Claude to create a new execution plan incorporating the user's feedback.
// Returns PlanGeneratedMsg with the updated plan and groups, or PlanErrorMsg on failure.
func RegeneratePlanCmd(
	cfg config.Config,
	requirements *understand.Requirements,
	graphSummary, runDir string,
	isGreenfield bool,
	feedback string,
) tea.Cmd {
	return func() tea.Msg {
		planResult, err := plan.RunPlanNonInteractive(
			cfg,
			&plan.Requirements{Title: requirements.Title, Content: requirements.Content},
			graphSummary,
			runDir,
			isGreenfield,
			feedback,
		)
		if err != nil {
			return tui.PlanErrorMsg{Err: err}
		}

		tuiPlan := plan.ConvertToTUIPlan(planResult)
		executionBeads := plan.ConvertToExecutionBeads(planResult.Beads)
		groups := execute.ComputeGroups(executionBeads)
		tuiGroups := convertGroups(groups)

		return tui.PlanGeneratedMsg{Plan: tuiPlan, Groups: tuiGroups}
	}
}

// convertGroups converts execute.ExecutionGroup to tui.ExecutionGroup.
func convertGroups(groups []execute.ExecutionGroup) []tui.ExecutionGroup {
	result := make([]tui.ExecutionGroup, len(groups))
	for i, g := range groups {
		result[i] = tui.ExecutionGroup{
			Index:    g.Index,
			BeadIDs:  g.BeadIDs,
			Parallel: g.Parallel,
		}
	}
	return result
}
