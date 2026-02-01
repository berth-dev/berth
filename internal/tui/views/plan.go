// Package views provides TUI view components for the Berth application.
package views

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/berth-dev/berth/internal/tui"
)

// ============================================================================
// PlanModel
// ============================================================================

// PlanModel is the view model for the plan approval screen.
type PlanModel struct {
	plan              *tui.Plan
	groups            []tui.ExecutionGroup
	selectedBead      int
	expanded          map[string]bool
	showFeedbackInput bool
	feedbackInput     textinput.Model
	width             int
	height            int
}

// NewPlanModel creates a new PlanModel for the given plan and execution groups.
func NewPlanModel(plan *tui.Plan, groups []tui.ExecutionGroup, width, height int) PlanModel {
	ti := textinput.New()
	ti.Placeholder = "Enter feedback..."
	ti.CharLimit = 1000
	ti.SetWidth(width - 10)

	return PlanModel{
		plan:              plan,
		groups:            groups,
		selectedBead:      0,
		expanded:          make(map[string]bool),
		showFeedbackInput: false,
		feedbackInput:     ti,
		width:             width,
		height:            height,
	}
}

// Init returns the initial command for the plan view.
func (m PlanModel) Init() tea.Cmd {
	return nil
}

// Update handles messages for the plan view.
func (m PlanModel) Update(msg tea.Msg) (PlanModel, tea.Cmd) {
	var cmd tea.Cmd

	// Handle feedback input mode
	if m.showFeedbackInput {
		switch msg := msg.(type) {
		case tea.KeyPressMsg:
			switch msg.String() {
			case tui.KeyEsc:
				m.showFeedbackInput = false
				m.feedbackInput.Blur()
				return m, nil
			case tui.KeyEnter:
				feedback := strings.TrimSpace(m.feedbackInput.Value())
				m.showFeedbackInput = false
				m.feedbackInput.Blur()
				return m, func() tea.Msg {
					return tui.RejectMsg{Feedback: feedback}
				}
			}
		}

		m.feedbackInput, cmd = m.feedbackInput.Update(msg)
		return m, cmd
	}

	// Handle normal navigation mode
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "a":
			return m, func() tea.Msg {
				return tui.ApproveMsg{}
			}
		case "r":
			m.showFeedbackInput = true
			m.feedbackInput.Focus()
			return m, textinput.Blink
		case tui.KeyEnter:
			// Toggle expansion of selected bead
			beadID := m.getSelectedBeadID()
			if beadID != "" {
				m.expanded[beadID] = !m.expanded[beadID]
			}
			return m, nil
		case tui.KeyUp, "k":
			if m.selectedBead > 0 {
				m.selectedBead--
			}
			return m, nil
		case tui.KeyDown, "j":
			totalBeads := m.countTotalBeads()
			if m.selectedBead < totalBeads-1 {
				m.selectedBead++
			}
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.feedbackInput.SetWidth(msg.Width - 10)
		return m, nil
	}

	return m, nil
}

// View renders the plan view.
func (m PlanModel) View() string {
	var b strings.Builder

	// Header
	title := "PLAN"
	if m.plan != nil && m.plan.Title != "" {
		title = fmt.Sprintf("PLAN: %s", m.plan.Title)
	}
	header := tui.TitleStyle.Render(title)
	b.WriteString(header)
	b.WriteString("\n\n")

	// Subheader with bead count and group count
	totalBeads := m.countTotalBeads()
	groupCount := len(m.groups)
	subheader := tui.DimStyle.Render(fmt.Sprintf("%d beads in %d execution groups", totalBeads, groupCount))
	b.WriteString(subheader)
	b.WriteString("\n\n")

	// Render groups and beads
	beadIndex := 0
	for groupIdx, group := range m.groups {
		// Group header
		if group.Parallel && len(group.BeadIDs) > 1 {
			groupHeader := tui.WarningStyle.Render(fmt.Sprintf("Group %d (parallel):", groupIdx+1))
			b.WriteString(groupHeader)
		} else {
			groupHeader := tui.DimStyle.Render(fmt.Sprintf("Group %d:", groupIdx+1))
			b.WriteString(groupHeader)
		}
		b.WriteString("\n")

		// Render beads in this group
		for _, beadID := range group.BeadIDs {
			bead := m.findBead(beadID)

			// Selection indicator
			var indicator string
			if beadIndex == m.selectedBead {
				indicator = tui.SelectedStyle.Render("▸ ")
			} else {
				indicator = "  "
			}
			b.WriteString(indicator)

			// Bead ID and title
			beadTitle := ""
			if bead != nil {
				beadTitle = truncate(bead.Title, 40)
			}
			beadLine := fmt.Sprintf("%s: %s", beadID, beadTitle)
			if beadIndex == m.selectedBead {
				b.WriteString(tui.SelectedStyle.Render(beadLine))
			} else {
				b.WriteString(beadLine)
			}

			// Show dependencies if any
			if bead != nil && len(bead.DependsOn) > 0 {
				deps := tui.DimStyle.Render(fmt.Sprintf(" [%s]", strings.Join(bead.DependsOn, ", ")))
				b.WriteString(deps)
			}
			b.WriteString("\n")

			// Show expanded details if this bead is expanded
			if m.expanded[beadID] && bead != nil {
				if bead.Description != "" {
					b.WriteString("    ")
					b.WriteString(tui.DimStyle.Render(truncate(bead.Description, 60)))
					b.WriteString("\n")
				}
				if len(bead.Files) > 0 {
					b.WriteString("    ")
					b.WriteString(tui.DimStyle.Render("Files: " + strings.Join(bead.Files, ", ")))
					b.WriteString("\n")
				}
			}

			beadIndex++
		}
		b.WriteString("\n")
	}

	// Feedback input if showing
	if m.showFeedbackInput {
		b.WriteString("\n")
		b.WriteString("Enter feedback for rejection:\n")
		b.WriteString(m.feedbackInput.View())
		b.WriteString("\n\n")
		b.WriteString(tui.DimStyle.Render("Enter: Submit | Esc: Cancel"))
	}

	b.WriteString("\n")

	// Footer
	footer := tui.DimStyle.Render("[a] Approve · [r] Reject · [↑ ↓] Navigate · [Enter] Expand")
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

// findBead returns the BeadSpec for the given ID, or nil if not found.
func (m PlanModel) findBead(id string) *tui.BeadSpec {
	if m.plan == nil {
		return nil
	}
	for i := range m.plan.Beads {
		if m.plan.Beads[i].ID == id {
			return &m.plan.Beads[i]
		}
	}
	return nil
}

// getSelectedBeadID returns the bead ID at the current selection index.
func (m PlanModel) getSelectedBeadID() string {
	beadIndex := 0
	for _, group := range m.groups {
		for _, beadID := range group.BeadIDs {
			if beadIndex == m.selectedBead {
				return beadID
			}
			beadIndex++
		}
	}
	return ""
}

// countTotalBeads returns the total number of beads across all groups.
func (m PlanModel) countTotalBeads() int {
	count := 0
	for _, group := range m.groups {
		count += len(group.BeadIDs)
	}
	return count
}

// truncate truncates a string to max characters, adding "..." if truncated.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
