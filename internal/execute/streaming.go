// streaming.go provides types for streaming execution output to the TUI.
package execute

import (
	"time"
)

// StreamEvent represents a streaming event from bead execution to the TUI.
// It extends OutputEvent with additional event types for TUI rendering.
type StreamEvent struct {
	Type     string // "output", "complete", "error", "token_update", "bead_init", "bead_complete", "group_start"
	BeadID   string
	Content  string
	Tokens   int
	IsStderr bool
}

// ChannelWriter implements io.Writer and sends output to a channel as StreamEvents.
// It is used to capture stdout/stderr from Claude subprocess execution.
type ChannelWriter struct {
	ch       chan<- StreamEvent
	beadID   string
	isStderr bool
}

// NewChannelWriter creates a new ChannelWriter that sends StreamEvents to the given channel.
// Each Write call produces an "output" event with the provided beadID.
func NewChannelWriter(ch chan<- StreamEvent, beadID string, isStderr bool) *ChannelWriter {
	return &ChannelWriter{
		ch:       ch,
		beadID:   beadID,
		isStderr: isStderr,
	}
}

// Write implements io.Writer. It sends the data as a StreamEvent to the channel.
// Uses a non-blocking send with a timeout to prevent deadlocks if the receiver is slow.
func (cw *ChannelWriter) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	event := StreamEvent{
		Type:     "output",
		BeadID:   cw.beadID,
		Content:  string(p),
		IsStderr: cw.isStderr,
	}

	// Non-blocking send with timeout to prevent deadlocks.
	select {
	case cw.ch <- event:
		// Successfully sent.
	case <-time.After(100 * time.Millisecond):
		// Channel full or slow receiver; drop the event to avoid blocking execution.
	}

	return len(p), nil
}
