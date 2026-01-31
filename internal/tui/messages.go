// Package tui implements the terminal user interface using Bubble Tea.
package tui

import (
	"github.com/berth-dev/berth/internal/session"
	"github.com/berth-dev/berth/internal/understand"
)

// ============================================================================
// State Transition Messages
// ============================================================================

// EnterChatMsg triggers a transition into chat mode for a specific question.
type EnterChatMsg struct {
	QuestionID string
}

// SkipInterviewMsg signals that the interview phase should be skipped.
type SkipInterviewMsg struct{}

// AnswerMsg contains the user's response to an interview question.
type AnswerMsg struct {
	QuestionID string
	Value      string
}

// ApproveMsg signals that the user approved the plan.
type ApproveMsg struct{}

// RejectMsg signals that the user rejected the plan with feedback.
type RejectMsg struct {
	Feedback string
}

// ============================================================================
// Execution Control Messages
// ============================================================================

// PauseMsg toggles or sets the paused state of execution.
type PauseMsg struct {
	Paused bool
}

// SkipBeadMsg requests skipping a specific bead during execution.
type SkipBeadMsg struct {
	BeadID string
}

// ChatAboutBeadMsg initiates a chat session about a specific bead.
type ChatAboutBeadMsg struct {
	BeadID string
}

// BeadStartMsg signals that a bead execution has started.
type BeadStartMsg struct {
	Index int
}

// BeadCompleteMsg signals that a bead execution has completed.
type BeadCompleteMsg struct {
	Index  int
	Passed bool
}

// ============================================================================
// Session Messages
// ============================================================================

// SessionLoadedMsg signals that a session has been loaded from storage.
type SessionLoadedMsg struct {
	Session *session.Session
}

// SessionSavedMsg signals that the session has been saved to storage.
type SessionSavedMsg struct{}

// SessionErrorMsg signals an error during session operations.
type SessionErrorMsg struct {
	Err error
}

// ============================================================================
// Claude Spawn Messages
// ============================================================================

// ClaudeStartMsg signals that a Claude process has been spawned for a bead.
type ClaudeStartMsg struct {
	BeadID string
}

// ClaudeOutputMsg contains streaming output from a Claude process.
type ClaudeOutputMsg struct {
	BeadID   string
	Content  string
	IsStderr bool
}

// ClaudeCompleteMsg signals that a Claude process has completed.
type ClaudeCompleteMsg struct {
	BeadID     string
	Result     string
	CostUSD    float64
	DurationMS int64
}

// ClaudeErrorMsg signals an error from a Claude process.
type ClaudeErrorMsg struct {
	BeadID string
	Err    error
}

// ============================================================================
// Analysis Messages
// ============================================================================

// AnalysisStartMsg signals that project analysis has started.
type AnalysisStartMsg struct{}

// AnalysisCompleteMsg signals that project analysis has completed.
type AnalysisCompleteMsg struct {
	IsSurgical bool
	Questions  []Question
}

// RequirementsCompleteMsg signals that requirements gathering has completed.
type RequirementsCompleteMsg struct {
	Title   string
	Content string
}

// ============================================================================
// Interview Messages
// ============================================================================

// InterviewStartedMsg signals that an interview session has been initialized.
type InterviewStartedMsg struct {
	Session *understand.InterviewSession
}

// InterviewQuestionsMsg provides questions for the current round.
type InterviewQuestionsMsg struct {
	Questions []Question // uses existing tui.Question type
	Round     int
}

// InterviewCompleteMsg signals that the interview is done with requirements.
type InterviewCompleteMsg struct {
	Requirements *understand.Requirements
}

// InterviewErrorMsg signals an interview error.
type InterviewErrorMsg struct {
	Err error
}

// ============================================================================
// Plan Messages
// ============================================================================

// PlanGeneratedMsg signals that a plan has been generated.
type PlanGeneratedMsg struct {
	Plan   *Plan
	Groups []ExecutionGroup
}

// PlanErrorMsg signals an error during plan generation.
type PlanErrorMsg struct {
	Err error
}

// ============================================================================
// Utility Messages
// ============================================================================

// TickMsg is sent periodically for time-based updates (spinners, timers).
type TickMsg struct{}

// ErrorMsg is a generic error message for unrecoverable errors.
type ErrorMsg struct {
	Err error
}

// WindowSizeMsg signals that the terminal window has been resized.
type WindowSizeMsg struct {
	Width  int
	Height int
}
