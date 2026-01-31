// Package views provides TUI view components for the Berth application.
package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/berth-dev/berth/internal/session"
	"github.com/berth-dev/berth/internal/tui"
)

// ============================================================================
// Message Types
// ============================================================================

// SubmitTaskMsg is sent when the user submits a task description.
type SubmitTaskMsg struct {
	Description string
}

// ResumeSessionMsg is sent when the user chooses to resume a previous session.
type ResumeSessionMsg struct {
	SessionID string
}

// ============================================================================
// HomeModel
// ============================================================================

// HomeModel is the view model for the home screen.
type HomeModel struct {
	textInput     textinput.Model
	resumeSession *session.Session
	showResume    bool
	width         int
	height        int
}

// NewHomeModel creates a new HomeModel with optional resume session.
func NewHomeModel(resumeSession *session.Session, width, height int) HomeModel {
	ti := textinput.New()
	ti.Placeholder = "type something..."
	ti.CharLimit = 2000
	ti.Width = width - 10 // Account for padding/borders
	ti.Focus()

	return HomeModel{
		textInput:     ti,
		resumeSession: resumeSession,
		showResume:    resumeSession != nil,
		width:         width,
		height:        height,
	}
}

// Init returns the initial command for the home view.
func (m HomeModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages for the home view.
func (m HomeModel) Update(msg tea.Msg) (HomeModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case tui.KeyEnter:
			value := strings.TrimSpace(m.textInput.Value())
			if value != "" {
				return m, func() tea.Msg {
					return SubmitTaskMsg{Description: value}
				}
			}
		case "r", "R":
			if m.showResume && m.resumeSession != nil {
				return m, func() tea.Msg {
					return ResumeSessionMsg{SessionID: m.resumeSession.ID}
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textInput.Width = msg.Width - 10
		return m, nil
	}

	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

// View renders the home view.
func (m HomeModel) View() string {
	var b strings.Builder

	// Header
	header := tui.TitleStyle.Render("Berth - Software Factory")
	b.WriteString(header)
	b.WriteString("\n\n")

	// Resume session hint (if available)
	if m.showResume && m.resumeSession != nil {
		resumeStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F59E0B")) // Amber/warning color

		resumeText := fmt.Sprintf("Resume: %s", m.resumeSession.Task)
		resumeLine := resumeStyle.Render(resumeText)
		hint := tui.DimStyle.Render(" (press 'r' to resume)")
		b.WriteString(resumeLine)
		b.WriteString(hint)
		b.WriteString("\n\n")
	}

	// Prompt
	prompt := "What would you like to work on?"
	b.WriteString(prompt)
	b.WriteString("\n\n")

	// Text input
	b.WriteString(m.textInput.View())
	b.WriteString("\n\n")

	// Footer with tab hints
	footer := tui.DimStyle.Render("Tab: Switch tabs       Ctrl+C: Exit")
	b.WriteString(footer)

	// Wrap in box style
	content := b.String()
	boxed := tui.BoxStyle.
		Width(m.width - 4).
		Render(content)

	// Center vertically if there's space
	contentHeight := lipgloss.Height(boxed)
	if m.height > contentHeight {
		padding := (m.height - contentHeight) / 3 // Slight offset toward top
		if padding > 0 {
			boxed = strings.Repeat("\n", padding) + boxed
		}
	}

	return boxed
}
