// Package commands provides Bubble Tea commands for TUI operations.
package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/context"
	"github.com/berth-dev/berth/internal/detect"
	"github.com/berth-dev/berth/internal/git"
	"github.com/berth-dev/berth/internal/tui"
)

// CheckInitCmd checks if the project needs initialization.
// Returns InitCheckMsg with NeedsInit=true if .berth/ doesn't exist.
func CheckInitCmd(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		berthDir := filepath.Join(projectRoot, ".berth")
		info, err := os.Stat(berthDir)
		needsInit := err != nil || !info.IsDir()
		return tui.InitCheckMsg{NeedsInit: needsInit}
	}
}

// RunInitCmd performs project initialization.
// This mirrors the logic from cli/init.go but adapted for TUI use.
// Returns InitCompleteMsg on success with detected stack info, or InitErrorMsg on failure.
func RunInitCmd(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		// Create .berth/ directory structure
		for _, subdir := range []string{".berth", ".berth/runs"} {
			if err := os.MkdirAll(filepath.Join(projectRoot, subdir), 0755); err != nil {
				return tui.InitErrorMsg{Err: fmt.Errorf("creating directory %s: %w", subdir, err)}
			}
		}

		// Ensure .gitignore exists with sensible defaults
		if err := ensureGitignore(projectRoot); err != nil {
			// Non-fatal, just log
			fmt.Fprintf(os.Stderr, "Warning: failed to set up .gitignore: %v\n", err)
		}

		// Ensure git repo exists
		if err := git.EnsureRepo(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to ensure git repo: %v\n", err)
		}
		if err := git.EnsureInitialCommit(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create initial commit: %v\n", err)
		}

		// Detect brownfield vs greenfield
		brownfield := detect.HasExistingCode(projectRoot)
		cfg := config.DefaultConfig()

		var stackInfo detect.StackInfo
		if brownfield {
			stackInfo = detect.DetectStack(projectRoot)

			// Populate config from detected stack
			cfg.Project.Name = filepath.Base(projectRoot)
			cfg.Project.Language = stackInfo.Language
			cfg.Project.Framework = stackInfo.Framework
			cfg.Project.PackageManager = stackInfo.PackageManager

			// Build verify pipeline from detected commands
			var pipeline []string
			if stackInfo.BuildCmd != "" {
				pipeline = append(pipeline, stackInfo.BuildCmd)
			}
			if stackInfo.LintCmd != "" {
				pipeline = append(pipeline, stackInfo.LintCmd)
			}
			if stackInfo.TestCmd != "" {
				pipeline = append(pipeline, stackInfo.TestCmd)
			}
			cfg.VerifyPipeline = pipeline

			// Write config
			if err := config.WriteConfig(projectRoot, cfg); err != nil {
				return tui.InitErrorMsg{Err: fmt.Errorf("writing config: %w", err)}
			}

			// Generate and write CLAUDE.md
			learnings := context.ReadLearnings(projectRoot)
			claudeContent := context.GenerateCLAUDEMD(*cfg, stackInfo, learnings, nil)
			if err := context.WriteCLAUDEMD(projectRoot, claudeContent); err != nil {
				return tui.InitErrorMsg{Err: fmt.Errorf("writing CLAUDE.md: %w", err)}
			}
		} else {
			// Greenfield: minimal setup
			cfg.Project.Name = filepath.Base(projectRoot)
			stackInfo = detect.StackInfo{} // Empty for greenfield

			if err := config.WriteConfig(projectRoot, cfg); err != nil {
				return tui.InitErrorMsg{Err: fmt.Errorf("writing config: %w", err)}
			}
		}

		// Initialize beads system
		if err := beads.Init(); err != nil {
			if errors.Is(err, beads.ErrBDNotInstalled) {
				return tui.InitErrorMsg{Err: fmt.Errorf("bd CLI not found - install beads first: npm install -g beads")}
			}
			errMsg := err.Error()
			if !strings.Contains(errMsg, "already initialized") && !strings.Contains(errMsg, "existing database") {
				return tui.InitErrorMsg{Err: fmt.Errorf("initializing beads: %w", err)}
			}
		}

		// Clean beads artifacts
		cleanBeadsArtifacts(projectRoot)

		// Auto-commit init files
		if err := commitBerthInit(projectRoot); err != nil {
			// Non-fatal
			fmt.Fprintf(os.Stderr, "Warning: failed to auto-commit init files: %v\n", err)
		}

		return tui.InitCompleteMsg{StackInfo: stackInfo}
	}
}

// ensureGitignore creates or appends to .gitignore with common entries.
func ensureGitignore(dir string) error {
	gitignorePath := filepath.Join(dir, ".gitignore")

	requiredEntries := []string{
		"node_modules/",
		"dist/",
		"build/",
		".env",
		".env.*",
		".DS_Store",
		"Thumbs.db",
		".berth/log.jsonl",
		".berth/mcp.pid",
		".berth/mcp.log",
		".berth/runs/",
		".beads/",
	}

	existing := ""
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existing = string(data)
	}

	var missing []string
	for _, entry := range requiredEntries {
		if !strings.Contains(existing, entry) {
			missing = append(missing, entry)
		}
	}

	if len(missing) == 0 {
		return nil
	}

	var toAppend strings.Builder
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		toAppend.WriteString("\n")
	}
	if existing != "" {
		toAppend.WriteString("\n# Added by berth init\n")
	}
	for _, entry := range missing {
		toAppend.WriteString(entry + "\n")
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening .gitignore: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.WriteString(toAppend.String()); err != nil {
		return fmt.Errorf("writing .gitignore: %w", err)
	}

	return nil
}

// cleanBeadsArtifacts removes files that bd init may create.
func cleanBeadsArtifacts(dir string) {
	for _, name := range []string{"AGENTS.md", ".claude"} {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil {
			if name == ".claude" && info.IsDir() {
				_ = os.RemoveAll(path)
			} else if !info.IsDir() {
				_ = os.Remove(path)
			}
		}
	}
}

// commitBerthInit stages and commits init files.
func commitBerthInit(dir string) error {
	if err := git.EnsureInitialCommit(); err != nil {
		return err
	}

	candidates := []string{".gitignore", ".berth/config.yaml"}
	var files []string
	for _, f := range candidates {
		if !git.IsIgnored(f) {
			files = append(files, f)
		}
	}

	if len(files) == 0 {
		return nil
	}

	if err := git.CommitFiles(files, "chore: initialize berth"); err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "nothing to commit") ||
			strings.Contains(errStr, "no changes added to commit") {
			return nil
		}
		return err
	}

	return nil
}
