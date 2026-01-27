//go:build !windows

package graph

import "syscall"

// processAlive checks whether a process with the given PID is still running
// by sending signal 0 (Unix-specific).
func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
