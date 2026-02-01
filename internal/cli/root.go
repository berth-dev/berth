// Package cli defines Cobra command definitions for the berth CLI.
// This file contains the root command, version flag, and help output.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/tui"
	"github.com/berth-dev/berth/internal/tui/app"
)

var (
	verbose bool
	debug   bool
	version = "dev" // set via ldflags at build time
)

var rootCmd = &cobra.Command{
	Use:   "berth",
	Short: "AI-powered development workflow orchestrator",
	Long: `Berth orchestrates multi-step development tasks using Claude.
It breaks work into beads (atomic tasks), executes them with fresh
Claude processes, and verifies each step before moving on.`,
	Version:       version,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// When no subcommand is provided, launch TUI if TTY, show help otherwise
		if !tui.IsTTY() {
			return cmd.Help()
		}

		// Get the current working directory as project root
		projectRoot, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting current directory: %w", err)
		}

		// Try to read config, use defaults if not initialized
		cfg, err := config.ReadConfig(projectRoot)
		if err != nil {
			// Config not found or invalid, use defaults
			cfg = config.DefaultConfig()
		}

		// Create and run the TUI app
		tuiApp := app.New(cfg, projectRoot)
		return tui.Run(tuiApp)
	},
}

// Execute runs the root command. Called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Verbose returns true if --verbose flag is set.
func Verbose() bool {
	return verbose
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Stream Claude output instead of progress bar")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Pass --mcp-debug to Claude processes for MCP troubleshooting")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(reportCmd)
	rootCmd.AddCommand(prCmd)
	rootCmd.AddCommand(resumeCmd)
	rootCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(bridgeCmd)
}
