// Package views provides TUI view components for the Berth application.
package views

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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

// HomeEscResetMsg resets the ESC pending state after timeout.
type HomeEscResetMsg struct{}

// ============================================================================
// HomeModel
// ============================================================================

// HomeModel is the view model for the home screen.
type HomeModel struct {
	textArea                textarea.Model
	resumeSession           *session.Session
	showResume              bool
	width                   int
	height                  int
	Err                     error
	ctrlCPending            bool
	escPending              bool // For ESC+CR sequence detection (terminals without native Shift+Enter)
	hasKeyboardEnhancements bool // True if terminal supports Shift+Enter natively
}

// maxBoxWidth is the maximum width for the home view box.
const maxBoxWidth = 80

// NewHomeModel creates a new HomeModel with optional resume session.
func NewHomeModel(resumeSession *session.Session, width, height int) HomeModel {
	ta := textarea.New()
	ta.Placeholder = "Describe what you'd like to build or work on..."
	ta.CharLimit = 10000
	ta.SetWidth(maxBoxWidth - 6) // Account for padding/borders
	ta.SetHeight(1)              // Start with 1 line, will grow as needed
	ta.Focus()

	// Configure key bindings: Shift+Enter for newline (native Kitty protocol)
	// ESC+CR sequence is handled separately in Update() for configured terminals
	keyMap := ta.KeyMap
	keyMap.InsertNewline = key.NewBinding(
		key.WithKeys("shift+enter"),
		key.WithHelp("shift+enter", "new line"),
	)
	ta.KeyMap = keyMap

	// Style the textarea (v2 API uses SetStyles and Styles())
	styles := ta.Styles()
	styles.Focused.CursorLine = lipgloss.NewStyle()
	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	styles.Focused.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))
	styles.Focused.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED"))
	ta.SetStyles(styles)
	ta.Prompt = "> "
	ta.ShowLineNumbers = false

	return HomeModel{
		textArea:      ta,
		resumeSession: resumeSession,
		showResume:    resumeSession != nil,
		width:         width,
		height:        height,
	}
}

// Init returns the initial command for the home view.
func (m HomeModel) Init() tea.Cmd {
	return textarea.Blink
}

// Update handles messages for the home view.
func (m HomeModel) Update(msg tea.Msg) (HomeModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyboardEnhancementsMsg:
		// Terminal reported its keyboard enhancement capabilities
		// If we receive this message, the terminal supports the Kitty keyboard protocol
		// which means Shift+Enter detection works
		m.hasKeyboardEnhancements = true
		return m, nil

	case HomeEscResetMsg:
		// Timeout expired - reset ESC pending state
		m.escPending = false
		return m, nil

	case tea.KeyPressMsg:
		keyStr := msg.String()

		// ESC+CR sequence detection (for terminals configured via /terminal-setup)
		// If ESC was pressed and now Enter is pressed within timeout, insert newline
		if m.escPending && keyStr == tui.KeyEnter {
			m.escPending = false
			m.textArea.InsertString("\n")
			m.adjustTextAreaHeight()
			return m, nil
		}

		// Track ESC key for ESC+CR sequence
		if keyStr == tui.KeyEsc {
			m.escPending = true
			// Reset after 200ms if no CR follows
			return m, tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
				return HomeEscResetMsg{}
			})
		}

		// Reset ESC pending on any other key
		m.escPending = false

		// Enter submits (only if not part of ESC+CR sequence)
		if keyStr == tui.KeyEnter {
			text := m.textArea.Value()

			// Backslash+Enter = newline (fallback for terminals without keyboard enhancements)
			// This works like Claude Code: type \ then Enter to insert a newline
			if strings.HasSuffix(text, "\\") {
				// Remove trailing backslash and add newline
				newText := text[:len(text)-1] + "\n"
				m.textArea.SetValue(newText)
				m.textArea.CursorEnd()
				m.adjustTextAreaHeight()
				return m, nil
			}

			// Normal submit
			value := strings.TrimSpace(text)
			if value != "" {
				return m, func() tea.Msg {
					return SubmitTaskMsg{Description: value}
				}
			}
			return m, nil
		}

		// Shift+Enter inserts newline (native Kitty protocol support)
		if keyStr == tui.KeyShiftEnter {
			m.textArea.InsertString("\n")
			m.adjustTextAreaHeight()
			return m, nil
		}

		// Handle 'r' to resume (only if textarea is empty)
		if keyStr == "r" || keyStr == "R" {
			if m.showResume && m.resumeSession != nil && m.textArea.Value() == "" {
				return m, func() tea.Msg {
					return ResumeSessionMsg{SessionID: m.resumeSession.ID}
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Adjust textarea width based on box width
		boxWidth := maxBoxWidth
		if m.width-4 < boxWidth {
			boxWidth = m.width - 4
		}
		m.textArea.SetWidth(boxWidth - 6)
		return m, nil
	}

	// Update textarea and adjust height based on content
	m.textArea, cmd = m.textArea.Update(msg)

	// Dynamically adjust height based on content - no clipping
	m.adjustTextAreaHeight()

	return m, cmd
}

// adjustTextAreaHeight calculates and sets the textarea height based on content.
func (m *HomeModel) adjustTextAreaHeight() {
	content := m.textArea.Value()
	if content == "" {
		m.textArea.SetHeight(1)
		return
	}

	// Calculate visible width for wrapping
	visibleWidth := m.textArea.Width() - 2 // Account for prompt "> "

	// Count total lines including wrapped lines
	totalLines := 0
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if len(line) == 0 {
			totalLines++
		} else {
			// Calculate how many visual lines this logical line takes
			wrappedLines := (len(line) + visibleWidth - 1) / visibleWidth
			if wrappedLines < 1 {
				wrappedLines = 1
			}
			totalLines += wrappedLines
		}
	}

	// Set height to fit all content (minimum 1)
	if totalLines < 1 {
		totalLines = 1
	}
	m.textArea.SetHeight(totalLines)
}

// View renders the home view.
func (m HomeModel) View() string {
	var b strings.Builder

	// Header
	header := tui.TitleStyle.Render("Berth - Software Factory")
	b.WriteString(header)
	b.WriteString("\n\n")

	// Display error if present
	if m.Err != nil {
		errorBox := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")).
			Background(lipgloss.Color("#3D0000")).
			Padding(0, 1).
			Render(fmt.Sprintf("Error: %s", m.Err.Error()))
		b.WriteString(errorBox)
		b.WriteString("\n\n")
	}

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

	// Text area
	b.WriteString(m.textArea.View())
	b.WriteString("\n\n")

	// Footer with hints - show appropriate newline hint based on terminal capability
	ctrlCHint := "Ctrl+C: Exit"
	if m.ctrlCPending {
		ctrlCHint = tui.WarningStyle.Render("Press Ctrl+C again to exit")
	} else {
		ctrlCHint = tui.DimStyle.Render(ctrlCHint)
	}
	newlineHint := "\\+Enter: New line"
	if m.hasKeyboardEnhancements {
		newlineHint = "Shift+Enter: New line"
	}
	footer := tui.DimStyle.Render("Enter: Submit · "+newlineHint+" · Tab: Switch tabs · ") + ctrlCHint
	b.WriteString(footer)

	// Determine box width - use max width or screen width, whichever is smaller
	boxWidth := maxBoxWidth
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

// GetBoxWidth returns the actual box width used for centering calculations.
func (m HomeModel) GetBoxWidth() int {
	boxWidth := maxBoxWidth
	if m.width-4 < boxWidth {
		boxWidth = m.width - 4
	}
	return boxWidth + 4 // Account for border
}

// SetCtrlCPending sets the Ctrl+C pending state for display.
func (m *HomeModel) SetCtrlCPending(pending bool) {
	m.ctrlCPending = pending
}

// SetHasKeyboardEnhancements sets whether the terminal supports keyboard enhancements.
func (m *HomeModel) SetHasKeyboardEnhancements(has bool) {
	m.hasKeyboardEnhancements = has
}
