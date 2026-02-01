// Package session provides SQLite-backed persistence for Berth sessions.
package session

import "time"

// Session represents a Berth execution session.
type Session struct {
	ID        string
	Project   string
	Task      string
	Status    string // active, paused, completed
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Message represents a chat message within a session.
type Message struct {
	ID        int
	SessionID string
	Role      string // user, assistant
	Content   string
	Timestamp time.Time
}

// Answer represents a user's answer to an interview question.
type Answer struct {
	ID         int
	SessionID  string
	QuestionID string
	Answer     string
	Timestamp  time.Time
}

// BeadState represents the execution state of a bead within a session.
type BeadState struct {
	ID         int
	SessionID  string
	BeadID     string
	Status     string
	Output     string
	Tokens     int
	DurationMs int64
	UpdatedAt  time.Time
}

// Summary provides a high-level view of a session for listing.
type Summary struct {
	ID             string
	Task           string
	Status         string
	BeadsCompleted int
	BeadsTotal     int
	UpdatedAt      time.Time
}
