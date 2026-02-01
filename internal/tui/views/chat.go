// Package views provides TUI view components for the Berth application.
package views

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/berth-dev/berth/internal/tui"
)

// ============================================================================
// Message Types
// ============================================================================

// SendChatMsg is sent when the user submits a chat message.
type SendChatMsg struct {
	Content string
}

// ChatResponseMsg contains the assistant's response to a chat message.
type ChatResponseMsg struct {
	Content string
}

// ExitChatMsg signals that the user wants to exit the chat view.
type ExitChatMsg struct{}

// ============================================================================
// ChatModel
// ============================================================================

// ChatModel is the view model for the free-form chat screen.
type ChatModel struct {
	messages     []tui.ChatMessage
	textarea     textarea.Model
	viewport     viewport.Model
	contextLabel string
	isLoading    bool
	spinner      spinner.Model
	width        int
	height       int
}

// NewChatModel creates a new ChatModel with the given context and initial messages.
func NewChatModel(contextLabel string, initialMessages []tui.ChatMessage, width, height int) ChatModel {
	// Initialize textarea
	ta := textarea.New()
	ta.Placeholder = "Type your message... (Enter to send)"
	ta.CharLimit = 5000
	ta.SetWidth(width - 8) // Account for box padding
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	// Configure key bindings: Shift+Enter for newline, Enter for submit
	keyMap := ta.KeyMap
	keyMap.InsertNewline = key.NewBinding(
		key.WithKeys("shift+enter", "ctrl+j"),
		key.WithHelp("shift+enter", "new line"),
	)
	ta.KeyMap = keyMap

	// Initialize spinner
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED"))

	// Calculate viewport dimensions
	// Reserve space for: header (2 lines), loading indicator (2 lines), textarea (5 lines), footer (2 lines)
	vpHeight := height - 14
	if vpHeight < 5 {
		vpHeight = 5
	}
	vpWidth := width - 8
	if vpWidth < 20 {
		vpWidth = 20
	}

	// Initialize viewport with functional options (v2 API)
	vp := viewport.New(viewport.WithWidth(vpWidth), viewport.WithHeight(vpHeight))
	vp.SetContent(formatMessages(initialMessages))

	return ChatModel{
		messages:     initialMessages,
		textarea:     ta,
		viewport:     vp,
		contextLabel: contextLabel,
		isLoading:    false,
		spinner:      sp,
		width:        width,
		height:       height,
	}
}

// Init returns the initial command for the chat view.
func (m ChatModel) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick)
}

// Update handles messages for the chat view.
func (m ChatModel) Update(msg tea.Msg) (ChatModel, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		keyStr := msg.String()

		// Enter submits
		if keyStr == tui.KeyEnter {
			// Send message if textarea has content
			content := strings.TrimSpace(m.textarea.Value())
			if content != "" {
				// Add user message to history
				m.messages = append(m.messages, tui.ChatMessage{
					Role:    "user",
					Content: content,
				})

				// Update viewport with new messages
				m.viewport.SetContent(formatMessages(m.messages))
				m.viewport.GotoBottom()

				// Clear textarea and set loading state
				m.textarea.Reset()
				m.isLoading = true

				return m, func() tea.Msg {
					return SendChatMsg{Content: content}
				}
			}
			return m, nil
		}

		// Shift+Enter or Ctrl+J inserts newline
		if keyStr == tui.KeyShiftEnter || keyStr == tui.KeyCtrlJ {
			m.textarea.InsertString("\n")
			return m, nil
		}

		if keyStr == tui.KeyEsc {
			return m, func() tea.Msg {
				return ExitChatMsg{}
			}
		}

	case ChatResponseMsg:
		// Add assistant message
		m.messages = append(m.messages, tui.ChatMessage{
			Role:    "assistant",
			Content: msg.Content,
		})

		// Update viewport and clear loading state
		m.viewport.SetContent(formatMessages(m.messages))
		m.viewport.GotoBottom()
		m.isLoading = false
		return m, nil

	case spinner.TickMsg:
		if m.isLoading {
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Recalculate dimensions
		vpHeight := msg.Height - 14
		if vpHeight < 5 {
			vpHeight = 5
		}
		vpWidth := msg.Width - 8
		if vpWidth < 20 {
			vpWidth = 20
		}

		m.viewport.SetWidth(vpWidth)
		m.viewport.SetHeight(vpHeight)
		m.textarea.SetWidth(vpWidth)

		// Re-format messages with new width
		m.viewport.SetContent(formatMessages(m.messages))
		return m, nil
	}

	// Update textarea (only if not loading)
	if !m.isLoading {
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update viewport for scrolling
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View renders the chat view.
func (m ChatModel) View() string {
	var b strings.Builder

	// Header
	header := tui.TitleStyle.Render(fmt.Sprintf("Chat: %s", m.contextLabel))
	b.WriteString(header)
	b.WriteString("\n\n")

	// Viewport showing message history
	b.WriteString(m.viewport.View())
	b.WriteString("\n\n")

	// Loading indicator or textarea
	if m.isLoading {
		loadingLine := fmt.Sprintf("%s Thinking...", m.spinner.View())
		b.WriteString(loadingLine)
		b.WriteString("\n\n")
		// Show disabled textarea
		b.WriteString(tui.DimStyle.Render(m.textarea.View()))
	} else {
		b.WriteString(m.textarea.View())
	}

	b.WriteString("\n\n")

	// Footer
	footer := tui.DimStyle.Render("Enter: Submit · Shift+Enter: New line · Esc: Back")
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

// formatMessages formats the chat message history for display in the viewport.
func formatMessages(messages []tui.ChatMessage) string {
	if len(messages) == 0 {
		return tui.DimStyle.Render("No messages yet. Start the conversation!")
	}

	var b strings.Builder

	userStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#10B981")). // Green for user
		Bold(true)

	assistantStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7C3AED")). // Purple for Claude
		Bold(true)

	for i, msg := range messages {
		var prefix string
		var style lipgloss.Style

		switch msg.Role {
		case "user":
			prefix = "You: "
			style = userStyle
		case "assistant":
			prefix = "Claude: "
			style = assistantStyle
		case "system":
			prefix = "System: "
			style = tui.DimStyle
		default:
			prefix = msg.Role + ": "
			style = tui.DimStyle
		}

		// Render the prefix in the appropriate style
		b.WriteString(style.Render(prefix))

		// Render the content
		b.WriteString(msg.Content)

		// Add spacing between messages (except after the last one)
		if i < len(messages)-1 {
			b.WriteString("\n\n")
		}
	}

	return b.String()
}
