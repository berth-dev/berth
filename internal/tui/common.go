// Package tui implements the terminal user interface using Bubble Tea.
package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"
)

// Common key binding constants.
const (
	KeyCtrlC      = "ctrl+c"
	KeyEnter      = "enter"        // Submit
	KeyShiftEnter = "shift+enter"  // New line (native Kitty protocol)
	KeyEsc        = "esc"          // Used for ESC+CR sequence detection
	KeyTab        = "tab"
	KeyUp         = "up"
	KeyDown       = "down"
	KeyLeft  = "left"
	KeyRight = "right"
)

// TerminalType represents the detected terminal emulator.
type TerminalType string

const (
	TerminalUnknown  TerminalType = "unknown"
	TerminalITerm2   TerminalType = "iterm2"
	TerminalVSCode   TerminalType = "vscode"
	TerminalWarp     TerminalType = "warp"
	TerminalGhostty  TerminalType = "ghostty"
	TerminalWezTerm  TerminalType = "wezterm"
	TerminalKitty    TerminalType = "kitty"
	TerminalApple    TerminalType = "apple"
	TerminalAlacritty TerminalType = "alacritty"
)

// DetectTerminal returns the detected terminal type based on environment variables.
func DetectTerminal() TerminalType {
	termProgram := os.Getenv("TERM_PROGRAM")
	termEnv := os.Getenv("TERM")

	// Check TERM_PROGRAM first (most reliable)
	switch strings.ToLower(termProgram) {
	case "iterm.app":
		return TerminalITerm2
	case "vscode":
		return TerminalVSCode
	case "warpterm", "warp":
		return TerminalWarp
	case "ghostty":
		return TerminalGhostty
	case "wezterm":
		return TerminalWezTerm
	case "apple_terminal":
		return TerminalApple
	case "alacritty":
		return TerminalAlacritty
	}

	// Check for Warp via environment variable
	if os.Getenv("WARP_HONOR_PS1") != "" || os.Getenv("WARP_IS_LOCAL_SHELL_SESSION") != "" {
		return TerminalWarp
	}

	// Check for VS Code via environment variable
	if os.Getenv("VSCODE_PID") != "" || os.Getenv("TERM_PROGRAM_VERSION") != "" && termProgram == "" {
		if os.Getenv("VSCODE_GIT_IPC_HANDLE") != "" {
			return TerminalVSCode
		}
	}

	// Check TERM variable for Kitty
	if strings.Contains(termEnv, "kitty") {
		return TerminalKitty
	}

	// Check for Alacritty via TERM
	if strings.Contains(termEnv, "alacritty") {
		return TerminalAlacritty
	}

	return TerminalUnknown
}

// HasNativeShiftEnter returns true if the terminal supports Shift+Enter natively
// via the Kitty keyboard protocol.
func HasNativeShiftEnter() bool {
	terminal := DetectTerminal()
	switch terminal {
	case TerminalITerm2, TerminalGhostty, TerminalWezTerm, TerminalKitty:
		return true
	default:
		return false
	}
}

// NeedsTerminalSetup returns true if the terminal requires keybinding configuration
// to support Shift+Enter for newlines.
func NeedsTerminalSetup() bool {
	terminal := DetectTerminal()
	switch terminal {
	case TerminalVSCode, TerminalWarp, TerminalApple, TerminalAlacritty, TerminalUnknown:
		return true
	default:
		return false
	}
}

// TerminalDisplayName returns a human-readable name for the terminal.
func TerminalDisplayName(t TerminalType) string {
	switch t {
	case TerminalITerm2:
		return "iTerm2"
	case TerminalVSCode:
		return "VS Code"
	case TerminalWarp:
		return "Warp"
	case TerminalGhostty:
		return "Ghostty"
	case TerminalWezTerm:
		return "WezTerm"
	case TerminalKitty:
		return "Kitty"
	case TerminalApple:
		return "Terminal.app"
	case TerminalAlacritty:
		return "Alacritty"
	default:
		return "Unknown terminal"
	}
}

// IsTTY returns true if stdout is connected to a terminal.
func IsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// Run starts the TUI program with the given model.
// If stdout is a TTY, it runs with the Bubble Tea program.
// Otherwise, it delegates to runFallback for non-interactive behavior.
// Note: Bubble Tea v2 automatically requests basic keyboard disambiguation
// from the terminal (Kitty protocol). For terminals that support it
// (iTerm2, Kitty, Ghostty, WezTerm, Alacritty), Shift+Enter detection works.
// For terminals without support (Warp, VS Code, Terminal.app), use
// backslash+Enter as fallback.
func Run(m tea.Model) error {
	if IsTTY() {
		p := tea.NewProgram(m)
		_, err := p.Run()
		return err
	}
	return runFallback(m)
}

// runFallback handles non-TTY execution.
// It uses the FallbackRunner to guide users to the appropriate CLI commands.
func runFallback(_ tea.Model) error {
	fmt.Println("Non-TTY environment detected.")
	fmt.Println("Please use 'berth run <description>' for non-interactive execution.")
	return nil
}
