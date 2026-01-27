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

// SpawnClaude invokes the Claude CLI as a subprocess with the given system
// and task prompts, waits for completion, and returns the parsed output.
// It enforces cfg.Execution.TimeoutPerBead as a hard timeout.
func SpawnClaude(cfg config.Config, systemPrompt, taskPrompt string, projectRoot string) (*ClaudeOutput, error) {
	timeout := time.Duration(cfg.Execution.TimeoutPerBead) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := buildClaudeArgs(cfg, systemPrompt, taskPrompt)

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = projectRoot

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
func buildClaudeArgs(cfg config.Config, systemPrompt, taskPrompt string) []string {
	args := []string{
		"-p", taskPrompt,
		"--append-system-prompt", systemPrompt,
		"--allowedTools", "Read,Write,Edit,Bash,Grep,Glob",
		"--output-format", "json",
		"--dangerously-skip-permissions",
		"--model", "opus",
	}

	if cfg.KnowledgeGraph.MCPDebug {
		args = append(args, "--mcp-debug")
	}

	return args
}
