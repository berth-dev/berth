// Package ui provides terminal UI components for berth.
// This file implements the progress display shown during bead execution.
package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// BeadStatus represents the execution status of a single bead.
type BeadStatus int

const (
	StatusPending    BeadStatus = iota // Waiting or blocked
	StatusExecuting                    // Currently running
	StatusCompleted                    // Finished successfully
	StatusFailed                       // Failed / stuck after retries
	StatusSkipped                      // Skipped by user
)

// BeadState holds the display state of a single bead.
type BeadState struct {
	ID        string
	Title     string
	Status    BeadStatus
	Attempt   int
	Elapsed   time.Duration
	BlockedBy []string // IDs of beads blocking this one
}

// ProgressDisplay manages a live-updating terminal progress view.
type ProgressDisplay struct {
	mu          sync.Mutex
	taskDesc    string
	beads       []*BeadState
	beadIndex   map[string]int        // ID -> index in beads slice
	started     bool
	isTTY       bool
	linesDrawn  int
	startTimes  map[string]time.Time
	lastPrinted map[string]BeadStatus // tracks last printed status per bead (non-TTY)
}

// NewProgressDisplay creates a ProgressDisplay for the given task description.
func NewProgressDisplay(taskDesc string) *ProgressDisplay {
	return &ProgressDisplay{
		taskDesc:    taskDesc,
		beadIndex:   make(map[string]int),
		startTimes:  make(map[string]time.Time),
		lastPrinted: make(map[string]BeadStatus),
		isTTY:       term.IsTerminal(int(os.Stdout.Fd())),
	}
}

// AddBead registers a bead for progress tracking.
func (p *ProgressDisplay) AddBead(id, title string, blockedBy []string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	state := &BeadState{
		ID:        id,
		Title:     title,
		Status:    StatusPending,
		BlockedBy: blockedBy,
	}
	p.beadIndex[id] = len(p.beads)
	p.beads = append(p.beads, state)
}

// Start draws the initial progress display.
func (p *ProgressDisplay) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.started = true
	p.render()
}

// UpdateBead updates a bead's status and re-renders the display.
func (p *ProgressDisplay) UpdateBead(id string, status BeadStatus, attempt int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	idx, ok := p.beadIndex[id]
	if !ok {
		return
	}

	bead := p.beads[idx]
	bead.Status = status
	bead.Attempt = attempt

	switch status {
	case StatusExecuting:
		p.startTimes[id] = time.Now()
	case StatusCompleted, StatusFailed, StatusSkipped:
		if start, ok := p.startTimes[id]; ok {
			bead.Elapsed = time.Since(start)
		}
	}

	if p.started {
		p.render()
	}
}

// Finish finalizes the display by moving the cursor below all output
// and printing a summary line.
func (p *ProgressDisplay) Finish() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isTTY && p.linesDrawn > 0 {
		// Move cursor to end
		fmt.Print("\n")
	}

	completed := 0
	failed := 0
	skipped := 0
	for _, b := range p.beads {
		switch b.Status {
		case StatusCompleted:
			completed++
		case StatusFailed:
			failed++
		case StatusSkipped:
			skipped++
		}
	}

	total := len(p.beads)
	fmt.Printf("\nDone: %d/%d completed", completed, total)
	if failed > 0 {
		fmt.Printf(", %d failed", failed)
	}
	if skipped > 0 {
		fmt.Printf(", %d skipped", skipped)
	}
	fmt.Println()
}

// render draws or redraws the progress display.
func (p *ProgressDisplay) render() {
	if !p.isTTY {
		p.renderPlain()
		return
	}
	p.renderTTY()
}

// renderTTY draws the progress display using ANSI escape codes for in-place updates.
func (p *ProgressDisplay) renderTTY() {
	// Move cursor up to overwrite previous output.
	if p.linesDrawn > 0 {
		fmt.Printf("\033[%dA", p.linesDrawn)
	}

	var buf strings.Builder

	// Header line.
	buf.WriteString(fmt.Sprintf("\033[2K\033[1m\u2693 Berth - %q\033[0m\n", p.taskDesc))
	buf.WriteString("\033[2K\n")

	// Bead lines.
	for _, bead := range p.beads {
		buf.WriteString("\033[2K")
		buf.WriteString(formatBeadLine(bead, p.startTimes))
		buf.WriteString("\n")
	}

	fmt.Print(buf.String())
	p.linesDrawn = len(p.beads) + 2 // header + blank + beads
}

// renderPlain writes non-TTY output (for CI/piping).
// Only prints on status transitions to avoid duplicate lines.
func (p *ProgressDisplay) renderPlain() {
	for _, bead := range p.beads {
		if bead.Status == StatusPending {
			continue
		}
		if prev, seen := p.lastPrinted[bead.ID]; seen && prev == bead.Status {
			continue
		}
		fmt.Println(formatBeadLinePlain(bead))
		p.lastPrinted[bead.ID] = bead.Status
	}
}

// formatBeadLine formats a single bead line with ANSI colors and status icons.
func formatBeadLine(bead *BeadState, startTimes map[string]time.Time) string {
	icon := statusIcon(bead.Status)
	detail := statusDetail(bead, startTimes)

	title := bead.Title
	if len(title) > 45 {
		title = title[:42] + "..."
	}

	return fmt.Sprintf("  %s %s %s  %s", icon, bead.ID, title, detail)
}

// formatBeadLinePlain formats a bead line for non-TTY output.
func formatBeadLinePlain(bead *BeadState) string {
	var status string
	switch bead.Status {
	case StatusPending:
		status = "PENDING"
	case StatusExecuting:
		status = fmt.Sprintf("RUNNING (attempt %d)", bead.Attempt)
	case StatusCompleted:
		status = fmt.Sprintf("DONE [%s]", formatDuration(bead.Elapsed))
	case StatusFailed:
		status = "FAILED"
	case StatusSkipped:
		status = "SKIPPED"
	}
	return fmt.Sprintf("[%s] %s: %s - %s", status, bead.ID, bead.Title, status)
}

// statusIcon returns the status icon for a bead.
func statusIcon(status BeadStatus) string {
	switch status {
	case StatusCompleted:
		return "\033[32m\u2705\033[0m" // green checkmark
	case StatusExecuting:
		return "\033[33m\u23f3\033[0m" // yellow hourglass
	case StatusFailed:
		return "\033[31m\u274c\033[0m" // red X
	case StatusSkipped:
		return "\033[90m\u23ed\033[0m" // dim skip
	default:
		return "\033[90m\u25cb\033[0m" // dim circle
	}
}

// statusDetail returns the right-side detail text for a bead.
func statusDetail(bead *BeadState, startTimes map[string]time.Time) string {
	switch bead.Status {
	case StatusCompleted:
		return fmt.Sprintf("\033[90m[%s]\033[0m", formatDuration(bead.Elapsed))
	case StatusExecuting:
		elapsed := time.Since(startTimes[bead.ID])
		return fmt.Sprintf("\033[33m[attempt %d, %s]\033[0m", bead.Attempt, formatDuration(elapsed))
	case StatusFailed:
		return "\033[31m[stuck]\033[0m"
	case StatusSkipped:
		return "\033[90m[skipped]\033[0m"
	default:
		if len(bead.BlockedBy) > 0 {
			return fmt.Sprintf("\033[90m[blocked by %s]\033[0m", strings.Join(bead.BlockedBy, ", "))
		}
		return "\033[90m[pending]\033[0m"
	}
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dh%dm%ds", h, m, s)
}
