// spawner.go manages spawning and lifecycle of Claude CLI processes.
package execute

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/berth-dev/berth/internal/config"
)

// SpawnClaudeOpts holds optional overrides for Claude subprocess invocation.
// Pass nil for default behavior (backward compatible).
type SpawnClaudeOpts struct {
	WorkDir       string // Override working directory (default: projectRoot)
	MCPConfigPath string // Path to MCP config JSON for coordinator bridge
	SystemPrompt  string // Override system prompt (default: prompts.ExecutorSystemPrompt)
}

// SpawnClaude invokes the Claude CLI as a subprocess with the given system
// and task prompts, waits for completion, and returns the parsed output.
// It enforces cfg.Execution.TimeoutPerBead as a hard timeout.
// Pass nil for opts to use default behavior.
func SpawnClaude(cfg config.Config, systemPrompt, taskPrompt string, projectRoot string, opts *SpawnClaudeOpts) (*ClaudeOutput, error) {
	timeout := time.Duration(cfg.Execution.TimeoutPerBead) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := buildClaudeArgs(cfg, systemPrompt, taskPrompt, opts)

	cmd := exec.CommandContext(ctx, "claude", args...)
	if opts != nil && opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	} else {
		cmd.Dir = projectRoot
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Check if the error was due to context timeout.
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("claude timed out after %s: %w", timeout, ctx.Err())
		}
		return nil, fmt.Errorf("claude exited with error: %w\nstderr: %s", err, stderr.String())
	}

	output, parseErr := ParseClaudeOutput(stdout.Bytes())
	if parseErr != nil {
		return nil, fmt.Errorf("parsing claude output: %w\nraw stdout: %s", parseErr, stdout.String())
	}

	return output, nil
}

// buildClaudeArgs constructs the CLI argument slice for a Claude invocation.
func buildClaudeArgs(cfg config.Config, systemPrompt, taskPrompt string, opts *SpawnClaudeOpts) []string {
	args := []string{
		"-p", taskPrompt,
		"--append-system-prompt", systemPrompt,
		"--allowedTools", "Read,Write,Edit,Bash,Grep,Glob",
		"--output-format", "json",
		"--dangerously-skip-permissions",
		"--model", "opus",
	}

	if opts != nil && opts.MCPConfigPath != "" {
		args = append(args, "--mcp-config", opts.MCPConfigPath)
	}

	if cfg.KnowledgeGraph.MCPDebug {
		args = append(args, "--mcp-debug")
	}

	return args
}
