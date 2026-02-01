// Package terminal provides terminal detection and setup utilities.
package terminal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SetupConfig tracks the terminal setup state.
type SetupConfig struct {
	SetupCompleted bool   `json:"setup_completed"`
	SetupDeclined  bool   `json:"setup_declined"`
	TerminalType   string `json:"terminal_type"`
}

// configPath returns the path to the terminal setup config file.
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".berth", "terminal-setup.json"), nil
}

// LoadConfig loads the terminal setup configuration.
func LoadConfig() (*SetupConfig, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &SetupConfig{}, nil
	}
	if err != nil {
		return nil, err
	}

	var cfg SetupConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveConfig saves the terminal setup configuration.
func SaveConfig(cfg *SetupConfig) error {
	path, err := configPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SetupResult contains the result of a terminal setup operation.
type SetupResult struct {
	Success     bool
	Message     string
	NeedsRestart bool
	ConfigPath  string
}

// SetupVSCode configures VS Code keybindings for Shift+Enter.
func SetupVSCode() (*SetupResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	// VS Code keybindings.json locations (macOS, Linux, Windows)
	possiblePaths := []string{
		filepath.Join(home, "Library", "Application Support", "Code", "User", "keybindings.json"),
		filepath.Join(home, ".config", "Code", "User", "keybindings.json"),
		filepath.Join(home, "AppData", "Roaming", "Code", "User", "keybindings.json"),
		// Cursor (VS Code fork)
		filepath.Join(home, "Library", "Application Support", "Cursor", "User", "keybindings.json"),
		filepath.Join(home, ".config", "Cursor", "User", "keybindings.json"),
	}

	var targetPath string
	for _, p := range possiblePaths {
		dir := filepath.Dir(p)
		if _, err := os.Stat(dir); err == nil {
			targetPath = p
			break
		}
	}

	if targetPath == "" {
		return &SetupResult{
			Success: false,
			Message: "Could not find VS Code configuration directory",
		}, nil
	}

	// The keybinding we want to add
	newBinding := map[string]interface{}{
		"key":     "shift+enter",
		"command": "workbench.action.terminal.sendSequence",
		"args": map[string]string{
			"text": "\u001b\r",
		},
		"when": "terminalFocus",
	}

	// Read existing keybindings or create empty array
	var keybindings []map[string]interface{}
	data, err := os.ReadFile(targetPath)
	if err == nil {
		// Remove comments (VS Code allows JSON with comments)
		cleanData := removeJSONComments(string(data))
		if err := json.Unmarshal([]byte(cleanData), &keybindings); err != nil {
			// If parsing fails, start fresh
			keybindings = []map[string]interface{}{}
		}
	}

	// Check if binding already exists
	for _, binding := range keybindings {
		if key, ok := binding["key"].(string); ok && key == "shift+enter" {
			if cmd, ok := binding["command"].(string); ok && cmd == "workbench.action.terminal.sendSequence" {
				return &SetupResult{
					Success:    true,
					Message:    "Shift+Enter keybinding already configured",
					ConfigPath: targetPath,
				}, nil
			}
		}
	}

	// Add new binding
	keybindings = append(keybindings, newBinding)

	// Write back
	output, err := json.MarshalIndent(keybindings, "", "  ")
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(targetPath, output, 0644); err != nil {
		return nil, err
	}

	return &SetupResult{
		Success:      true,
		Message:      "VS Code keybinding configured successfully",
		NeedsRestart: true,
		ConfigPath:   targetPath,
	}, nil
}

// SetupWarp configures Warp keybindings for Shift+Enter.
func SetupWarp() (*SetupResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	// Warp config directory
	warpDir := filepath.Join(home, ".warp")
	keybindingsPath := filepath.Join(warpDir, "keybindings.yaml")

	// Ensure directory exists
	if err := os.MkdirAll(warpDir, 0755); err != nil {
		return nil, err
	}

	// The keybinding we want to add
	newBinding := map[string]interface{}{
		"key":    "shift-enter",
		"action": "send_text",
		"text":   "\x1b\r", // ESC + CR
	}

	// Read existing keybindings or create empty
	var keybindings []map[string]interface{}
	data, err := os.ReadFile(keybindingsPath)
	if err == nil {
		if err := yaml.Unmarshal(data, &keybindings); err != nil {
			keybindings = []map[string]interface{}{}
		}
	}

	// Check if binding already exists
	for _, binding := range keybindings {
		if key, ok := binding["key"].(string); ok && key == "shift-enter" {
			return &SetupResult{
				Success:    true,
				Message:    "Shift+Enter keybinding already configured",
				ConfigPath: keybindingsPath,
			}, nil
		}
	}

	// Add new binding
	keybindings = append(keybindings, newBinding)

	// Write back
	output, err := yaml.Marshal(keybindings)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(keybindingsPath, output, 0644); err != nil {
		return nil, err
	}

	return &SetupResult{
		Success:      true,
		Message:      "Warp keybinding configured successfully",
		NeedsRestart: true,
		ConfigPath:   keybindingsPath,
	}, nil
}

// SetupAlacritty configures Alacritty keybindings for Shift+Enter.
func SetupAlacritty() (*SetupResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	// Alacritty config locations
	possiblePaths := []string{
		filepath.Join(home, ".config", "alacritty", "alacritty.toml"),
		filepath.Join(home, ".alacritty.toml"),
	}

	var targetPath string
	for _, p := range possiblePaths {
		if _, err := os.Stat(p); err == nil {
			targetPath = p
			break
		}
	}

	// If no config exists, create in default location
	if targetPath == "" {
		targetPath = filepath.Join(home, ".config", "alacritty", "alacritty.toml")
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return nil, err
		}
	}

	// Read existing config
	data, err := os.ReadFile(targetPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	content := string(data)

	// Check if binding already exists
	if strings.Contains(content, "shift-enter") || strings.Contains(content, `key = "Return"`) && strings.Contains(content, `mods = "Shift"`) {
		return &SetupResult{
			Success:    true,
			Message:    "Shift+Enter keybinding already configured",
			ConfigPath: targetPath,
		}, nil
	}

	// Append keybinding section
	keybindingConfig := `
# Berth: Shift+Enter sends newline sequence
[[keyboard.bindings]]
key = "Return"
mods = "Shift"
chars = "\u001b\r"
`

	content += keybindingConfig

	if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
		return nil, err
	}

	return &SetupResult{
		Success:      true,
		Message:      "Alacritty keybinding configured successfully",
		NeedsRestart: true,
		ConfigPath:   targetPath,
	}, nil
}

// SetupTerminalApp provides instructions for Terminal.app (cannot be auto-configured).
func SetupTerminalApp() (*SetupResult, error) {
	return &SetupResult{
		Success: false,
		Message: "Terminal.app cannot be automatically configured.\n\nUse Option+Enter instead:\n1. Open Terminal > Settings > Profiles > Keyboard\n2. Check \"Use Option as Meta Key\"\n\nOr use the backslash method: type \\ then Enter for newlines.",
	}, nil
}

// removeJSONComments removes // and /* */ comments from JSON (VS Code allows this).
func removeJSONComments(input string) string {
	var result strings.Builder
	inString := false
	inLineComment := false
	inBlockComment := false

	for i := 0; i < len(input); i++ {
		if inLineComment {
			if input[i] == '\n' {
				inLineComment = false
				result.WriteByte('\n')
			}
			continue
		}

		if inBlockComment {
			if i+1 < len(input) && input[i] == '*' && input[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}

		if input[i] == '"' && (i == 0 || input[i-1] != '\\') {
			inString = !inString
		}

		if !inString {
			if i+1 < len(input) && input[i] == '/' && input[i+1] == '/' {
				inLineComment = true
				continue
			}
			if i+1 < len(input) && input[i] == '/' && input[i+1] == '*' {
				inBlockComment = true
				i++
				continue
			}
		}

		result.WriteByte(input[i])
	}

	return result.String()
}

// Setup runs the appropriate setup for the given terminal type.
func Setup(terminalType string) (*SetupResult, error) {
	switch terminalType {
	case "vscode":
		return SetupVSCode()
	case "warp":
		return SetupWarp()
	case "alacritty":
		return SetupAlacritty()
	case "apple":
		return SetupTerminalApp()
	default:
		return &SetupResult{
			Success: false,
			Message: fmt.Sprintf("No automatic setup available for %s.\n\nUse the backslash method: type \\ then Enter for newlines.", terminalType),
		}, nil
	}
}
