// Package views provides TUI view components for the Berth application.
package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/berth-dev/berth/internal/detect"
	"github.com/berth-dev/berth/internal/tui"
)

// ============================================================================
// InitModel
// ============================================================================

// InitModel is the view model for the initialization prompt screen.
type InitModel struct {
	width         int
	height        int
	selected      int // 0 = Yes (Initialize), 1 = No (Exit)
	stackInfo     detect.StackInfo
	projectName   string
	isDetecting   bool
	ctrlCPending  bool // Whether waiting for second Ctrl+C
}

// NewInitModel creates a new InitModel.
func NewInitModel(width, height int, projectName string) InitModel {
	return InitModel{
		width:       width,
		height:      height,
		selected:    0, // Default to "Yes"
		projectName: projectName,
		isDetecting: false,
	}
}

// Init returns the initial command for the init view.
func (m InitModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the init view.
func (m InitModel) Update(msg tea.Msg) (InitModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h":
			if m.selected > 0 {
				m.selected--
			}
		case "right", "l":
			if m.selected < 1 {
				m.selected++
			}
		case "tab":
			m.selected = (m.selected + 1) % 2
		case "enter", " ":
			if m.selected == 0 {
				return m, func() tea.Msg {
					return tui.InitConfirmMsg{}
				}
			}
			return m, func() tea.Msg {
				return tui.InitDeclineMsg{}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	}

	return m, nil
}

// View renders the init view.
func (m InitModel) View() string {
	var b strings.Builder

	// Header with welcome message
	header := tui.TitleStyle.Render("Welcome to Berth")
	b.WriteString(header)
	b.WriteString("\n\n")

	// Project detection info
	projectLine := fmt.Sprintf("Project: %s", m.projectName)
	b.WriteString(tui.DimStyle.Render(projectLine))
	b.WriteString("\n\n")

	// Main message
	messageStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E5E7EB"))

	b.WriteString(messageStyle.Render("This project hasn't been initialized with Berth yet."))
	b.WriteString("\n\n")

	// What init does - styled as a subtle info box
	infoBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#4B5563")).
		Padding(0, 1).
		Foreground(lipgloss.Color("#9CA3AF"))

	infoContent := strings.Join([]string{
		"Initialization will:",
		"  - Create .berth/ configuration directory",
		"  - Detect your project's tech stack",
		"  - Set up the task management system",
		"  - Generate CLAUDE.md for AI context",
	}, "\n")

	b.WriteString(infoBoxStyle.Render(infoContent))
	b.WriteString("\n\n")

	// Question
	questionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F3F4F6")).
		Bold(true)

	b.WriteString(questionStyle.Render("Would you like to initialize Berth now?"))
	b.WriteString("\n\n")

	// Options
	yesStyle := lipgloss.NewStyle().
		Padding(0, 2)
	noStyle := lipgloss.NewStyle().
		Padding(0, 2)

	if m.selected == 0 {
		yesStyle = yesStyle.
			Background(lipgloss.Color("#7C3AED")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true)
		noStyle = noStyle.
			Foreground(lipgloss.Color("#9CA3AF"))
	} else {
		yesStyle = yesStyle.
			Foreground(lipgloss.Color("#9CA3AF"))
		noStyle = noStyle.
			Background(lipgloss.Color("#7C3AED")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true)
	}

	yesBtn := yesStyle.Render("Yes, initialize")
	noBtn := noStyle.Render("No, exit")

	buttons := lipgloss.JoinHorizontal(lipgloss.Center, yesBtn, "  ", noBtn)
	b.WriteString(buttons)
	b.WriteString("\n\n")

	// Keyboard hints
	ctrlCHint := "Ctrl+C: Exit"
	if m.ctrlCPending {
		ctrlCHint = tui.WarningStyle.Render("Press Ctrl+C again to exit")
	} else {
		ctrlCHint = tui.DimStyle.Render(ctrlCHint)
	}
	hints := tui.DimStyle.Render("← →: Select · Enter: Confirm · ") + ctrlCHint
	b.WriteString(hints)

	// Determine box width - use max width or screen width, whichever is smaller
	const maxInitBoxWidth = 70
	boxWidth := maxInitBoxWidth
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

// SetStackInfo sets the detected stack info for display.
func (m *InitModel) SetStackInfo(info detect.StackInfo) {
	m.stackInfo = info
}

// SetCtrlCPending sets the Ctrl+C pending state for display.
func (m *InitModel) SetCtrlCPending(pending bool) {
	m.ctrlCPending = pending
}
