// Package tui implements the terminal user interface using Bubble Tea.
package tui

import (
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"

	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/detect"
	"github.com/berth-dev/berth/internal/execute"
	"github.com/berth-dev/berth/internal/graph"
	"github.com/berth-dev/berth/internal/understand"
)

// ViewState represents the current state of the TUI.
type ViewState int

const (
	StateInit ViewState = iota // Project needs initialization
	StateHome
	StateAnalyzing
	StateInterview
	StateChat
	StateApproval
	StateExecuting
	StateComplete
	StateDashboard
)

// Tab represents the active tab in the TUI.
type Tab int

const (
	TabChat Tab = iota
	TabDashboard
	TabSessions
)

// ChatMessage represents a single message in the chat history.
type ChatMessage struct {
	Role    string // "user", "assistant", "system"
	Content string
}

// Option represents a selectable option for a question.
type Option struct {
	Key         string
	Label       string
	Recommended bool
}

// Question represents an interview question.
type Question struct {
	ID          string
	Text        string
	Options     []Option
	AllowCustom bool
	AllowHelp   bool
}

// Answer represents a user's response to a question.
type Answer struct {
	ID    string
	Value string
}

// BeadState represents the current state of a bead during execution.
type BeadState struct {
	ID         string
	Title      string
	Status     string // "pending", "running", "success", "failed", "blocked"
	TokenCount int
	Duration   time.Duration
	Attempt    int
	BlockedBy  []string
}

// ExecutionGroup represents a group of beads that can be executed together.
type ExecutionGroup struct {
	Index    int
	BeadIDs  []string
	Parallel bool
}

// BeadSpec represents a bead specification from the plan.
type BeadSpec struct {
	ID          string
	Title       string
	Description string
	Files       []string
	DependsOn   []string
	VerifyExtra []string
}

// Plan represents the execution plan generated during planning phase.
type Plan struct {
	Title       string
	Description string
	Beads       []BeadSpec
	RawOutput   string
}

// OutputEvent represents an event from bead execution output.
type OutputEvent struct {
	Type     string // "stdout", "stderr", "token", "status"
	BeadID   string
	Content  string
	Tokens   int
	IsStderr bool
}

// SessionInfo represents a saved session for the sessions view.
type SessionInfo struct {
	ID        string
	Name      string
	CreatedAt time.Time
	Status    string
	BeadCount int
}

// Model is the main TUI model that holds all application state.
type Model struct {
	// State management
	State              ViewState
	ActiveTab          Tab
	Err                error
	AnalyzingStartTime time.Time

	// Configuration
	Cfg         *config.Config
	ProjectRoot string
	RunDir      string

	// Session management (interfaces to be defined later)
	Session interface{}
	Store   interface{}

	// Stack detection
	StackInfo detect.StackInfo

	// Knowledge Graph
	KGClient     *graph.Client
	GraphSummary string

	// Project flags
	IsGreenfield bool

	// Interview state
	ChatHistory      []ChatMessage
	Questions        []Question
	Answers          []Answer
	CurrentQ         int
	InChatMode       bool
	InterviewSession *understand.InterviewSession
	Requirements     *understand.Requirements

	// Plan state
	Plan   *Plan
	Groups []ExecutionGroup

	// Execution state
	Beads       []BeadState
	CurrentBead int
	BeadOutput  []string
	IsPaused    bool
	TokenCount  int
	ElapsedTime time.Duration

	// Dashboard state
	Diagram   string
	Learnings []string
	Sessions  []SessionInfo

	// Bubbles components
	List      list.Model
	TextInput textinput.Model
	Textarea  textarea.Model
	Viewport  viewport.Model
	Spinner   spinner.Model

	// Terminal dimensions
	Width  int
	Height int

	// Output channel for streaming bead output
	OutputChan chan execute.StreamEvent

	// Branch name for execution
	BranchName string

	// Ctrl+C confirmation state
	CtrlCPending bool // True when waiting for second Ctrl+C press
}

// NewModel creates a new Model with the given configuration.
func NewModel(cfg *config.Config, projectRoot string) *Model {
	// Initialize text input
	ti := textinput.New()
	ti.Placeholder = "Type your message..."
	ti.CharLimit = 2000
	ti.SetWidth(80)

	// Initialize textarea
	ta := textarea.New()
	ta.Placeholder = "Enter your response..."
	ta.CharLimit = 5000

	// Initialize spinner
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	// Initialize viewport with functional options (v2 API)
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(24))

	// Initialize list with empty items
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 80, 24)
	l.Title = "Sessions"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	return &Model{
		State:       StateHome,
		ActiveTab:   TabChat,
		Cfg:         cfg,
		ProjectRoot: projectRoot,

		// Initialize slices
		ChatHistory: make([]ChatMessage, 0),
		Questions:   make([]Question, 0),
		Answers:     make([]Answer, 0),
		Beads:       make([]BeadState, 0),
		BeadOutput:  make([]string, 0),
		Groups:      make([]ExecutionGroup, 0),
		Learnings:   make([]string, 0),
		Sessions:    make([]SessionInfo, 0),

		// Bubbles components
		TextInput: ti,
		Textarea:  ta,
		Spinner:   sp,
		Viewport:  vp,
		List:      l,

		// Default dimensions (will be updated on WindowSizeMsg)
		Width:  80,
		Height: 24,

		// Output channel - initialized when execution starts
		OutputChan: nil,
	}
}
