// Package views provides TUI view components for the Berth application.
package views

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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

// questionAnswer stores the answer for a question.
type questionAnswer struct {
	value  string   // For single-select
	values []string // For multi-select
}

// InterviewModel is the view model for the interview screen.
type InterviewModel struct {
	// All questions for this round
	questions []tui.Question

	// Answer tracking - map by question ID for random access
	answers map[string]*questionAnswer

	// Navigation state
	currentQ   int  // Current question index (0-based)
	isOnSubmit bool // True when on Submit review screen

	// Option selection (for current question)
	options        []interviewOption
	selectedOption int
	selectedValues map[string]bool // For multi-select: option key -> selected

	// Custom input
	customInput textinput.Model

	// Submit screen state
	submitFocused int // 0=Submit, 1=Go back

	// UI state
	escPending bool
	width      int
	height     int
}

// NewInterviewModel creates a new InterviewModel for all questions in a round.
func NewInterviewModel(questions []tui.Question, width, height int) InterviewModel {
	// Create custom input
	ti := textinput.New()
	ti.Placeholder = "Type your answer here..."
	ti.CharLimit = 500
	ti.SetWidth(maxInterviewWidth - 12)

	m := InterviewModel{
		questions:      questions,
		answers:        make(map[string]*questionAnswer),
		currentQ:       0,
		isOnSubmit:     false,
		selectedOption: 0,
		selectedValues: make(map[string]bool),
		customInput:    ti,
		submitFocused:  0,
		width:          width,
		height:         height,
	}

	// Load first question if we have any
	if len(questions) > 0 {
		m.loadCurrentQuestion()
	}

	return m
}

// loadCurrentQuestion builds the options list for the current question
// and restores any previously selected values.
func (m *InterviewModel) loadCurrentQuestion() {
	if m.currentQ < 0 || m.currentQ >= len(m.questions) {
		return
	}

	q := m.questions[m.currentQ]

	// Build options list
	m.options = make([]interviewOption, 0, len(q.Options)+3)

	// Add question options
	for _, opt := range q.Options {
		m.options = append(m.options, interviewOption{
			key:         opt.Key,
			label:       opt.Label,
			description: opt.Description,
			recommended: opt.Recommended,
		})
	}

	// Add custom input option if allowed
	if q.AllowCustom {
		m.options = append(m.options, interviewOption{
			key:      "__custom__",
			label:    "Type something...",
			isCustom: true,
		})
	}

	// Add meta options (after separator)
	m.options = append(m.options, interviewOption{
		key:    "__chat__",
		label:  "Chat about this",
		isMeta: true,
	})
	m.options = append(m.options, interviewOption{
		key:    "__skip__",
		label:  "Skip interview and plan immediately",
		isMeta: true,
	})

	// Reset selection state
	m.selectedOption = 0
	m.selectedValues = make(map[string]bool)

	// Restore previously selected values if we have an answer for this question
	if answer, ok := m.answers[q.ID]; ok {
		if q.MultiSelect && len(answer.values) > 0 {
			// Restore multi-select values
			for _, v := range answer.values {
				// Find the option key for this value
				for _, opt := range m.options {
					if opt.label == v {
						m.selectedValues[opt.key] = true
						break
					}
				}
			}
		} else if answer.value != "" {
			// For single-select, highlight the selected option
			for i, opt := range m.options {
				if opt.label == answer.value {
					m.selectedOption = i
					break
				}
			}
		}
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
	// Handle Esc reset timeout
	if _, ok := msg.(EscResetMsg); ok {
		m.escPending = false
		return m, nil
	}

	// Handle window resize
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsm.Width
		m.height = wsm.Height
		return m, nil
	}

	// Route based on whether we're on submit screen or question
	if m.isOnSubmit {
		return m.updateSubmitScreen(msg)
	}

	return m.updateQuestionScreen(msg)
}

// updateSubmitScreen handles input on the submit review screen.
func (m InterviewModel) updateSubmitScreen(msg tea.Msg) (InterviewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case tui.KeyLeft, "h":
			// Focus Submit button
			m.submitFocused = 0
			return m, nil

		case tui.KeyRight, "l":
			// Focus Go back button
			m.submitFocused = 1
			return m, nil

		case tui.KeyEnter, " ":
			if m.submitFocused == 0 {
				// Submit all answers
				return m, m.submitAllAnswers()
			}
			// Go back to last question
			m.isOnSubmit = false
			m.currentQ = len(m.questions) - 1
			m.loadCurrentQuestion()
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
		}
	}

	return m, nil
}

// updateQuestionScreen handles input on a question screen.
func (m InterviewModel) updateQuestionScreen(msg tea.Msg) (InterviewModel, tea.Cmd) {
	var cmd tea.Cmd

	// Check if currently on custom option (auto-typing mode)
	isOnCustomOption := m.selectedOption >= 0 && m.selectedOption < len(m.options) && m.options[m.selectedOption].isCustom

	// Handle typing mode (when on custom option)
	if isOnCustomOption {
		switch msg := msg.(type) {
		case tea.KeyPressMsg:
			switch msg.String() {
			case tui.KeyEnter:
				// Submit custom value
				value := strings.TrimSpace(m.customInput.Value())
				if value != "" {
					m.customInput.Blur()
					m.saveAnswer(value, nil)
					return m.advanceToNext()
				}
				return m, nil

			case tui.KeyUp, "k":
				// Navigate up - blur input and move selection
				m.customInput.Blur()
				if m.selectedOption > 0 {
					m.selectedOption--
				}
				return m, nil

			case tui.KeyDown, "j":
				// Navigate down - blur input and move selection
				m.customInput.Blur()
				if m.selectedOption < len(m.options)-1 {
					m.selectedOption++
				}
				return m, nil

			case tui.KeyLeft:
				// Navigate to previous question
				return m.navigatePrev()

			case tui.KeyRight:
				// Navigate to next question
				return m.navigateNext()

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
	case tea.KeyPressMsg:
		switch msg.String() {
		case tui.KeyUp, "k":
			if m.selectedOption > 0 {
				m.selectedOption--
			}
			// Auto-focus if landing on custom option
			return m, m.checkAutoFocus()

		case tui.KeyDown, "j":
			if m.selectedOption < len(m.options)-1 {
				m.selectedOption++
			}
			// Auto-focus if landing on custom option
			return m, m.checkAutoFocus()

		case tui.KeyLeft:
			// Navigate to previous question
			return m.navigatePrev()

		case tui.KeyRight:
			// Navigate to next question
			return m.navigateNext()

		case " ":
			// Space: toggle for multi-select, select for single-select
			if m.currentQ >= 0 && m.currentQ < len(m.questions) && m.questions[m.currentQ].MultiSelect {
				return m.toggleMultiSelect()
			}
			return m.handleSelection()

		case tui.KeyEnter:
			return m.handleSelection()

		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			// Quick navigate by number (user must press Enter to confirm)
			idx := int(msg.String()[0] - '1')
			if idx >= 0 && idx < len(m.options) {
				m.selectedOption = idx
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
	}

	return m, nil
}

// navigatePrev moves to the previous question.
func (m InterviewModel) navigatePrev() (InterviewModel, tea.Cmd) {
	if m.isOnSubmit {
		m.isOnSubmit = false
		m.currentQ = len(m.questions) - 1
		m.loadCurrentQuestion()
	} else if m.currentQ > 0 {
		m.currentQ--
		m.loadCurrentQuestion()
	}
	return m, nil
}

// navigateNext moves to the next question or submit screen.
func (m InterviewModel) navigateNext() (InterviewModel, tea.Cmd) {
	if m.currentQ < len(m.questions)-1 {
		m.currentQ++
		m.loadCurrentQuestion()
	} else if !m.isOnSubmit {
		m.isOnSubmit = true
	}
	return m, nil
}

// toggleMultiSelect toggles the current option in multi-select mode.
func (m InterviewModel) toggleMultiSelect() (InterviewModel, tea.Cmd) {
	if m.selectedOption < 0 || m.selectedOption >= len(m.options) {
		return m, nil
	}

	opt := m.options[m.selectedOption]

	// Don't toggle meta options or custom
	if opt.isMeta || opt.isCustom {
		return m.handleSelection()
	}

	// Toggle selection
	m.selectedValues[opt.key] = !m.selectedValues[opt.key]
	return m, nil
}

// handleSelection processes the currently selected option.
func (m InterviewModel) handleSelection() (InterviewModel, tea.Cmd) {
	if m.selectedOption < 0 || m.selectedOption >= len(m.options) {
		return m, nil
	}

	opt := m.options[m.selectedOption]

	switch opt.key {
	case "__chat__":
		return m, func() tea.Msg {
			return tui.EnterChatMsg{QuestionID: m.questions[m.currentQ].ID}
		}
	case "__skip__":
		return m, func() tea.Msg {
			return tui.SkipInterviewMsg{}
		}
	case "__custom__":
		// Submit custom value if there's content
		value := strings.TrimSpace(m.customInput.Value())
		if value != "" {
			m.saveAnswer(value, nil)
			return m.advanceToNext()
		}
		// No content - stay on this option (input is already focused)
		return m, nil
	default:
		// Regular option selected
		q := m.questions[m.currentQ]

		if q.MultiSelect {
			// For multi-select, confirm all selections
			var values []string
			for _, o := range m.options {
				if m.selectedValues[o.key] && !o.isMeta && !o.isCustom {
					values = append(values, o.label)
				}
			}
			m.saveAnswer("", values)
		} else {
			// Single select
			m.saveAnswer(opt.label, nil)
		}

		return m.advanceToNext()
	}
}

// saveAnswer stores the answer for the current question.
func (m *InterviewModel) saveAnswer(value string, values []string) {
	if m.currentQ < 0 || m.currentQ >= len(m.questions) {
		return
	}

	q := m.questions[m.currentQ]
	m.answers[q.ID] = &questionAnswer{
		value:  value,
		values: values,
	}
}

// advanceToNext moves to the next question or submit screen.
func (m InterviewModel) advanceToNext() (InterviewModel, tea.Cmd) {
	if m.currentQ < len(m.questions)-1 {
		m.currentQ++
		m.loadCurrentQuestion()
	} else {
		// Last question answered, go to submit screen
		m.isOnSubmit = true
	}
	return m, nil
}

// submitAllAnswers creates the SubmitAllAnswersMsg with all collected answers.
func (m *InterviewModel) submitAllAnswers() tea.Cmd {
	answers := make([]tui.Answer, 0, len(m.answers))

	for _, q := range m.questions {
		if answer, ok := m.answers[q.ID]; ok {
			answers = append(answers, tui.Answer{
				ID:     q.ID,
				Value:  answer.value,
				Values: answer.values,
			})
		}
	}

	return func() tea.Msg {
		return tui.SubmitAllAnswersMsg{Answers: answers}
	}
}

// checkAutoFocus returns a command to focus the input if on custom option.
func (m *InterviewModel) checkAutoFocus() tea.Cmd {
	if m.selectedOption >= 0 && m.selectedOption < len(m.options) && m.options[m.selectedOption].isCustom {
		m.customInput.Focus()
		return textinput.Blink
	}
	return nil
}

// View renders the interview view.
func (m InterviewModel) View() string {
	if m.isOnSubmit {
		return m.renderSubmitScreen()
	}
	return m.renderQuestionScreen()
}

// renderNavBar renders the horizontal navigation bar.
func (m InterviewModel) renderNavBar() string {
	var parts []string

	// Styles
	activeStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#7C3AED")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Padding(0, 1)

	answeredStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#10B981"))

	unansweredStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6B7280"))

	arrowStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6B7280"))

	submitActiveStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#10B981")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Padding(0, 1)

	// Left arrow
	if m.currentQ > 0 || m.isOnSubmit {
		parts = append(parts, arrowStyle.Render("<"))
	} else {
		parts = append(parts, arrowStyle.Render(" "))
	}

	parts = append(parts, " ")

	// Question indicators
	for i, q := range m.questions {
		var label string
		if q.ShortLabel != "" {
			label = q.ShortLabel
		} else {
			label = fmt.Sprintf("Q%d", i+1)
		}

		_, answered := m.answers[q.ID]
		var icon string
		if answered {
			icon = "+"
		} else {
			icon = "o"
		}

		item := fmt.Sprintf("%s %s", icon, label)

		if i == m.currentQ && !m.isOnSubmit {
			parts = append(parts, activeStyle.Render(item))
		} else if answered {
			parts = append(parts, answeredStyle.Render(item))
		} else {
			parts = append(parts, unansweredStyle.Render(item))
		}

		parts = append(parts, " ")
	}

	// Submit indicator
	submitLabel := "Submit"
	if m.isOnSubmit {
		parts = append(parts, submitActiveStyle.Render(submitLabel))
	} else if m.allQuestionsAnswered() {
		parts = append(parts, answeredStyle.Render(submitLabel))
	} else {
		parts = append(parts, unansweredStyle.Render(submitLabel))
	}

	parts = append(parts, " ")

	// Right arrow
	if !m.isOnSubmit {
		parts = append(parts, arrowStyle.Render(">"))
	} else {
		parts = append(parts, arrowStyle.Render(" "))
	}

	return strings.Join(parts, "")
}

// allQuestionsAnswered returns true if all questions have answers.
func (m InterviewModel) allQuestionsAnswered() bool {
	for _, q := range m.questions {
		if _, ok := m.answers[q.ID]; !ok {
			return false
		}
	}
	return true
}

// renderQuestionScreen renders the current question.
func (m InterviewModel) renderQuestionScreen() string {
	if m.currentQ < 0 || m.currentQ >= len(m.questions) {
		return "No questions available"
	}

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

	descriptionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9CA3AF")).
		Italic(true)

	q := m.questions[m.currentQ]
	isMultiSelect := q.MultiSelect

	// Navigation bar
	navBar := m.renderNavBar()
	b.WriteString(navBar)
	b.WriteString("\n\n")

	// Header
	b.WriteString(titleStyle.Render("Understanding your requirements"))
	b.WriteString("\n\n")

	// Question
	b.WriteString(questionStyle.Render(q.Text))
	if isMultiSelect {
		b.WriteString(dimStyle.Render(" (multi-select)"))
	}
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
			separator := strings.Repeat("-", 60)
			b.WriteString(separatorStyle.Render(separator))
			b.WriteString("\n\n")
		}

		isSelected := i == m.selectedOption

		// Build option line
		var line strings.Builder

		// Selection indicator
		if isSelected {
			line.WriteString("> ")
		} else {
			line.WriteString("  ")
		}

		// Number prefix
		line.WriteString(fmt.Sprintf("%d. ", optionNum))
		optionNum++

		// Checkbox for multi-select (non-meta options only)
		if isMultiSelect && !opt.isMeta && !opt.isCustom {
			if m.selectedValues[opt.key] {
				line.WriteString("[x] ")
			} else {
				line.WriteString("[ ] ")
			}
		}

		// Option label
		label := opt.label
		if opt.recommended {
			label += " (Recommended)"
		}

		// For custom option when selected, show the input inline
		if opt.isCustom && isSelected {
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
			b.WriteString(descriptionStyle.Render(opt.description))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	// Footer
	var footerHint string
	if isMultiSelect {
		footerHint = "Space to toggle · Enter to confirm · arrows to navigate"
	} else {
		footerHint = "Enter to select · arrows to navigate"
	}
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

// renderSubmitScreen renders the submit review screen.
func (m InterviewModel) renderSubmitScreen() string {
	var b strings.Builder

	// Styles
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7C3AED")).
		Bold(true)

	questionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E5E7EB")).
		Bold(true)

	answerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#10B981"))

	unansweredStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F59E0B"))

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6B7280"))

	buttonActiveStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#7C3AED")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Padding(0, 2)

	buttonInactiveStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#374151")).
		Foreground(lipgloss.Color("#9CA3AF")).
		Padding(0, 2)

	warningStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#F59E0B"))

	// Navigation bar
	navBar := m.renderNavBar()
	b.WriteString(navBar)
	b.WriteString("\n\n")

	// Header
	b.WriteString(titleStyle.Render("Review Your Answers"))
	b.WriteString("\n\n")

	// Warning if incomplete
	if !m.allQuestionsAnswered() {
		b.WriteString(warningStyle.Render("! You have not answered all questions"))
		b.WriteString("\n\n")
	}

	// List all Q&A
	for _, q := range m.questions {
		// Question bullet
		b.WriteString("* ")
		b.WriteString(questionStyle.Render(q.Text))
		b.WriteString("\n")

		// Answer
		if answer, ok := m.answers[q.ID]; ok {
			b.WriteString("  -> ")
			if q.MultiSelect && len(answer.values) > 0 {
				b.WriteString(answerStyle.Render(strings.Join(answer.values, ", ")))
			} else if answer.value != "" {
				b.WriteString(answerStyle.Render(answer.value))
			} else {
				b.WriteString(unansweredStyle.Render("(empty)"))
			}
		} else {
			b.WriteString("  -> ")
			b.WriteString(unansweredStyle.Render("(not answered)"))
		}
		b.WriteString("\n\n")
	}

	// Buttons
	var submitButton, backButton string
	if m.submitFocused == 0 {
		submitButton = buttonActiveStyle.Render("Submit answers")
		backButton = buttonInactiveStyle.Render("Go back")
	} else {
		submitButton = buttonInactiveStyle.Render("Submit answers")
		backButton = buttonActiveStyle.Render("Go back")
	}

	b.WriteString(submitButton)
	b.WriteString("  ")
	b.WriteString(backButton)
	b.WriteString("\n\n")

	// Footer hint
	b.WriteString(dimStyle.Render("<-/-> toggle · Enter to confirm"))

	// Esc hint
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
