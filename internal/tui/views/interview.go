// Package views provides TUI view components for the Berth application.
package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/berth-dev/berth/internal/tui"
)

// ============================================================================
// InterviewModel
// ============================================================================

// maxInterviewWidth is the maximum width for the interview box.
const maxInterviewWidth = 90

// interviewOption represents a selectable option in the interview.
type interviewOption struct {
	key         string
	label       string
	description string
	recommended bool
	isCustom    bool
	isMeta      bool // true for chat/skip options (shown after separator)
}

// InterviewModel is the view model for the interview screen.
type InterviewModel struct {
	question    tui.Question
	options     []interviewOption
	selected    int
	customInput textinput.Model
	escPending  bool // true when waiting for second Esc press
	width       int
	height      int
}

// NewInterviewModel creates a new InterviewModel for the given question.
func NewInterviewModel(q tui.Question, width, height int) InterviewModel {
	// Build options list
	options := make([]interviewOption, 0, len(q.Options)+3)

	// Add question options
	for _, opt := range q.Options {
		options = append(options, interviewOption{
			key:         opt.Key,
			label:       opt.Label,
			recommended: opt.Recommended,
		})
	}

	// Add custom input option if allowed
	if q.AllowCustom {
		options = append(options, interviewOption{
			key:      "__custom__",
			label:    "Type something...",
			isCustom: true,
		})
	}

	// Add meta options (after separator)
	options = append(options, interviewOption{
		key:    "__chat__",
		label:  "Chat about this",
		isMeta: true,
	})
	options = append(options, interviewOption{
		key:    "__skip__",
		label:  "Skip interview and plan immediately",
		isMeta: true,
	})

	// Create custom input
	ti := textinput.New()
	ti.Placeholder = "Type your answer here..."
	ti.CharLimit = 500
	ti.Width = maxInterviewWidth - 12

	return InterviewModel{
		question:    q,
		options:     options,
		selected:    0,
		customInput: ti,
		width:       width,
		height:      height,
	}
}

// Init returns the initial command for the interview view.
func (m InterviewModel) Init() tea.Cmd {
	return nil
}

// EscResetMsg resets the Esc pending state after timeout.
type EscResetMsg struct{}

// Update handles messages for the interview view.
func (m InterviewModel) Update(msg tea.Msg) (InterviewModel, tea.Cmd) {
	var cmd tea.Cmd

	// Handle Esc reset timeout
	if _, ok := msg.(EscResetMsg); ok {
		m.escPending = false
		return m, nil
	}

	// Check if currently on custom option (auto-typing mode)
	isOnCustomOption := m.selected >= 0 && m.selected < len(m.options) && m.options[m.selected].isCustom

	// Handle typing mode (when on custom option)
	if isOnCustomOption {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case tui.KeyEnter:
				// Submit custom value
				value := strings.TrimSpace(m.customInput.Value())
				if value != "" {
					m.customInput.Blur()
					return m, func() tea.Msg {
						return tui.AnswerMsg{
							QuestionID: m.question.ID,
							Value:      value,
						}
					}
				}
				return m, nil
			case tui.KeyUp, "k":
				// Navigate up - blur input and move selection
				m.customInput.Blur()
				if m.selected > 0 {
					m.selected--
				}
				return m, nil
			case tui.KeyDown, "j":
				// Navigate down - blur input and move selection
				m.customInput.Blur()
				if m.selected < len(m.options)-1 {
					m.selected++
				}
				return m, nil
			case tui.KeyEsc:
				// Handle Esc for going home (double-press)
				if m.escPending {
					return m, func() tea.Msg {
						return tui.GoHomeMsg{}
					}
				}
				m.escPending = true
				return m, tea.Tick(time.Second, func(t time.Time) tea.Msg {
					return EscResetMsg{}
				})
			default:
				// Update text input for typing
				m.customInput, cmd = m.customInput.Update(msg)
				return m, cmd
			}
		default:
			m.customInput, cmd = m.customInput.Update(msg)
			return m, cmd
		}
	}

	// Handle normal navigation mode
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case tui.KeyUp, "k":
			if m.selected > 0 {
				m.selected--
			}
			// Auto-focus if landing on custom option
			return m, m.checkAutoFocus()

		case tui.KeyDown, "j":
			if m.selected < len(m.options)-1 {
				m.selected++
			}
			// Auto-focus if landing on custom option
			return m, m.checkAutoFocus()

		case tui.KeyEnter, " ":
			return m.handleSelection()

		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// Quick navigate by number (user must press Enter to confirm)
			idx := int(msg.String()[0] - '1')
			if idx >= 0 && idx < len(m.options) {
				m.selected = idx
			}
			return m, nil

		case tui.KeyEsc:
			if m.escPending {
				// Second press - go back to home
				return m, func() tea.Msg {
					return tui.GoHomeMsg{}
				}
			}
			// First press - set pending and start timeout
			m.escPending = true
			return m, tea.Tick(time.Second, func(t time.Time) tea.Msg {
				return EscResetMsg{}
			})
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	}

	return m, nil
}

// handleSelection processes the currently selected option.
func (m InterviewModel) handleSelection() (InterviewModel, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.options) {
		return m, nil
	}

	opt := m.options[m.selected]

	switch opt.key {
	case "__chat__":
		return m, func() tea.Msg {
			return tui.EnterChatMsg{QuestionID: m.question.ID}
		}
	case "__skip__":
		return m, func() tea.Msg {
			return tui.SkipInterviewMsg{}
		}
	case "__custom__":
		// Submit custom value if there's content
		value := strings.TrimSpace(m.customInput.Value())
		if value != "" {
			return m, func() tea.Msg {
				return tui.AnswerMsg{
					QuestionID: m.question.ID,
					Value:      value,
				}
			}
		}
		// No content - stay on this option (input is already focused)
		return m, nil
	default:
		// Regular option selected
		return m, func() tea.Msg {
			return tui.AnswerMsg{
				QuestionID: m.question.ID,
				Value:      opt.label,
			}
		}
	}
}

// checkAutoFocus returns a command to focus the input if on custom option.
func (m *InterviewModel) checkAutoFocus() tea.Cmd {
	if m.selected >= 0 && m.selected < len(m.options) && m.options[m.selected].isCustom {
		m.customInput.Focus()
		return textinput.Blink
	}
	return nil
}

// View renders the interview view.
func (m InterviewModel) View() string {
	var b strings.Builder

	// Styles
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7C3AED")).
		Bold(true)

	questionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E5E7EB")).
		Bold(true).
		MarginBottom(1)

	selectedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7C3AED")).
		Bold(true)

	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF"))

	recommendedStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#10B981"))

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6B7280"))

	separatorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#374151"))

	// Header
	b.WriteString(titleStyle.Render("Understanding your requirements"))
	b.WriteString("\n\n")

	// Question
	b.WriteString(questionStyle.Render(m.question.Text))
	b.WriteString("\n\n")

	// Track where meta options start (for separator)
	metaStartIdx := -1
	for i, opt := range m.options {
		if opt.isMeta {
			metaStartIdx = i
			break
		}
	}

	// Render options
	optionNum := 1
	for i, opt := range m.options {
		// Add separator before meta options
		if i == metaStartIdx && metaStartIdx > 0 {
			separator := strings.Repeat("─", 60)
			b.WriteString(separatorStyle.Render(separator))
			b.WriteString("\n\n")
		}

		isSelected := i == m.selected

		// Build option line
		var line strings.Builder

		// Selection indicator
		if isSelected {
			line.WriteString("❯ ")
		} else {
			line.WriteString("  ")
		}

		// Number prefix
		line.WriteString(fmt.Sprintf("%d. ", optionNum))
		optionNum++

		// Option label
		label := opt.label
		if opt.recommended {
			label += " (Recommended)"
		}

		// For custom option when selected, show the input inline
		if opt.isCustom && isSelected {
			// Show the text input inline
			b.WriteString(line.String())
			b.WriteString(m.customInput.View())
			b.WriteString("\n")
			continue
		}

		// Apply styling based on selection and type
		var styledLabel string
		if isSelected {
			styledLabel = selectedStyle.Render(label)
		} else if opt.recommended {
			styledLabel = recommendedStyle.Render(label)
		} else if opt.isMeta {
			styledLabel = dimStyle.Render(label)
		} else {
			styledLabel = normalStyle.Render(label)
		}

		line.WriteString(styledLabel)
		b.WriteString(line.String())
		b.WriteString("\n")

		// Show description if present and selected
		if opt.description != "" && isSelected {
			b.WriteString("     ")
			b.WriteString(dimStyle.Render(opt.description))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	// Footer
	footerHint := "Enter to select · ↑↓ to navigate"
	b.WriteString(dimStyle.Render(footerHint))

	// Esc hint - dynamic based on pending state
	b.WriteString(" · ")
	if m.escPending {
		escHint := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F59E0B")).
			Render("Press Esc again to go back to Home")
		b.WriteString(escHint)
	} else {
		b.WriteString(dimStyle.Render("Esc: Home"))
	}

	// Determine box width
	boxWidth := maxInterviewWidth
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
