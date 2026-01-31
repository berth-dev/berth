// Package commands provides Bubble Tea commands for TUI operations.
package commands

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/execute"
	"github.com/berth-dev/berth/internal/tui"
)

// StartExecutionCmd launches the execution loop in a background goroutine.
// The execution runs asynchronously and streams events to outputChan.
// Returns ExecutionStartedMsg to signal the TUI that execution has begun.
func StartExecutionCmd(
	cfg config.Config,
	projectRoot, runDir, branchName string,
	outputChan chan execute.StreamEvent,
) tea.Cmd {
	return func() tea.Msg {
		go func() {
			defer close(outputChan)
			// Create execution state and run with streaming output.
			// verbose=false for TUI mode, state=nil for fresh execution.
			err := execute.RunExecuteWithState(
				cfg,
				projectRoot,
				runDir,
				branchName,
				false, // verbose
				nil,   // fresh execution, no checkpoint
				outputChan,
			)
			if err != nil {
				outputChan <- execute.StreamEvent{
					Type:    "error",
					Content: err.Error(),
				}
			}
		}()
		return tui.ExecutionStartedMsg{}
	}
}

// ListenExecutionCmd polls the output channel for streaming events.
// Returns ExecutionEventMsg for each event, ExecutionCompleteMsg when the
// channel closes, or TickMsg on timeout to keep polling.
func ListenExecutionCmd(outputChan <-chan execute.StreamEvent) tea.Cmd {
	return func() tea.Msg {
		select {
		case event, ok := <-outputChan:
			if !ok {
				return tui.ExecutionCompleteMsg{} // channel closed
			}
			return tui.ExecutionEventMsg{Event: event}
		case <-time.After(100 * time.Millisecond):
			return tui.TickMsg{} // keep polling
		}
	}
}

// PauseExecutionCmd signals that execution should be paused.
// Note: actual pause requires context cancellation in the execution loop.
func PauseExecutionCmd() tea.Cmd {
	return func() tea.Msg {
		return tui.PauseMsg{Paused: true}
	}
}

// ResumeExecutionCmd signals that execution should resume.
func ResumeExecutionCmd() tea.Cmd {
	return func() tea.Msg {
		return tui.PauseMsg{Paused: false}
	}
}

// SkipBeadCmd signals to skip a specific bead during execution.
func SkipBeadCmd(beadID string) tea.Cmd {
	return func() tea.Msg {
		return tui.SkipBeadMsg{BeadID: beadID}
	}
}
