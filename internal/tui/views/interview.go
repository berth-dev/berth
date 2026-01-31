// Package views provides TUI view components for the Berth application.
package views

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/berth-dev/berth/internal/tui"
)

// ============================================================================
// OptionItem - implements list.Item interface
// ============================================================================

// OptionItem wraps a tui.Option to implement the list.Item interface.
type OptionItem struct {
	option tui.Option
}

// Title returns the option label, appending " (Recommended)" if applicable.
func (o OptionItem) Title() string {
	if o.option.Recommended {
		return o.option.Label + " (Recommended)"
	}
	return o.option.Label
}

// Description returns an empty string as options don't have descriptions.
func (o OptionItem) Description() string {
	return ""
}

// FilterValue returns the option label for filtering purposes.
func (o OptionItem) FilterValue() string {
	return o.option.Label
}

// ============================================================================
// InterviewModel
// ============================================================================

// InterviewModel is the view model for the interview screen.
type InterviewModel struct {
	question        tui.Question
	list            list.Model
	selected        int
	showCustomInput bool
	customInput     textinput.Model
	width           int
	height          int
}

// NewInterviewModel creates a new InterviewModel for the given question.
func NewInterviewModel(q tui.Question, width, height int) InterviewModel {
	// Create items slice from question options
	items := make([]list.Item, 0, len(q.Options)+3)
	for _, opt := range q.Options {
		items = append(items, OptionItem{option: opt})
	}

	// Add custom input option if allowed
	if q.AllowCustom {
		items = append(items, OptionItem{
			option: tui.Option{
				Key:   "__custom__",
				Label: "Type something...",
			},
		})
	}

	// Always add chat option
	items = append(items, OptionItem{
		option: tui.Option{
			Key:   "__chat__",
			Label: "Chat about this (get explanation, clarification...)",
		},
	})

	// Always add skip option
	items = append(items, OptionItem{
		option: tui.Option{
			Key:   "__skip__",
			Label: "Skip interview and plan immediately",
		},
	})

	// Create custom delegate with selected style
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("#7C3AED")).
		BorderLeftForeground(lipgloss.Color("#7C3AED"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color("#7C3AED")).
		BorderLeftForeground(lipgloss.Color("#7C3AED"))

	// Calculate list dimensions (account for box padding and header/footer)
	listHeight := height - 10
	if listHeight < 5 {
		listHeight = 5
	}
	listWidth := width - 8
	if listWidth < 20 {
		listWidth = 20
	}

	// Create list model
	l := list.New(items, delegate, listWidth, listHeight)
	l.Title = q.Text
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)

	// Create custom input for custom responses
	ti := textinput.New()
	ti.Placeholder = "Enter your custom response..."
	ti.CharLimit = 500
	ti.Width = listWidth - 4

	return InterviewModel{
		question:        q,
		list:            l,
		selected:        0,
		showCustomInput: false,
		customInput:     ti,
		width:           width,
		height:          height,
	}
}

// Init returns the initial command for the interview view.
func (m InterviewModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the interview view.
func (m InterviewModel) Update(msg tea.Msg) (InterviewModel, tea.Cmd) {
	var cmd tea.Cmd

	// Handle custom input mode
	if m.showCustomInput {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case tui.KeyEsc:
				m.showCustomInput = false
				m.customInput.Blur()
				return m, nil
			case tui.KeyEnter:
				value := strings.TrimSpace(m.customInput.Value())
				if value != "" {
					m.showCustomInput = false
					m.customInput.Blur()
					return m, func() tea.Msg {
						return tui.AnswerMsg{
							QuestionID: m.question.ID,
							Value:      value,
						}
					}
				}
				return m, nil
			}
		}

		m.customInput, cmd = m.customInput.Update(msg)
		return m, cmd
	}

	// Handle normal list mode
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case tui.KeyEnter:
			// Get selected item
			selectedItem, ok := m.list.SelectedItem().(OptionItem)
			if !ok {
				return m, nil
			}

			switch selectedItem.option.Key {
			case "__chat__":
				return m, func() tea.Msg {
					return tui.EnterChatMsg{QuestionID: m.question.ID}
				}
			case "__skip__":
				return m, func() tea.Msg {
					return tui.SkipInterviewMsg{}
				}
			case "__custom__":
				m.showCustomInput = true
				m.customInput.Focus()
				return m, textinput.Blink
			default:
				return m, func() tea.Msg {
					return tui.AnswerMsg{
						QuestionID: m.question.ID,
						Value:      selectedItem.option.Label,
					}
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Update list dimensions
		listHeight := msg.Height - 10
		if listHeight < 5 {
			listHeight = 5
		}
		listWidth := msg.Width - 8
		if listWidth < 20 {
			listWidth = 20
		}
		m.list.SetSize(listWidth, listHeight)
		m.customInput.Width = listWidth - 4
		return m, nil
	}

	// Pass through to list
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View renders the interview view.
func (m InterviewModel) View() string {
	var b strings.Builder

	// Header
	header := tui.TitleStyle.Render("* Understanding your requirements")
	b.WriteString(header)
	b.WriteString("\n\n")

	// Main content - list or custom input
	if m.showCustomInput {
		b.WriteString("Enter your custom response:\n\n")
		b.WriteString(m.customInput.View())
		b.WriteString("\n\n")
		b.WriteString(tui.DimStyle.Render("Enter: Submit | Esc: Cancel"))
	} else {
		b.WriteString(m.list.View())
	}

	b.WriteString("\n\n")

	// Footer
	footer := tui.DimStyle.Render("Chat about this                    Ctrl+Enter: Skip to planning")
	b.WriteString(footer)

	// Wrap in box style
	content := b.String()
	boxed := tui.BoxStyle.
		Width(m.width - 4).
		Render(content)

	// Center vertically if there's space
	contentHeight := lipgloss.Height(boxed)
	if m.height > contentHeight {
		padding := (m.height - contentHeight) / 3
		if padding > 0 {
			boxed = strings.Repeat("\n", padding) + boxed
		}
	}

	return boxed
}
