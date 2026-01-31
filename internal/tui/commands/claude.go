// Package commands provides Bubble Tea commands for TUI operations.
package commands

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/execute"
	"github.com/berth-dev/berth/internal/tui"
)

// SpawnClaudeCmd starts a Claude subprocess and returns immediately.
// The Claude process runs in a goroutine and streams output to outputChan.
// Returns ClaudeStartMsg to signal the TUI that Claude has been spawned.
func SpawnClaudeCmd(
	cfg config.Config,
	systemPrompt, taskPrompt, projectRoot, beadID string,
	outputChan chan execute.StreamEvent,
) tea.Cmd {
	return func() tea.Msg {
		opts := &execute.SpawnClaudeOpts{
			OutputChan: outputChan,
			BeadID:     beadID,
		}

		go func() {
			defer close(outputChan)
			execute.SpawnClaude(cfg, systemPrompt, taskPrompt, projectRoot, opts)
		}()

		return tui.ClaudeStartMsg{BeadID: beadID}
	}
}

// ListenOutputCmd polls the output channel for streaming events from Claude.
// Returns ClaudeOutputMsg for each event, or ClaudeCompleteMsg when the channel closes.
func ListenOutputCmd(outputChan <-chan execute.StreamEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-outputChan
		if !ok {
			return tui.ClaudeCompleteMsg{}
		}
		return tui.ClaudeOutputMsg{
			BeadID:   event.BeadID,
			Content:  event.Content,
			IsStderr: event.IsStderr,
		}
	}
}
