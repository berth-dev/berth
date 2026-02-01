// Package app provides the main TUI application that wires all views together.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/execute"
	"github.com/berth-dev/berth/internal/session"
	"github.com/berth-dev/berth/internal/tui"
	"github.com/berth-dev/berth/internal/tui/commands"
	"github.com/berth-dev/berth/internal/tui/views"
)

const analyzingTimeout = 5 * time.Minute

// App is the main TUI application that wires all views together.
type App struct {
	model *tui.Model

	// View models
	initView      views.InitModel
	homeView      views.HomeModel
	interviewView views.InterviewModel
	chatView      views.ChatModel
	planView      views.PlanModel
	executionView views.ExecutionModel
	dashboardView views.DashboardModel
}

// New creates a new App with the given configuration.
func New(cfg *config.Config, projectRoot string) *App {
	model := tui.NewModel(cfg, projectRoot)

	return &App{
		model:    model,
		homeView: views.NewHomeModel(nil, model.Width, model.Height),
	}
}

// Init returns the initial command for the TUI.
// It first checks if the project needs initialization.
func (a *App) Init() tea.Cmd {
	return commands.CheckInitCmd(a.model.ProjectRoot)
}

// Update handles messages and updates the application state.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.model.Width = msg.Width
		a.model.Height = msg.Height
		// Only propagate to the currently active view to avoid nil pointer on uninitialized views
		var cmd tea.Cmd
		switch a.model.State {
		case tui.StateHome:
			a.homeView, cmd = a.homeView.Update(msg)
		case tui.StateInterview:
			a.interviewView, cmd = a.interviewView.Update(msg)
		case tui.StateChat:
			a.chatView, cmd = a.chatView.Update(msg)
		case tui.StateApproval:
			a.planView, cmd = a.planView.Update(msg)
		case tui.StateExecuting:
			a.executionView, cmd = a.executionView.Update(msg)
		case tui.StateDashboard:
			a.dashboardView, cmd = a.dashboardView.Update(msg)
		}
		return a, cmd

	case tea.KeyPressMsg:
		switch msg.String() {
		case tui.KeyCtrlC:
			if a.model.CtrlCPending {
				// Second press within timeout - exit
				return a, tea.Quit
			}
			// First press - set pending and start timeout
			a.model.CtrlCPending = true
			return a, tea.Tick(time.Second, func(t time.Time) tea.Msg {
				return tui.CtrlCResetMsg{}
			})

		case tui.KeyTab:
			// Only cycle tabs when on home or dashboard screens
			if a.model.State == tui.StateHome || a.model.State == tui.StateDashboard {
				cmd := a.cycleTab()
				return a, cmd
			}
			// For other states, let the view handle tab (e.g., toggle selection in init view)
		}

	case tui.CtrlCResetMsg:
		// Reset Ctrl+C confirmation state after timeout
		a.model.CtrlCPending = false
		return a, nil
	}

	// Handle init check message (can arrive before state is set)
	if checkMsg, ok := msg.(tui.InitCheckMsg); ok {
		return a.handleInitCheck(checkMsg)
	}

	// Route messages based on current state
	switch a.model.State {
	case tui.StateInit:
		return a.updateInit(msg)

	case tui.StateHome:
		return a.updateHome(msg)

	case tui.StateAnalyzing:
		return a.updateAnalyzing(msg)

	case tui.StateInterview:
		return a.updateInterview(msg)

	case tui.StateChat:
		return a.updateChat(msg)

	case tui.StateApproval:
		return a.updateApproval(msg)

	case tui.StateExecuting:
		return a.updateExecuting(msg)

	case tui.StateDashboard:
		return a.updateDashboard(msg)

	case tui.StateComplete:
		// Handle any key to exit
		if _, ok := msg.(tea.KeyPressMsg); ok {
			return a, tea.Quit
		}
	}

	return a, tea.Batch(cmds...)
}

// View renders the current application state.
// In Bubble Tea v2, View() returns tea.View instead of string.
// Alt screen mode is enabled via the AltScreen field on the view.
func (a *App) View() tea.View {
	var content string
	var needsCentering bool

	// Sync Ctrl+C pending state to views
	a.initView.SetCtrlCPending(a.model.CtrlCPending)
	a.homeView.SetCtrlCPending(a.model.CtrlCPending)
	a.dashboardView.SetCtrlCPending(a.model.CtrlCPending)

	switch a.model.State {
	case tui.StateInit:
		content = a.initView.View()
		needsCentering = true

	case tui.StateHome:
		content = a.homeView.View()
		needsCentering = true

	case tui.StateAnalyzing:
		content = a.renderAnalyzingView()
		needsCentering = true

	case tui.StateInterview:
		content = a.interviewView.View()
		needsCentering = true

	case tui.StateChat:
		content = a.chatView.View()

	case tui.StateApproval:
		content = a.planView.View()

	case tui.StateExecuting:
		content = a.executionView.View()

	case tui.StateDashboard:
		content = a.dashboardView.View()
		needsCentering = true

	case tui.StateComplete:
		content = a.renderCompleteView()
		needsCentering = true

	default:
		content = "Unknown state"
	}

	// Add tab bar at bottom for applicable states
	if a.shouldShowTabBar() {
		tabBar := a.renderTabBar(a.model.ActiveTab)
		// Join vertically with center alignment so both box and tab bar are aligned
		content = lipgloss.JoinVertical(lipgloss.Center, content, "", tabBar)
	}

	// Center content both horizontally and vertically for applicable states
	if needsCentering {
		content = a.centerContent(content)
	}

	// Create tea.View with alt screen enabled for fullscreen mode
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// centerContent centers the given content both horizontally and vertically.
func (a *App) centerContent(content string) string {
	// Use lipgloss.Place to center content in the available space
	// This properly centers the entire block, not just the text within it
	return lipgloss.Place(
		a.model.Width,
		a.model.Height,
		lipgloss.Center,
		lipgloss.Center,
		content,
	)
}

// ============================================================================
// State Update Handlers
// ============================================================================

// handleInitCheck processes the init check result and transitions to appropriate state.
func (a *App) handleInitCheck(msg tui.InitCheckMsg) (tea.Model, tea.Cmd) {
	if msg.NeedsInit {
		// Project needs initialization - show init prompt
		a.model.State = tui.StateInit
		a.initView = views.NewInitModel(
			a.model.Width,
			a.model.Height,
			filepath.Base(a.model.ProjectRoot),
		)
		return a, a.initView.Init()
	}

	// Project already initialized - go to home
	a.model.State = tui.StateHome
	return a, a.homeView.Init()
}

// updateInit handles messages for the initialization prompt state.
func (a *App) updateInit(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.initView, cmd = a.initView.Update(msg)

	switch msg.(type) {
	case tui.InitConfirmMsg:
		// User confirmed - run initialization with spinner
		a.model.State = tui.StateAnalyzing
		a.model.AnalyzingStartTime = time.Now()
		return a, tea.Batch(
			a.model.Spinner.Tick,
			commands.RunInitCmd(a.model.ProjectRoot),
		)

	case tui.InitDeclineMsg:
		// User declined - exit
		return a, tea.Quit
	}

	return a, cmd
}

func (a *App) updateHome(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.homeView, cmd = a.homeView.Update(msg)

	// Handle messages from home view
	switch msg := msg.(type) {
	case views.SubmitTaskMsg:
		a.model.Err = nil
		a.homeView.Err = nil
		return a, a.transitionToAnalyzing(msg.Description)

	case views.ResumeSessionMsg:
		// Session resume is not yet implemented
		a.model.Err = fmt.Errorf("session resume not yet available (session: %s)", msg.SessionID)
		a.homeView.Err = a.model.Err
		return a, nil
	}

	return a, cmd
}

func (a *App) updateAnalyzing(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case spinner.TickMsg:
		a.model.Spinner, cmd = a.model.Spinner.Update(msg)
		if !a.model.AnalyzingStartTime.IsZero() && time.Since(a.model.AnalyzingStartTime) > analyzingTimeout {
			a.model.Err = fmt.Errorf("operation timed out after %v", analyzingTimeout)
			a.homeView.Err = a.model.Err
			a.model.State = tui.StateHome
			a.model.AnalyzingStartTime = time.Time{}
			return a, nil
		}
		return a, cmd

	case tui.OperationTimeoutMsg:
		a.model.Err = fmt.Errorf("operation %q timed out", msg.Operation)
		a.homeView.Err = a.model.Err
		a.model.State = tui.StateHome
		a.model.AnalyzingStartTime = time.Time{}
		return a, nil

	case tui.InitCompleteMsg:
		// Initialization complete - reload config and transition to home
		a.model.StackInfo = msg.StackInfo

		// Reload config now that it exists
		if cfg, err := config.ReadConfig(a.model.ProjectRoot); err == nil {
			a.model.Cfg = cfg
		}

		a.model.State = tui.StateHome
		a.model.AnalyzingStartTime = time.Time{}
		return a, a.homeView.Init()

	case tui.InitErrorMsg:
		a.model.Err = msg.Err
		a.homeView.Err = msg.Err
		a.model.State = tui.StateHome
		a.model.AnalyzingStartTime = time.Time{}
		return a, a.homeView.Init()

	case tui.AnalysisCompleteMsg:
		a.transitionToInterview(msg.Questions)
		return a, a.interviewView.Init()

	case tui.InterviewStartedMsg:
		a.model.InterviewSession = msg.Session
		return a, nil

	case tui.InterviewQuestionsMsg:
		// Transition to interview state with questions
		a.transitionToInterview(msg.Questions)
		return a, a.interviewView.Init()

	case tui.InterviewReadyMsg:
		// Composite message: store session and transition to interview in one step.
		// This replaces the tea.Batch()() pattern that was causing context issues.
		a.model.InterviewSession = msg.Session
		a.transitionToInterview(msg.Questions)
		return a, a.interviewView.Init()

	case tui.InterviewCompleteMsg:
		a.model.Requirements = msg.Requirements
		a.model.State = tui.StateAnalyzing // show spinner while generating plan
		a.model.AnalyzingStartTime = time.Now()
		return a, tea.Batch(
			a.model.Spinner.Tick,
			commands.GeneratePlanCmd(
				*a.model.Cfg,
				msg.Requirements,
				a.model.GraphSummary,
				a.model.RunDir,
				a.model.IsGreenfield,
			),
		)

	case tui.PlanGeneratedMsg:
		a.TransitionToApproval(msg.Plan, msg.Groups)
		return a, a.planView.Init()

	case tui.BeadsCreatedMsg:
		// Beads have been created in the beads system, now start execution.
		beads := make([]tui.BeadState, len(a.model.Plan.Beads))
		for i, b := range a.model.Plan.Beads {
			beads[i] = tui.BeadState{
				ID:        b.ID,
				Title:     b.Title,
				Status:    "pending",
				BlockedBy: b.DependsOn,
			}
		}
		a.transitionToExecuting(beads)

		// Create output channel for streaming events
		a.model.OutputChan = make(chan execute.StreamEvent, 100)

		// Compute branch name from plan title or use default
		branchName := a.model.BranchName
		if branchName == "" {
			branchName = "berth-execution"
		}

		return a, tea.Batch(
			a.executionView.Init(),
			commands.StartExecutionCmd(
				*a.model.Cfg,
				a.model.ProjectRoot,
				a.model.RunDir,
				branchName,
				a.model.OutputChan,
			),
		)

	case tui.BeadsCreateErrorMsg:
		a.model.Err = msg.Err
		a.homeView.Err = msg.Err
		a.model.State = tui.StateHome
		a.model.AnalyzingStartTime = time.Time{}
		return a, nil

	case tui.PlanErrorMsg:
		a.model.Err = msg.Err
		a.homeView.Err = msg.Err
		a.model.State = tui.StateHome
		return a, nil

	case tui.InterviewErrorMsg:
		a.model.Err = msg.Err
		a.homeView.Err = msg.Err
		a.model.State = tui.StateHome
		return a, nil

	case tui.SessionErrorMsg:
		a.model.Err = msg.Err
		a.homeView.Err = msg.Err
		a.model.State = tui.StateHome
		return a, nil

	case tui.ClaudeErrorMsg:
		a.model.Err = msg.Err
		a.homeView.Err = msg.Err
		a.model.State = tui.StateHome
		return a, nil

	case tui.ErrorMsg:
		a.model.Err = msg.Err
		a.homeView.Err = msg.Err
		a.model.State = tui.StateHome
		return a, nil
	}

	return a, nil
}

func (a *App) updateInterview(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.interviewView, cmd = a.interviewView.Update(msg)

	switch msg := msg.(type) {
	case tui.AnswerMsg:
		// Store answer and move to next question
		a.model.Answers = append(a.model.Answers, tui.Answer{
			ID:    msg.QuestionID,
			Value: msg.Value,
		})
		a.model.CurrentQ++

		// Check if more questions in current round
		if a.model.CurrentQ < len(a.model.Questions) {
			a.interviewView = views.NewInterviewModel(
				a.model.Questions[a.model.CurrentQ],
				a.model.Width,
				a.model.Height,
			)
			return a, a.interviewView.Init()
		}
		// All questions in this round answered - process and get next round or complete
		a.model.State = tui.StateAnalyzing
		a.model.AnalyzingStartTime = time.Now()
		return a, tea.Batch(
			a.model.Spinner.Tick,
			commands.ProcessAnswersCmd(a.model.InterviewSession, a.model.Answers),
		)

	case tui.EnterChatMsg:
		// Transition to chat mode for this question
		a.model.InChatMode = true
		a.model.State = tui.StateChat
		a.chatView = views.NewChatModel(
			msg.QuestionID,
			a.model.ChatHistory,
			a.model.Width,
			a.model.Height,
		)
		return a, a.chatView.Init()

	case tui.SkipInterviewMsg:
		// Skip remaining questions and go directly to planning
		a.model.State = tui.StateAnalyzing
		a.model.AnalyzingStartTime = time.Now()
		return a, tea.Batch(
			a.model.Spinner.Tick,
			commands.ProcessAnswersCmd(a.model.InterviewSession, a.model.Answers),
		)

	case tui.GoHomeMsg:
		// Return to home screen
		a.model.State = tui.StateHome
		a.model.ActiveTab = tui.TabChat
		return a, a.homeView.Init()
	}

	return a, cmd
}

func (a *App) updateChat(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.chatView, cmd = a.chatView.Update(msg)

	switch msg.(type) {
	case views.SendChatMsg:
		// Chat feature is not yet fully implemented
		// Return a placeholder response for now
		return a, func() tea.Msg {
			return views.ChatResponseMsg{
				Content: "Chat feature is not yet available. Please use the interview flow to provide answers.",
			}
		}

	case views.ExitChatMsg:
		// Return to previous state (interview or execution)
		if a.model.InChatMode {
			a.model.State = tui.StateInterview
			a.model.InChatMode = false
		}
		return a, nil
	}

	return a, cmd
}

func (a *App) updateApproval(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.planView, cmd = a.planView.Update(msg)

	switch msg := msg.(type) {
	case tui.ApproveMsg:
		// First, create beads in the beads system before execution.
		// Show spinner while creating beads.
		a.model.State = tui.StateAnalyzing
		a.model.AnalyzingStartTime = time.Now()
		return a, tea.Batch(
			a.model.Spinner.Tick,
			commands.CreateBeadsCmd(a.model.Plan, a.model.ProjectRoot),
		)

	case tui.RejectMsg:
		a.model.State = tui.StateAnalyzing
		a.model.AnalyzingStartTime = time.Now()
		return a, tea.Batch(
			a.model.Spinner.Tick,
			commands.RegeneratePlanCmd(
				*a.model.Cfg,
				a.model.Requirements,
				a.model.GraphSummary,
				a.model.RunDir,
				a.model.IsGreenfield,
				msg.Feedback,
			),
		)
	}

	return a, cmd
}

func (a *App) updateExecuting(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.executionView, cmd = a.executionView.Update(msg)

	switch msg := msg.(type) {
	case tui.ExecutionStartedMsg:
		// Start listening for execution events
		return a, commands.ListenExecutionCmd(a.model.OutputChan)

	case tui.ExecutionEventMsg:
		// Handle streaming events from execution
		switch msg.Event.Type {
		case "bead_init":
			// Find bead and mark as running
			a.updateBeadStatus(msg.Event.BeadID, "running")
		case "output":
			// Append to current bead output
			a.model.BeadOutput = append(a.model.BeadOutput, msg.Event.Content)
		case "bead_complete":
			a.updateBeadStatus(msg.Event.BeadID, "success")
		case "error":
			a.updateBeadStatus(msg.Event.BeadID, "failed")
		case "token_update":
			a.model.TokenCount += msg.Event.Tokens
		}
		// Continue listening for more events
		return a, commands.ListenExecutionCmd(a.model.OutputChan)

	case tui.ExecutionCompleteMsg:
		a.model.State = tui.StateComplete
		return a, nil

	case tui.TickMsg:
		// Keep polling for events if execution is in progress
		if a.model.OutputChan != nil {
			return a, commands.ListenExecutionCmd(a.model.OutputChan)
		}
		return a, nil

	case tui.PauseMsg:
		a.model.IsPaused = msg.Paused
		return a, cmd

	case tui.SkipBeadMsg:
		// Mark bead as skipped and continue
		for i := range a.model.Beads {
			if a.model.Beads[i].ID == msg.BeadID {
				a.model.Beads[i].Status = "skipped"
				break
			}
		}
		return a, cmd

	case tui.BeadCompleteMsg:
		// Check if all beads are done
		allDone := true
		for _, bead := range a.model.Beads {
			if bead.Status == "pending" || bead.Status == "running" {
				allDone = false
				break
			}
		}
		if allDone {
			a.transitionToComplete()
		}
		return a, cmd
	}

	return a, cmd
}

func (a *App) updateDashboard(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.dashboardView, cmd = a.dashboardView.Update(msg)

	switch msg := msg.(type) {
	case views.LoadSessionMsg:
		// Session resume is not yet implemented
		a.model.Err = fmt.Errorf("session resume not yet available (session: %s)", msg.SessionID)
		a.homeView.Err = a.model.Err
		a.model.State = tui.StateHome
		return a, nil

	case tui.ArchitectureDiagramMsg:
		// Cache diagram in model for future use
		if msg.Err == nil {
			a.model.Diagram = msg.Diagram
		}
		return a, cmd

	case tui.LearningsLoadMsg:
		// Cache learnings in model for future use
		if msg.Err == nil {
			a.model.Learnings = msg.Learnings
		}
		return a, cmd

	case tui.SessionsLoadMsg:
		// Cache sessions in model for future use
		if msg.Err == nil {
			a.model.Sessions = msg.Sessions
		}
		return a, cmd
	}

	return a, cmd
}

// ============================================================================
// State Transitions
// ============================================================================

// transitionToAnalyzing initiates the analysis phase and starts the interview.
func (a *App) transitionToAnalyzing(description string) tea.Cmd {
	a.model.State = tui.StateAnalyzing
	a.model.AnalyzingStartTime = time.Now()

	// Create run directory if not already set.
	// This mirrors the behavior of cli/run.go which creates .berth/runs/<timestamp>.
	if a.model.RunDir == "" {
		timestamp := time.Now().Format("20060102-150405")
		a.model.RunDir = filepath.Join(".berth", "runs", timestamp)
		if err := os.MkdirAll(a.model.RunDir, 0755); err != nil {
			// Return error message to transition back to home with error
			return func() tea.Msg {
				return tui.ErrorMsg{Err: fmt.Errorf("creating run directory: %w", err)}
			}
		}
	}

	// Add initial system message
	a.model.ChatHistory = append(a.model.ChatHistory, tui.ChatMessage{
		Role:    "system",
		Content: fmt.Sprintf("Task: %s", description),
	})

	// Start spinner and interview command
	return tea.Batch(
		a.model.Spinner.Tick,
		commands.StartInterviewCmd(
			*a.model.Cfg,
			a.model.StackInfo,
			description,
			a.model.RunDir,
			a.model.GraphSummary,
		),
	)
}

// transitionToInterview sets up the interview phase with questions.
func (a *App) transitionToInterview(questions []tui.Question) {
	a.model.State = tui.StateInterview
	a.model.Questions = questions
	a.model.CurrentQ = 0
	a.model.Answers = nil              // Clear answers for new round
	a.model.AnalyzingStartTime = time.Time{} // Reset timeout tracker

	if len(questions) > 0 {
		a.interviewView = views.NewInterviewModel(
			questions[0],
			a.model.Width,
			a.model.Height,
		)
	}
}

// TransitionToApproval sets up the plan approval phase.
func (a *App) TransitionToApproval(plan *tui.Plan, groups []tui.ExecutionGroup) {
	a.model.State = tui.StateApproval
	a.model.Plan = plan
	a.model.Groups = groups
	a.model.AnalyzingStartTime = time.Time{} // Reset timeout tracker

	a.planView = views.NewPlanModel(
		plan,
		groups,
		a.model.Width,
		a.model.Height,
	)
}

// transitionToExecuting sets up the bead execution phase.
func (a *App) transitionToExecuting(beads []tui.BeadState) {
	a.model.State = tui.StateExecuting
	a.model.Beads = beads
	a.model.CurrentBead = 0

	a.executionView = views.NewExecutionModel(
		beads,
		false, // parallel mode determined by config
		a.model.Width,
		a.model.Height,
	)
}

// transitionToComplete marks the session as complete.
func (a *App) transitionToComplete() {
	a.model.State = tui.StateComplete
}

// ============================================================================
// Helper Methods
// ============================================================================

// updateBeadStatus updates the status of a bead by ID.
func (a *App) updateBeadStatus(beadID string, status string) {
	for i := range a.model.Beads {
		if a.model.Beads[i].ID == beadID {
			a.model.Beads[i].Status = status
			break
		}
	}
}

// cycleTab cycles through available tabs (Chat ↔ Dashboard).
// Returns a command to initialize the new view if needed.
func (a *App) cycleTab() tea.Cmd {
	switch a.model.ActiveTab {
	case tui.TabChat:
		a.model.ActiveTab = tui.TabDashboard
		a.model.State = tui.StateDashboard

		// Build dependencies for dashboard
		deps := &views.DashboardDeps{
			KGClient:    a.model.KGClient,
			ProjectRoot: a.model.ProjectRoot,
		}

		// Type assert Store if available
		if store, ok := a.model.Store.(*session.Store); ok {
			deps.Store = store
		}

		// Determine root file for architecture diagram
		// Use a sensible default based on detected stack
		deps.RootFile = a.determineRootFile()

		a.dashboardView = views.NewDashboardModel(
			a.model.Diagram,
			a.model.Learnings,
			a.model.Sessions,
			a.model.Width,
			a.model.Height,
			deps,
		)

		// Return Init command to load dashboard data
		return a.dashboardView.Init()

	case tui.TabDashboard:
		a.model.ActiveTab = tui.TabChat
		a.model.State = tui.StateHome
	}

	return nil
}

// determineRootFile returns the root file path for architecture diagram.
// It uses a sensible default based on the detected stack.
func (a *App) determineRootFile() string {
	// Use stack info to determine entry point
	switch a.model.StackInfo.Language {
	case "go":
		return "main.go"
	case "typescript", "javascript":
		if a.model.StackInfo.Framework == "next" {
			return "app/page.tsx"
		}
		return "src/index.ts"
	case "python":
		return "main.py"
	case "rust":
		return "src/main.rs"
	default:
		// Return empty string - LoadDiagramCmd handles nil/empty gracefully
		return ""
	}
}

// shouldShowTabBar returns true if the tab bar should be displayed.
func (a *App) shouldShowTabBar() bool {
	switch a.model.State {
	case tui.StateHome, tui.StateDashboard:
		return true
	default:
		return false
	}
}

// renderTabBar renders the tab bar with the active tab highlighted.
func (a *App) renderTabBar(activeTab tui.Tab) string {
	tabs := []struct {
		name string
		tab  tui.Tab
	}{
		{"Home", tui.TabChat},
		{"Dashboard", tui.TabDashboard},
	}

	var rendered []string
	for _, t := range tabs {
		if t.tab == activeTab {
			rendered = append(rendered, tui.ActiveTabStyle.Render(t.name))
		} else {
			rendered = append(rendered, tui.InactiveTabStyle.Render(t.name))
		}
	}

	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
	return lipgloss.NewStyle().
		Width(a.model.Width).
		Align(lipgloss.Center).
		Render(tabBar)
}

// renderAnalyzingView renders the loading/analyzing state.
func (a *App) renderAnalyzingView() string {
	var b strings.Builder

	// Header
	header := tui.TitleStyle.Render("Analyzing Project...")
	b.WriteString(header)
	b.WriteString("\n\n")

	// Spinner and status
	spinnerLine := fmt.Sprintf("%s Understanding your codebase...", a.model.Spinner.View())
	b.WriteString(spinnerLine)
	b.WriteString("\n\n")

	// Progress hints
	hints := []string{
		"Detecting project structure",
		"Querying knowledge graph",
		"Generating clarifying questions",
	}
	for _, hint := range hints {
		b.WriteString(tui.DimStyle.Render("  - " + hint))
		b.WriteString("\n")
	}

	// Determine box width - use max width or screen width, whichever is smaller
	const maxBoxWidth = 70
	boxWidth := maxBoxWidth
	if a.model.Width-4 < boxWidth {
		boxWidth = a.model.Width - 4
	}

	// Wrap in box with fixed max width
	content := b.String()
	boxed := tui.BoxStyle.
		Width(boxWidth).
		Render(content)

	return boxed
}

// renderCompleteView renders the completion summary.
func (a *App) renderCompleteView() string {
	var b strings.Builder

	// Header
	header := tui.SuccessStyle.Render("Execution Complete!")
	b.WriteString(header)
	b.WriteString("\n\n")

	// Summary stats
	successCount := 0
	failedCount := 0
	skippedCount := 0
	for _, bead := range a.model.Beads {
		switch bead.Status {
		case "success":
			successCount++
		case "failed":
			failedCount++
		case "skipped":
			skippedCount++
		}
	}

	b.WriteString(fmt.Sprintf("Total beads: %d\n", len(a.model.Beads)))
	b.WriteString(tui.SuccessStyle.Render(fmt.Sprintf("Succeeded: %d\n", successCount)))
	if failedCount > 0 {
		b.WriteString(tui.ErrorStyle.Render(fmt.Sprintf("Failed: %d\n", failedCount)))
	}
	if skippedCount > 0 {
		b.WriteString(tui.DimStyle.Render(fmt.Sprintf("Skipped: %d\n", skippedCount)))
	}

	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Total tokens: %d\n", a.model.TokenCount))
	b.WriteString(fmt.Sprintf("Duration: %s\n", a.model.ElapsedTime))

	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("Press any key to exit..."))

	// Determine box width - use max width or screen width, whichever is smaller
	const maxBoxWidth = 70
	boxWidth := maxBoxWidth
	if a.model.Width-4 < boxWidth {
		boxWidth = a.model.Width - 4
	}

	// Wrap in box with fixed max width
	content := b.String()
	boxed := tui.BoxStyle.
		Width(boxWidth).
		Render(content)

	return boxed
}
