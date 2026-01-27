//go:build windows

package graph

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// processAlive checks whether a process with the given PID is still running.
// On Windows, os.FindProcess always succeeds, so we check via tasklist.
func processAlive(pid int) bool {
	cmd := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/NH")
	out, err := cmd.Output()
	if err != nil {
		// If tasklist fails, fall back to os.FindProcess + Signal check.
		proc, findErr := os.FindProcess(pid)
		if findErr != nil {
			return false
		}
		return proc.Signal(os.Interrupt) == nil
	}
	return strings.Contains(string(out), strconv.Itoa(pid))
}
