// Package commands provides Bubble Tea commands for TUI operations.
package commands

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/berth-dev/berth/internal/context"
	"github.com/berth-dev/berth/internal/graph"
	"github.com/berth-dev/berth/internal/session"
	"github.com/berth-dev/berth/internal/tui"
	"github.com/berth-dev/berth/internal/tui/diagram"
)

// LoadDiagramCmd fetches architecture diagram from KG.
func LoadDiagramCmd(kgClient *graph.Client, rootFile string) tea.Cmd {
	return func() tea.Msg {
		if kgClient == nil {
			return tui.ArchitectureDiagramMsg{
				Diagram: "Architecture unavailable (KG not connected)",
			}
		}

		nodes, err := kgClient.GetArchitectureDiagram(rootFile, 5)
		if err != nil {
			return tui.ArchitectureDiagramMsg{Err: err}
		}

		ascii := diagram.GenerateASCII(nodes)
		return tui.ArchitectureDiagramMsg{Diagram: ascii}
	}
}

// LoadLearningsCmd fetches learnings from context.
func LoadLearningsCmd(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		learnings := context.ReadLearnings(projectRoot)
		return tui.LearningsLoadMsg{Learnings: learnings}
	}
}

// LoadSessionsCmd fetches sessions from store.
func LoadSessionsCmd(store *session.Store, limit int) tea.Cmd {
	return func() tea.Msg {
		if store == nil {
			return tui.SessionsLoadMsg{
				Err: fmt.Errorf("session store not available"),
			}
		}

		summaries, err := store.ListSessions(limit)
		if err != nil {
			return tui.SessionsLoadMsg{Err: err}
		}

		sessions := convertSummaries(summaries)
		return tui.SessionsLoadMsg{Sessions: sessions}
	}
}

// convertSummaries converts session.Summary to tui.SessionInfo.
func convertSummaries(summaries []session.Summary) []tui.SessionInfo {
	sessions := make([]tui.SessionInfo, len(summaries))
	for i, s := range summaries {
		sessions[i] = tui.SessionInfo{
			ID:        s.ID,
			Name:      s.Task,
			CreatedAt: s.UpdatedAt,
			Status:    s.Status,
			BeadCount: s.BeadsTotal,
		}
	}
	return sessions
}
