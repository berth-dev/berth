package tui

import "charm.land/lipgloss/v2"

// Color constants matching Claude Code aesthetic.
const (
	primaryColor   = "#7C3AED" // Purple
	secondaryColor = "#10B981" // Green
	warningColor   = "#F59E0B" // Amber
	errorColor     = "#EF4444" // Red
	dimColor       = "#6B7280" // Gray
)

// Style variables for consistent TUI rendering.
var (
	// BoxStyle provides a rounded border box with primary color.
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(primaryColor)).
			Padding(1, 2)

	// TitleStyle renders titles in primary color with bold.
	TitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(primaryColor)).
			Bold(true)

	// SelectedStyle highlights selected items in primary color.
	SelectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(primaryColor)).
			Bold(true)

	// DimStyle renders dim/muted text.
	DimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(dimColor))

	// SuccessStyle renders success messages in green.
	SuccessStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(secondaryColor))

	// ErrorStyle renders error messages in red.
	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(errorColor))

	// WarningStyle renders warning messages in amber.
	WarningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(warningColor))

	// StatusBarStyle provides styling for the status bar.
	StatusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#1F2937")).
			Foreground(lipgloss.Color("#9CA3AF")).
			Padding(0, 1)

	// ActiveTabStyle renders the active tab.
	ActiveTabStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(primaryColor)).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 2)

	// InactiveTabStyle renders inactive tabs.
	InactiveTabStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#374151")).
				Foreground(lipgloss.Color("#9CA3AF")).
				Padding(0, 2)

	// ProgressFullStyle renders filled progress indicators.
	ProgressFullStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(secondaryColor))

	// ProgressEmptyStyle renders empty progress indicators.
	ProgressEmptyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(dimColor))
)

// Bead status icon variables (pre-rendered strings).
var (
	// BeadDone indicates a completed bead.
	BeadDone = SuccessStyle.Render("\u2713")

	// BeadExecuting indicates a currently running bead.
	BeadExecuting = WarningStyle.Render("\u25b8")

	// BeadPending indicates a bead waiting to execute.
	BeadPending = DimStyle.Render("\u25cb")

	// BeadFailed indicates a failed bead.
	BeadFailed = ErrorStyle.Render("\u2717")

	// BeadSkipped indicates a skipped bead.
	BeadSkipped = DimStyle.Render("\u2298")
)
