// Package views provides TUI view components for the Berth application.
package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/berth-dev/berth/internal/tui"
)

// ============================================================================
// ExecutionModel
// ============================================================================

// ExecutionModel is the view model for the bead execution screen.
type ExecutionModel struct {
	beads       []tui.BeadState
	currentBead int
	output      []string
	viewport    viewport.Model
	spinner     spinner.Model
	totalTokens int
	startTime   time.Time
	isPaused    bool
	isParallel  bool
	activeBeads []int
	width       int
	height      int
}

// NewExecutionModel creates a new ExecutionModel for bead execution.
func NewExecutionModel(beads []tui.BeadState, isParallel bool, width, height int) ExecutionModel {
	// Initialize spinner with Dot style and WarningStyle color
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = tui.WarningStyle

	// Initialize viewport for output display
	// Reserve space for header, progress, bead list, and footer
	viewportHeight := height - 16
	if viewportHeight < 5 {
		viewportHeight = 5
	}
	vp := viewport.New(width-6, viewportHeight)
	vp.SetContent("")

	return ExecutionModel{
		beads:       beads,
		currentBead: 0,
		output:      make([]string, 0),
		viewport:    vp,
		spinner:     sp,
		totalTokens: 0,
		startTime:   time.Now(),
		isPaused:    false,
		isParallel:  isParallel,
		activeBeads: make([]int, 0),
		width:       width,
		height:      height,
	}
}

// Init returns the initial command for the execution view.
func (m ExecutionModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update handles messages for the execution view.
func (m ExecutionModel) Update(msg tea.Msg) (ExecutionModel, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tui.OutputEvent:
		return m.handleOutputEvent(msg)

	case tui.BeadStartMsg:
		m.currentBead = msg.Index
		m.output = make([]string, 0)
		m.startTime = time.Now()
		m.viewport.SetContent("")
		m.viewport.GotoTop()
		// Mark the bead as running
		if msg.Index >= 0 && msg.Index < len(m.beads) {
			m.beads[msg.Index].Status = "running"
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "p":
			m.isPaused = !m.isPaused
			return m, func() tea.Msg {
				return tui.PauseMsg{Paused: m.isPaused}
			}
		case "s":
			if m.currentBead >= 0 && m.currentBead < len(m.beads) {
				beadID := m.beads[m.currentBead].ID
				return m, func() tea.Msg {
					return tui.SkipBeadMsg{BeadID: beadID}
				}
			}
			return m, nil
		case "c":
			if m.currentBead >= 0 && m.currentBead < len(m.beads) {
				beadID := m.beads[m.currentBead].ID
				return m, func() tea.Msg {
					return tui.ChatAboutBeadMsg{BeadID: beadID}
				}
			}
			return m, nil
		}

	case spinner.TickMsg:
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Recalculate viewport size
		viewportHeight := m.height - 16
		if viewportHeight < 5 {
			viewportHeight = 5
		}
		m.viewport.Width = m.width - 6
		m.viewport.Height = viewportHeight
		return m, nil
	}

	// Pass to viewport for scrolling
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// handleOutputEvent processes output events from bead execution.
func (m ExecutionModel) handleOutputEvent(event tui.OutputEvent) (ExecutionModel, tea.Cmd) {
	switch event.Type {
	case "output", "stdout", "stderr":
		// Append to output and update viewport
		m.output = append(m.output, event.Content)
		m.viewport.SetContent(strings.Join(m.output, "\n"))
		m.viewport.GotoBottom()

	case "token_update", "token":
		m.totalTokens = event.Tokens
		// Update current bead's token count
		if m.currentBead >= 0 && m.currentBead < len(m.beads) {
			m.beads[m.currentBead].TokenCount = event.Tokens
		}

	case "complete", "status":
		if event.Content == "complete" || event.Type == "complete" {
			if m.currentBead >= 0 && m.currentBead < len(m.beads) {
				m.beads[m.currentBead].Status = "success"
				m.beads[m.currentBead].Duration = time.Since(m.startTime)
			}
		}

	case "error":
		if m.currentBead >= 0 && m.currentBead < len(m.beads) {
			m.beads[m.currentBead].Status = "failed"
			m.beads[m.currentBead].Duration = time.Since(m.startTime)
		}
	}

	return m, nil
}

// View renders the execution view.
func (m ExecutionModel) View() string {
	var b strings.Builder

	// Header with current bead title
	currentTitle := "Waiting..."
	if m.currentBead >= 0 && m.currentBead < len(m.beads) {
		currentTitle = m.beads[m.currentBead].Title
	}
	header := tui.TitleStyle.Render(fmt.Sprintf("\u23fa Executing: %s", currentTitle))
	b.WriteString(header)
	b.WriteString("\n\n")

	// Progress indicator
	completed := m.countCompleted()
	total := len(m.beads)
	progress := tui.DimStyle.Render(fmt.Sprintf("Bead %d/%d", completed+1, total))
	b.WriteString(progress)
	b.WriteString("\n")

	// Progress bar
	progressBar := m.renderProgressBar(completed, total)
	b.WriteString(progressBar)
	b.WriteString("\n\n")

	// Bead list (parallel or sequential view)
	if m.isParallel && len(m.activeBeads) > 0 {
		b.WriteString(m.renderParallelView())
	} else {
		b.WriteString(m.renderSequentialView())
	}
	b.WriteString("\n")

	// Live output label
	outputLabel := tui.DimStyle.Render("[Live output from Claude]")
	b.WriteString(outputLabel)
	b.WriteString("\n")

	// Viewport with streaming output
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Token count and elapsed time
	elapsed := time.Since(m.startTime)
	stats := tui.DimStyle.Render(fmt.Sprintf("Tokens: %d | Elapsed: %s", m.totalTokens, formatDuration(elapsed)))
	b.WriteString(stats)
	b.WriteString("\n")

	// Paused indicator
	if m.isPaused {
		b.WriteString("\n")
		pausedIndicator := tui.WarningStyle.Render("[ PAUSED ]")
		b.WriteString(pausedIndicator)
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Footer with keybindings
	footer := tui.DimStyle.Render("p: Pause  s: Skip bead  c: Chat about this bead       Ctrl+C: Abort")
	b.WriteString(footer)

	// Wrap in box style
	content := b.String()
	boxed := tui.BoxStyle.
		Width(m.width - 4).
		Render(content)

	return boxed
}

// renderProgressBar renders a progress bar based on completion percentage.
func (m ExecutionModel) renderProgressBar(completed, total int) string {
	if total == 0 {
		return ""
	}

	barWidth := m.width - 10
	if barWidth < 10 {
		barWidth = 10
	}
	if barWidth > 60 {
		barWidth = 60
	}

	fillCount := 0
	if total > 0 {
		fillCount = (completed * barWidth) / total
	}
	emptyCount := barWidth - fillCount

	filled := tui.ProgressFullStyle.Render(strings.Repeat("\u2588", fillCount))
	empty := tui.ProgressEmptyStyle.Render(strings.Repeat("\u2591", emptyCount))

	return filled + empty
}

// renderSequentialView renders the bead list in sequential mode.
func (m ExecutionModel) renderSequentialView() string {
	var b strings.Builder

	// Show a window of beads around the current one
	windowSize := 5
	start := m.currentBead - windowSize/2
	if start < 0 {
		start = 0
	}
	end := start + windowSize
	if end > len(m.beads) {
		end = len(m.beads)
		start = end - windowSize
		if start < 0 {
			start = 0
		}
	}

	for i := start; i < end; i++ {
		bead := m.beads[i]
		icon := m.getStatusIcon(bead.Status, i == m.currentBead)

		title := truncate(bead.Title, 50)
		line := fmt.Sprintf("%s %s", icon, title)

		if i == m.currentBead {
			line = tui.SelectedStyle.Render(line)
			// Show spinner for current executing bead
			if bead.Status == "running" {
				line = fmt.Sprintf("%s %s", m.spinner.View(), tui.SelectedStyle.Render(title))
			}
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

// renderParallelView renders the bead list in parallel mode.
func (m ExecutionModel) renderParallelView() string {
	var b strings.Builder

	b.WriteString(tui.WarningStyle.Render("Parallel Execution:"))
	b.WriteString("\n")

	for _, beadIdx := range m.activeBeads {
		if beadIdx >= 0 && beadIdx < len(m.beads) {
			bead := m.beads[beadIdx]
			icon := m.getStatusIcon(bead.Status, true)

			// Calculate individual progress for parallel beads
			progressIndicator := ""
			if bead.TokenCount > 0 {
				progressIndicator = tui.DimStyle.Render(fmt.Sprintf(" (%d tokens)", bead.TokenCount))
			}

			title := truncate(bead.Title, 40)
			line := fmt.Sprintf("  %s %s %s%s", m.spinner.View(), icon, title, progressIndicator)
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	// Also show pending beads
	pendingCount := 0
	for _, bead := range m.beads {
		if bead.Status == "pending" {
			pendingCount++
		}
	}
	if pendingCount > 0 {
		pending := tui.DimStyle.Render(fmt.Sprintf("  %d beads pending...", pendingCount))
		b.WriteString(pending)
		b.WriteString("\n")
	}

	return b.String()
}

// getStatusIcon returns the appropriate icon for a bead's status.
func (m ExecutionModel) getStatusIcon(status string, isCurrent bool) string {
	switch status {
	case "success":
		return tui.BeadDone
	case "running":
		if isCurrent {
			return tui.BeadExecuting
		}
		return tui.BeadExecuting
	case "failed":
		return tui.BeadFailed
	case "skipped":
		return tui.BeadSkipped
	case "blocked":
		return tui.BeadPending
	default: // pending
		return tui.BeadPending
	}
}

// countCompleted returns the number of completed beads.
func (m ExecutionModel) countCompleted() int {
	count := 0
	for _, bead := range m.beads {
		if bead.Status == "success" || bead.Status == "failed" || bead.Status == "skipped" {
			count++
		}
	}
	return count
}

// formatDuration formats a duration as "Xs" or "Xm Ys".
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %ds", minutes, seconds)
}
