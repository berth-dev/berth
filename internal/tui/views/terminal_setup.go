// Package views provides TUI view components for the Berth application.
package views

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/berth-dev/berth/internal/tui"
	"github.com/berth-dev/berth/internal/tui/terminal"
)

// ============================================================================
// Message Types
// ============================================================================

// TerminalSetupCompleteMsg is sent when terminal setup is complete or skipped.
type TerminalSetupCompleteMsg struct {
	Configured bool
	Message    string
}

// ============================================================================
// TerminalSetupModel
// ============================================================================

// TerminalSetupModel is the view model for the terminal setup prompt.
type TerminalSetupModel struct {
	terminalType tui.TerminalType
	terminalName string
	selected     int // 0 = Yes, 1 = No
	width        int
	height       int
	configuring  bool
	result       *terminal.SetupResult
	err          error
}

// NewTerminalSetupModel creates a new TerminalSetupModel.
func NewTerminalSetupModel(terminalType tui.TerminalType, width, height int) TerminalSetupModel {
	return TerminalSetupModel{
		terminalType: terminalType,
		terminalName: tui.TerminalDisplayName(terminalType),
		selected:     0, // Default to Yes
		width:        width,
		height:       height,
	}
}

// Init returns the initial command for the terminal setup view.
func (m TerminalSetupModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the terminal setup view.
func (m TerminalSetupModel) Update(msg tea.Msg) (TerminalSetupModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		keyStr := msg.String()

		// If showing result, any key continues
		if m.result != nil || m.err != nil {
			return m, func() tea.Msg {
				return TerminalSetupCompleteMsg{
					Configured: m.result != nil && m.result.Success,
					Message:    m.getMessage(),
				}
			}
		}

		switch keyStr {
		case "left", "h":
			m.selected = 0
		case "right", "l":
			m.selected = 1
		case "y", "Y":
			m.selected = 0
			return m.runSetup()
		case "n", "N":
			m.selected = 1
			return m.skipSetup()
		case tui.KeyEnter:
			if m.selected == 0 {
				return m.runSetup()
			}
			return m.skipSetup()
		case tui.KeyEsc:
			return m.skipSetup()
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

func (m TerminalSetupModel) runSetup() (TerminalSetupModel, tea.Cmd) {
	m.configuring = true

	// Run the setup
	result, err := terminal.Setup(string(m.terminalType))
	m.configuring = false

	if err != nil {
		m.err = err
		return m, nil
	}

	m.result = result

	// Save config to remember setup was done
	cfg := &terminal.SetupConfig{
		SetupCompleted: result.Success,
		SetupDeclined:  false,
		TerminalType:   string(m.terminalType),
	}
	_ = terminal.SaveConfig(cfg)

	return m, nil
}

func (m TerminalSetupModel) skipSetup() (TerminalSetupModel, tea.Cmd) {
	// Save config to remember setup was declined
	cfg := &terminal.SetupConfig{
		SetupCompleted: false,
		SetupDeclined:  true,
		TerminalType:   string(m.terminalType),
	}
	_ = terminal.SaveConfig(cfg)

	return m, func() tea.Msg {
		return TerminalSetupCompleteMsg{
			Configured: false,
			Message:    "Setup skipped. You can use backslash+Enter for newlines.",
		}
	}
}

func (m TerminalSetupModel) getMessage() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %s", m.err.Error())
	}
	if m.result != nil {
		return m.result.Message
	}
	return ""
}

// View renders the terminal setup view.
func (m TerminalSetupModel) View() string {
	var b strings.Builder

	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7C3AED"))

	infoBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#374151")).
		Padding(1, 2)

	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#7C3AED")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Padding(0, 2).
		Bold(true)

	normalStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#374151")).
		Foreground(lipgloss.Color("#9CA3AF")).
		Padding(0, 2)

	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#10B981")).
		Bold(true)

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#EF4444")).
		Bold(true)

	// Title
	b.WriteString(titleStyle.Render("Terminal Setup"))
	b.WriteString("\n\n")

	// Show result if available
	if m.result != nil || m.err != nil {
		if m.err != nil {
			b.WriteString(errorStyle.Render("Setup Failed"))
			b.WriteString("\n\n")
			b.WriteString(m.err.Error())
		} else if m.result.Success {
			b.WriteString(successStyle.Render("Setup Complete!"))
			b.WriteString("\n\n")
			b.WriteString(m.result.Message)
			if m.result.NeedsRestart {
				b.WriteString("\n\n")
				b.WriteString(tui.WarningStyle.Render("Please restart your terminal for changes to take effect."))
			}
			if m.result.ConfigPath != "" {
				b.WriteString("\n\n")
				b.WriteString(tui.DimStyle.Render(fmt.Sprintf("Config: %s", m.result.ConfigPath)))
			}
		} else {
			b.WriteString(errorStyle.Render("Setup Not Available"))
			b.WriteString("\n\n")
			b.WriteString(m.result.Message)
		}
		b.WriteString("\n\n")
		b.WriteString(tui.DimStyle.Render("Press any key to continue..."))
	} else {
		// Detection info
		b.WriteString(fmt.Sprintf("Detected: %s", tui.WarningStyle.Render(m.terminalName)))
		b.WriteString("\n\n")

		// Info box
		infoText := fmt.Sprintf(
			"%s doesn't natively support Shift+Enter for newlines.\n\n"+
				"Berth can configure your terminal to enable this feature.\n"+
				"This will add a keybinding to your terminal's config file.",
			m.terminalName,
		)
		b.WriteString(infoBoxStyle.Render(infoText))
		b.WriteString("\n\n")

		// Question
		b.WriteString("Configure Shift+Enter automatically?")
		b.WriteString("\n\n")

		// Buttons
		var yesBtn, noBtn string
		if m.selected == 0 {
			yesBtn = selectedStyle.Render("Yes")
			noBtn = normalStyle.Render("No")
		} else {
			yesBtn = normalStyle.Render("Yes")
			noBtn = selectedStyle.Render("No")
		}
		buttons := lipgloss.JoinHorizontal(lipgloss.Center, yesBtn, "  ", noBtn)
		b.WriteString(buttons)
		b.WriteString("\n\n")

		// Hint
		b.WriteString(tui.DimStyle.Render("Use ←/→ to select, Enter to confirm, Esc to skip"))
	}

	// Wrap in box style
	maxWidth := 70
	boxWidth := maxWidth
	if m.width-4 < boxWidth {
		boxWidth = m.width - 4
	}

	content := b.String()
	boxed := tui.BoxStyle.
		Width(boxWidth).
		Render(content)

	// Center vertically
	contentHeight := lipgloss.Height(boxed)
	if m.height > contentHeight {
		padding := (m.height - contentHeight) / 3
		if padding > 0 {
			boxed = strings.Repeat("\n", padding) + boxed
		}
	}

	return boxed
}
