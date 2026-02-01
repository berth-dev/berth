// start.go handles starting, stopping, and health-checking the MCP process
// including PID file management and graceful shutdown.
package graph

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/berth-dev/berth/internal/config"
)

const (
	pidFileName    = "mcp.pid"
	mcpLogFileName = "mcp.log"
	berthDir       = ".berth"
	stopTimeout    = 5 * time.Second
)

// StartMCP starts the Knowledge Graph MCP Node.js process and returns
// a connected Client.
func StartMCP(projectRoot string, cfg config.KGConfig) (*Client, error) {
	mcpCommand := cfg.MCPCommand
	if mcpCommand == "" {
		mcpCommand = "node --max-old-space-size=512 tools/code-graph/dist/index.js"
	}

	parts := strings.Fields(mcpCommand)
	if len(parts) == 0 {
		return nil, fmt.Errorf("graph: empty MCP command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = projectRoot

	// Open log file for stderr output.
	logDir := filepath.Join(projectRoot, berthDir)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("graph: creating .berth directory: %w", err)
	}

	logFile, err := os.OpenFile(
		filepath.Join(logDir, mcpLogFileName),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	if err != nil {
		return nil, fmt.Errorf("graph: opening MCP log file: %w", err)
	}
	cmd.Stderr = logFile

	timeout := time.Duration(cfg.ToolCallTimeout) * time.Millisecond
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	client, err := NewClient(cmd, timeout)
	if err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("graph: starting MCP client: %w", err)
	}

	// Write PID file.
	if err := writePIDFile(projectRoot, cmd.Process.Pid); err != nil {
		_ = client.Close()
		_ = logFile.Close()
		return nil, fmt.Errorf("graph: writing PID file: %w", err)
	}

	return client, nil
}

// StopMCP gracefully stops the MCP process identified by the PID file
// in the project root.
func StopMCP(projectRoot string) error {
	pid := readPIDFile(projectRoot)
	if pid <= 0 {
		return nil // No PID file or invalid PID; nothing to stop.
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		_ = removePIDFile(projectRoot)
		return nil
	}

	// Send interrupt signal for graceful shutdown.
	if err := proc.Signal(os.Interrupt); err != nil {
		// Process may already be dead.
		_ = removePIDFile(projectRoot)
		return nil
	}

	// Wait for the process to exit with a timeout.
	done := make(chan error, 1)
	go func() {
		_, err := proc.Wait()
		done <- err
	}()

	select {
	case <-done:
		// Process exited gracefully.
	case <-time.After(stopTimeout):
		// Force kill if still alive.
		_ = proc.Kill()
		<-done
	}

	_ = removePIDFile(projectRoot)
	return nil
}

// EnsureMCPAlive checks if the MCP process is alive. If dead, it restarts
// via StartMCP. If alive but client is nil, it cannot reattach to an existing
// process so it restarts.
func EnsureMCPAlive(projectRoot string, cfg config.KGConfig, client *Client) (*Client, error) {
	pid := readPIDFile(projectRoot)

	if pid > 0 && processAlive(pid) {
		// Process is alive.
		if client != nil {
			return client, nil
		}
		// Cannot reattach to an existing process's stdio; stop and restart.
		_ = StopMCP(projectRoot)
	}

	return StartMCP(projectRoot, cfg)
}

// writePIDFile writes the process ID to .berth/mcp.pid.
func writePIDFile(projectRoot string, pid int) error {
	path := filepath.Join(projectRoot, berthDir, pidFileName)
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0644)
}

// readPIDFile reads the process ID from .berth/mcp.pid.
// Returns -1 if the file does not exist or cannot be parsed.
func readPIDFile(projectRoot string) int {
	path := filepath.Join(projectRoot, berthDir, pidFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return -1
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return -1
	}

	return pid
}

// removePIDFile removes the .berth/mcp.pid file.
func removePIDFile(projectRoot string) error {
	path := filepath.Join(projectRoot, berthDir, pidFileName)
	return os.Remove(path)
}

// processAlive is implemented in start_unix.go and start_windows.go.
