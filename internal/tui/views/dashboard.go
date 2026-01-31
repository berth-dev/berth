// Package views provides TUI view components for the Berth application.
package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/berth-dev/berth/internal/graph"
	"github.com/berth-dev/berth/internal/session"
	"github.com/berth-dev/berth/internal/tui"
	"github.com/berth-dev/berth/internal/tui/commands"
)

// ============================================================================
// Message Types
// ============================================================================

// LoadSessionMsg is sent when the user selects a session to load.
type LoadSessionMsg struct {
	SessionID string
}

// DeleteSessionMsg is sent when the user requests to delete a session.
type DeleteSessionMsg struct {
	SessionID string
}

// ============================================================================
// SessionItem
// ============================================================================

// SessionItem implements list.Item for the session list.
type SessionItem struct {
	session tui.SessionInfo
}

// NewSessionItem creates a new SessionItem from a SessionInfo.
func NewSessionItem(s tui.SessionInfo) SessionItem {
	return SessionItem{session: s}
}

// Title returns the session name/task for list display.
func (i SessionItem) Title() string {
	return i.session.Name
}

// Description returns the session status and date for list display.
func (i SessionItem) Description() string {
	return fmt.Sprintf("%s - %s (%d beads)",
		i.session.Status,
		i.session.CreatedAt.Format("Jan 02, 2006 15:04"),
		i.session.BeadCount,
	)
}

// FilterValue returns the value used for filtering in the list.
func (i SessionItem) FilterValue() string {
	return i.session.Name
}

// ============================================================================
// DashboardModel
// ============================================================================

// DashboardModel is the view model for the dashboard screen.
type DashboardModel struct {
	activeTab     int // 0=Architecture, 1=Learnings, 2=Sessions
	diagram       string
	learnings     []string
	sessions      []tui.SessionInfo
	sessionsError string
	sessionList   list.Model
	viewport      viewport.Model
	width         int
	height        int

	// Dependencies for loading data
	kgClient    *graph.Client
	store       *session.Store
	projectRoot string
	rootFile    string

	// Ctrl+C confirmation state
	ctrlCPending bool
}

// DashboardDeps holds the dependencies needed by the dashboard view.
type DashboardDeps struct {
	KGClient    *graph.Client
	Store       *session.Store
	ProjectRoot string
	RootFile    string
}

// maxDashboardWidth is the maximum width for the dashboard box.
const maxDashboardWidth = 110

// maxContentHeight is the maximum height for scrollable content areas.
const maxContentHeight = 15

// NewDashboardModel creates a new DashboardModel with the given data and dependencies.
func NewDashboardModel(diagram string, learnings []string, sessions []tui.SessionInfo, width, height int, deps *DashboardDeps) DashboardModel {
	// Use constrained dimensions for consistent sizing
	contentWidth := maxDashboardWidth - 8
	if contentWidth < 20 {
		contentWidth = 20
	}
	contentHeight := maxContentHeight

	vp := viewport.New(contentWidth, contentHeight)
	vp.SetContent(diagram)

	// Initialize session list
	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		items[i] = SessionItem{session: s}
	}

	// Configure list delegate for better display
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("#7C3AED")).
		BorderForeground(lipgloss.Color("#7C3AED"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color("#9CA3AF"))

	l := list.New(items, delegate, contentWidth, contentHeight)
	l.Title = "Sessions"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)

	m := DashboardModel{
		activeTab:   0,
		diagram:     diagram,
		learnings:   learnings,
		sessions:    sessions,
		sessionList: l,
		viewport:    vp,
		width:       width,
		height:      height,
	}

	// Set dependencies if provided
	if deps != nil {
		m.kgClient = deps.KGClient
		m.store = deps.Store
		m.projectRoot = deps.ProjectRoot
		m.rootFile = deps.RootFile
	}

	return m
}

// Init returns the initial command for the dashboard view.
// It triggers loading of architecture diagram, learnings, and sessions.
func (m DashboardModel) Init() tea.Cmd {
	return tea.Batch(
		commands.LoadDiagramCmd(m.kgClient, m.rootFile),
		commands.LoadLearningsCmd(m.projectRoot),
		commands.LoadSessionsCmd(m.store, 20),
	)
}

// Update handles messages for the dashboard view.
func (m DashboardModel) Update(msg tea.Msg) (DashboardModel, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "right":
			// Cycle to next internal tab
			m.activeTab = (m.activeTab + 1) % 3
			m.updateViewportContent()
			return m, nil

		case "left":
			// Cycle to previous internal tab
			m.activeTab = (m.activeTab + 2) % 3 // +2 is equivalent to -1 mod 3
			m.updateViewportContent()
			return m, nil

		case "enter":
			// If on sessions tab and a session is selected, return LoadSessionMsg
			if m.activeTab == 2 {
				if item, ok := m.sessionList.SelectedItem().(SessionItem); ok {
					return m, func() tea.Msg {
						return LoadSessionMsg{SessionID: item.session.ID}
					}
				}
			}
			return m, nil

		case "d":
			// If on sessions tab, return DeleteSessionMsg for selected session
			if m.activeTab == 2 {
				if item, ok := m.sessionList.SelectedItem().(SessionItem); ok {
					return m, func() tea.Msg {
						return DeleteSessionMsg{SessionID: item.session.ID}
					}
				}
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Viewport and list dimensions stay fixed for consistent sizing
		return m, nil

	case tui.ArchitectureDiagramMsg:
		if msg.Err != nil {
			m.diagram = "Architecture unavailable: " + msg.Err.Error()
		} else {
			m.diagram = msg.Diagram
		}
		if m.activeTab == 0 {
			m.updateViewportContent()
		}
		return m, nil

	case tui.LearningsLoadMsg:
		if msg.Err != nil {
			m.learnings = []string{"Learnings unavailable: " + msg.Err.Error()}
		} else {
			m.learnings = msg.Learnings
		}
		if m.activeTab == 1 {
			m.updateViewportContent()
		}
		return m, nil

	case tui.SessionsLoadMsg:
		if msg.Err != nil {
			m.sessions = nil
			m.sessionsError = "Failed to load sessions: " + msg.Err.Error()
		} else {
			m.sessions = msg.Sessions
			m.sessionsError = ""
			// Update session list items
			items := make([]list.Item, len(m.sessions))
			for i, s := range m.sessions {
				items[i] = SessionItem{session: s}
			}
			m.sessionList.SetItems(items)
		}
		return m, nil
	}

	// Pass messages to the appropriate component based on active tab
	switch m.activeTab {
	case 0, 1:
		// Architecture or Learnings tab - use viewport
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	case 2:
		// Sessions tab - use list
		m.sessionList, cmd = m.sessionList.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// updateViewportContent updates the viewport content based on the active tab.
func (m *DashboardModel) updateViewportContent() {
	switch m.activeTab {
	case 0:
		m.viewport.SetContent(m.diagram)
	case 1:
		if len(m.learnings) == 0 {
			m.viewport.SetContent("")
		} else {
			m.viewport.SetContent(strings.Join(m.learnings, "\n\n"))
		}
	}
}

// View renders the dashboard view.
func (m DashboardModel) View() string {
	var b strings.Builder

	// Header
	header := tui.TitleStyle.Render("Dashboard")
	b.WriteString(header)
	b.WriteString("\n\n")

	// Tab bar
	b.WriteString(renderTabs(m.activeTab))
	b.WriteString("\n\n")

	// Content based on active tab
	switch m.activeTab {
	case 0:
		// Architecture diagram
		if m.diagram == "" {
			b.WriteString(tui.DimStyle.Render("No architecture data available"))
		} else {
			b.WriteString(m.viewport.View())
		}

	case 1:
		// Learnings
		if len(m.learnings) == 0 {
			b.WriteString(tui.DimStyle.Render("No learnings yet"))
		} else {
			b.WriteString(m.viewport.View())
		}

	case 2:
		// Sessions list
		if m.sessionsError != "" {
			b.WriteString(tui.ErrorStyle.Render(m.sessionsError))
		} else if len(m.sessions) == 0 {
			b.WriteString(tui.DimStyle.Render("No sessions yet"))
		} else {
			b.WriteString(m.sessionList.View())
		}
	}

	b.WriteString("\n\n")

	// Footer with relevant keybindings per tab
	b.WriteString(m.renderFooter())

	// Determine box width - use max width or screen width, whichever is smaller
	boxWidth := maxDashboardWidth
	if m.width-4 < boxWidth {
		boxWidth = m.width - 4
	}

	// Wrap in box style with fixed max width
	content := b.String()
	boxed := tui.BoxStyle.
		Width(boxWidth).
		Render(content)

	return boxed
}

// renderTabs renders the tab bar with active highlighting.
func renderTabs(activeTab int) string {
	tabs := []string{"Architecture", "Learnings", "Sessions"}
	var rendered []string

	for i, tab := range tabs {
		if i == activeTab {
			rendered = append(rendered, tui.ActiveTabStyle.Render(tab))
		} else {
			rendered = append(rendered, tui.InactiveTabStyle.Render(tab))
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

// renderFooter renders the footer with relevant keybindings for the current tab.
func (m DashboardModel) renderFooter() string {
	var hints []string

	// Common hints
	hints = append(hints, "Tab/← →: Switch tabs")

	// Tab-specific hints
	switch m.activeTab {
	case 0, 1:
		// Architecture or Learnings - viewport controls
		hints = append(hints, "j/k: Scroll")
	case 2:
		// Sessions
		hints = append(hints, "Enter: Load session")
		hints = append(hints, "d: Delete session")
		hints = append(hints, "/: Filter")
	}

	// Build the hint string
	hintsStr := tui.DimStyle.Render(strings.Join(hints, " · "))

	// Exit hint - dynamic based on Ctrl+C state
	ctrlCHint := "Ctrl+C: Exit"
	if m.ctrlCPending {
		ctrlCHint = tui.WarningStyle.Render("Press Ctrl+C again to exit")
	} else {
		ctrlCHint = tui.DimStyle.Render(ctrlCHint)
	}

	return hintsStr + " · " + ctrlCHint
}

// SetCtrlCPending sets the Ctrl+C pending state for display.
func (m *DashboardModel) SetCtrlCPending(pending bool) {
	m.ctrlCPending = pending
}
