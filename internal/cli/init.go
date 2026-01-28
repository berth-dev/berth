// init.go implements the "berth init" command with optional --guided flag.
package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/berth-dev/berth/internal/beads"
	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/context"
	"github.com/berth-dev/berth/internal/detect"
	"github.com/berth-dev/berth/internal/git"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize berth in the current project",
	Long: `Initialize the .berth/ directory with configuration, CLAUDE.md,
and the beads task system. Auto-detects project stack for brownfield
projects or creates minimal defaults for greenfield.`,
	RunE: runInit,
}

var guidedFlag bool

func init() {
	initCmd.Flags().BoolVar(&guidedFlag, "guided", false, "Interactive prompts for configuration overrides")
}

func runInit(cmd *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Check for existing .berth/ directory.
	berthDir := filepath.Join(dir, ".berth")
	if info, statErr := os.Stat(berthDir); statErr == nil && info.IsDir() {
		fmt.Println("Warning: .berth/ directory already exists.")
		fmt.Print("Reinitialize? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Create .berth/ directory structure.
	for _, subdir := range []string{".berth", ".berth/runs"} {
		if mkErr := os.MkdirAll(filepath.Join(dir, subdir), 0755); mkErr != nil {
			return fmt.Errorf("creating directory %s: %w", subdir, mkErr)
		}
	}

	// Ensure .gitignore exists with sensible defaults.
	if err := ensureGitignore(dir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to set up .gitignore: %v\n", err)
	}

	// Ensure a git repo exists before beads init (bd --stealth needs .git/).
	if err := git.EnsureRepo(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to ensure git repo: %v\n", err)
	}
	if err := git.EnsureInitialCommit(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create initial commit: %v\n", err)
	}

	// Detect brownfield vs greenfield.
	brownfield := detect.HasExistingCode(dir)

	cfg := config.DefaultConfig()

	if brownfield {
		stackInfo := detect.DetectStack(dir)

		// Populate config from detected stack.
		cfg.Project.Name = filepath.Base(dir)
		cfg.Project.Language = stackInfo.Language
		cfg.Project.Framework = stackInfo.Framework
		cfg.Project.PackageManager = stackInfo.PackageManager

		// Build verify pipeline from detected commands.
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

		// Guided mode: allow overrides.
		if guidedFlag {
			cfg, err = guidedOverrides(cfg, stackInfo)
			if err != nil {
				return fmt.Errorf("guided setup: %w", err)
			}
		}

		// Write config.
		if writeErr := config.WriteConfig(dir, cfg); writeErr != nil {
			return fmt.Errorf("writing config: %w", writeErr)
		}

		// Generate and write CLAUDE.md.
		learnings := context.ReadLearnings(dir)
		claudeContent := context.GenerateCLAUDEMD(*cfg, stackInfo, learnings, nil)
		if writeErr := context.WriteCLAUDEMD(dir, claudeContent); writeErr != nil {
			return fmt.Errorf("writing CLAUDE.md: %w", writeErr)
		}

		// Initialize beads system (skip if already initialized).
		if beadErr := beads.Init(); beadErr != nil {
			if errors.Is(beadErr, beads.ErrBDNotInstalled) {
				fmt.Println()
				fmt.Println("Error: bd CLI not found.")
				fmt.Println("Install beads first: npm install -g beads")
				return beadErr
			}
			errMsg := beadErr.Error()
			if !strings.Contains(errMsg, "already initialized") && !strings.Contains(errMsg, "existing database") {
				return fmt.Errorf("initializing beads: %w", beadErr)
			}
		}
		cleanBeadsArtifacts(dir)

		// Print detection results.
		fmt.Println()
		fmt.Println("Berth initialized (brownfield project detected)")
		fmt.Printf("  Language:        %s\n", stackInfo.Language)
		fmt.Printf("  Framework:       %s\n", stackInfo.Framework)
		fmt.Printf("  Package Manager: %s\n", stackInfo.PackageManager)
		if stackInfo.TestCmd != "" {
			fmt.Printf("  Test Command:    %s\n", stackInfo.TestCmd)
		}
		if stackInfo.BuildCmd != "" {
			fmt.Printf("  Build Command:   %s\n", stackInfo.BuildCmd)
		}
		if stackInfo.LintCmd != "" {
			fmt.Printf("  Lint Command:    %s\n", stackInfo.LintCmd)
		}
		fmt.Println()
		fmt.Println("Configuration written to .berth/config.yaml")
		fmt.Println("Ready to run: berth run \"your task description\"")
	} else {
		// Greenfield: minimal setup.
		cfg.Project.Name = filepath.Base(dir)

		if guidedFlag {
			cfg, err = guidedOverrides(cfg, detect.StackInfo{})
			if err != nil {
				return fmt.Errorf("guided setup: %w", err)
			}
		}

		if writeErr := config.WriteConfig(dir, cfg); writeErr != nil {
			return fmt.Errorf("writing config: %w", writeErr)
		}

		// Initialize beads system (skip if already initialized).
		if beadErr := beads.Init(); beadErr != nil {
			if errors.Is(beadErr, beads.ErrBDNotInstalled) {
				fmt.Println()
				fmt.Println("Error: bd CLI not found.")
				fmt.Println("Install beads first: npm install -g beads")
				return beadErr
			}
			// Ignore "already initialized" errors from bd init.
			errMsg := beadErr.Error()
			if !strings.Contains(errMsg, "already initialized") && !strings.Contains(errMsg, "existing database") {
				return fmt.Errorf("initializing beads: %w", beadErr)
			}
		}
		cleanBeadsArtifacts(dir)

		fmt.Println()
		fmt.Println("Berth initialized (greenfield project)")
		fmt.Println("Configuration written to .berth/config.yaml")
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  1. Edit .berth/config.yaml to set project language/framework")
		fmt.Println("  2. Run: berth run \"your task description\"")
	}

	// Auto-commit berth init files so the user starts with a clean git status.
	// Creates an initial empty commit first if the repo has no commits.
	if commitErr := commitBerthInit(dir); commitErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to auto-commit init files: %v\n", commitErr)
	}

	return nil
}

// commitBerthInit stages .gitignore and .berth/config.yaml and commits them.
// If the repo has no commits yet, creates an initial empty commit first.
// Silently skips if files are gitignored or if there are no changes to commit.
func commitBerthInit(dir string) error {
	// Ensure at least one commit exists so we can stage files.
	if err := git.EnsureInitialCommit(); err != nil {
		return err
	}

	// Filter out gitignored files.
	candidates := []string{".gitignore", ".berth/config.yaml"}
	var files []string
	for _, f := range candidates {
		if !git.IsIgnored(f) {
			files = append(files, f)
		}
	}

	// Nothing to commit if all files are ignored.
	if len(files) == 0 {
		return nil
	}

	// Try to commit; ignore "nothing to commit" errors silently.
	if err := git.CommitFiles(files, "chore: initialize berth"); err != nil {
		// Check if error is just "nothing to commit" - this is fine.
		errStr := err.Error()
		if strings.Contains(errStr, "nothing to commit") ||
			strings.Contains(errStr, "no changes added to commit") {
			return nil
		}
		return err
	}

	return nil
}

// guidedOverrides prompts the user for optional configuration overrides.
func guidedOverrides(cfg *config.Config, stackInfo detect.StackInfo) (*config.Config, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println()
	fmt.Println("--- Guided Configuration ---")

	// Project name.
	fmt.Printf("Project name [%s]: ", cfg.Project.Name)
	if name, err := reader.ReadString('\n'); err == nil {
		name = strings.TrimSpace(name)
		if name != "" {
			cfg.Project.Name = name
		}
	}

	// Language.
	defaultLang := cfg.Project.Language
	if defaultLang == "" {
		defaultLang = "auto-detect"
	}
	fmt.Printf("Language [%s]: ", defaultLang)
	if lang, err := reader.ReadString('\n'); err == nil {
		lang = strings.TrimSpace(lang)
		if lang != "" {
			cfg.Project.Language = lang
		}
	}

	// Framework.
	defaultFW := cfg.Project.Framework
	if defaultFW == "" {
		defaultFW = "none"
	}
	fmt.Printf("Framework [%s]: ", defaultFW)
	if fw, err := reader.ReadString('\n'); err == nil {
		fw = strings.TrimSpace(fw)
		if fw != "" {
			cfg.Project.Framework = fw
		}
	}

	// Knowledge Graph enabled.
	fmt.Printf("Knowledge Graph [%s]: ", cfg.KnowledgeGraph.Enabled)
	if kg, err := reader.ReadString('\n'); err == nil {
		kg = strings.TrimSpace(kg)
		if kg != "" {
			cfg.KnowledgeGraph.Enabled = kg
		}
	}

	fmt.Println("--- End Guided Configuration ---")
	fmt.Println()

	return cfg, nil
}

// cleanBeadsArtifacts removes files that bd init may create in the working tree
// that we don't want visible to the user (AGENTS.md, .gitattributes from beads).
func cleanBeadsArtifacts(dir string) {
	for _, name := range []string{"AGENTS.md", ".claude"} {
		path := filepath.Join(dir, name)
		// Only remove if it looks like a beads artifact.
		if info, err := os.Stat(path); err == nil {
			if name == ".claude" && info.IsDir() {
				os.RemoveAll(path)
			} else if !info.IsDir() {
				os.Remove(path)
			}
		}
	}
}

// ensureGitignore creates or appends to .gitignore with common entries
// that should never be committed. It's smart: it reads the existing file
// and only adds entries that aren't already present.
func ensureGitignore(dir string) error {
	gitignorePath := filepath.Join(dir, ".gitignore")

	// Entries that should always be gitignored.
	requiredEntries := []string{
		// Common build artifacts
		"node_modules/",
		"dist/",
		"build/",
		// Secrets
		".env",
		".env.*",
		// OS files
		".DS_Store",
		"Thumbs.db",
		// Berth runtime (config.yaml and learnings.md ARE committed)
		".berth/log.jsonl",
		".berth/mcp.pid",
		".berth/mcp.log",
		".berth/runs/",
		// Beads runtime (stealth mode handles this via .git/info/exclude,
		// but belt-and-suspenders in case user runs bd init manually)
		".beads/",
	}

	// Read existing content.
	existing := ""
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existing = string(data)
	}

	// Find entries that are missing.
	var missing []string
	for _, entry := range requiredEntries {
		if !strings.Contains(existing, entry) {
			missing = append(missing, entry)
		}
	}

	if len(missing) == 0 {
		return nil
	}

	// Build the content to append.
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

	// Write.
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening .gitignore: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(toAppend.String()); err != nil {
		return fmt.Errorf("writing .gitignore: %w", err)
	}

	return nil
}
