// Package tui implements the terminal user interface using Bubble Tea.
package tui

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

// Common key binding constants.
const (
	KeyCtrlC     = "ctrl+c"
	KeyCtrlEnter = "ctrl+enter"
	KeyTab       = "tab"
	KeyEnter     = "enter"
	KeyEsc       = "esc"
	KeyUp        = "up"
	KeyDown      = "down"
	KeyLeft      = "left"
	KeyRight     = "right"
)

// IsTTY returns true if stdout is connected to a terminal.
func IsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// Run starts the TUI program with the given model.
// If stdout is a TTY, it runs in alternate screen mode.
// Otherwise, it delegates to runFallback for non-interactive behavior.
func Run(m tea.Model) error {
	if IsTTY() {
		p := tea.NewProgram(m, tea.WithAltScreen())
		_, err := p.Run()
		return err
	}
	return runFallback(m)
}

// runFallback handles non-TTY execution.
// TODO: Delegate to existing CLI behavior for non-TTY
func runFallback(_ tea.Model) error {
	return nil
}
