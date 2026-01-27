// Package log provides structured event logging.
// This file appends JSON events to log.jsonl.
package log

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event type constants.
const (
	EventRunStarted         = "run_started"
	EventUnderstandComplete = "understand_complete"
	EventPlanApproved       = "plan_approved"
	EventTaskStarted        = "task_started"
	EventVerifyPassed       = "verify_passed"
	EventVerifyFailed       = "verify_failed"
	EventTaskRetry          = "task_retry"
	EventTaskCompleted      = "task_completed"
	EventRunComplete        = "run_complete"
)

// LogEvent represents a single structured event written to the log.
type LogEvent struct {
	Time         time.Time              `json:"time"`
	Event        string                 `json:"event"`
	BeadID       string                 `json:"bead,omitempty"`
	Title        string                 `json:"title,omitempty"`
	Description  string                 `json:"description,omitempty"`
	Branch       string                 `json:"branch,omitempty"`
	Beads        int                    `json:"beads,omitempty"`
	Commits      []string               `json:"commits,omitempty"`
	Reason       string                 `json:"reason,omitempty"`
	Step         string                 `json:"step,omitempty"`
	Error        string                 `json:"error,omitempty"`
	Attempt      int                    `json:"attempt,omitempty"`
	Completed    int                    `json:"completed,omitempty"`
	Stuck        int                    `json:"stuck,omitempty"`
	Total        int                    `json:"total,omitempty"`
	Requirements string                 `json:"requirements,omitempty"`
	DurationMs   int64                  `json:"duration_ms,omitempty"`
	CostUSD      float64                `json:"cost_usd,omitempty"`
	Data         map[string]interface{} `json:"data,omitempty"`
}

// Logger writes append-only JSONL events to a log file.
type Logger struct {
	path string
	mu   sync.Mutex
}

// NewLogger creates a Logger that writes to .berth/log.jsonl inside dir.
// Creates the .berth/ directory if it does not already exist.
// Does not truncate an existing log file.
func NewLogger(dir string) (*Logger, error) {
	berthDir := filepath.Join(dir, ".berth")
	if err := os.MkdirAll(berthDir, 0755); err != nil {
		return nil, fmt.Errorf("create .berth directory: %w", err)
	}

	return &Logger{
		path: filepath.Join(berthDir, "log.jsonl"),
	}, nil
}

// Append writes a single LogEvent as one JSON line to the log file.
// If event.Time is the zero value, it is automatically set to time.Now().UTC().
// The file is opened in append mode, written to, and then closed.
// Thread-safe via mutex.
func (l *Logger) Append(event LogEvent) error {
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal log event: %w", err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer f.Close()

	// Write the JSON line followed by a newline.
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write log event: %w", err)
	}

	return nil
}

// ReadAll reads and parses all events from the log file.
// Returns an empty slice (not an error) if the file does not exist.
func (l *Logger) ReadAll() ([]LogEvent, error) {
	f, err := os.Open(l.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []LogEvent{}, nil
		}
		return nil, fmt.Errorf("open log file: %w", err)
	}
	defer f.Close()

	var events []LogEvent
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var event LogEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf("parse log line %d: %w", lineNum, err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read log file: %w", err)
	}

	return events, nil
}
