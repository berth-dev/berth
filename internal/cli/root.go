// Package cli defines Cobra command definitions for the berth CLI.
// This file contains the root command, version flag, and help output.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
}

// Execute runs the root command. Called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
