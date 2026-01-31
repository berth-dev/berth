// Package commands provides Bubble Tea commands for TUI operations.
package commands

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/berth-dev/berth/internal/config"
	"github.com/berth-dev/berth/internal/detect"
	"github.com/berth-dev/berth/internal/tui"
	"github.com/berth-dev/berth/internal/understand"
)

// StartInterviewCmd starts an interview session and returns the first questions.
// It spawns Claude to generate initial questions based on the project context.
// Returns InterviewStartedMsg with the session, followed by InterviewQuestionsMsg
// with the first set of questions, or InterviewErrorMsg on failure.
func StartInterviewCmd(
	cfg config.Config,
	stackInfo detect.StackInfo,
	description, runDir, graphSummary string,
) tea.Cmd {
	return func() tea.Msg {
		session, questions, err := understand.StartInterviewSession(
			context.Background(),
			cfg, stackInfo, description, runDir, graphSummary,
		)
		if err != nil {
			return tui.InterviewErrorMsg{Err: err}
		}

		// Convert understand.Question to tui.Question
		tuiQuestions := convertQuestions(questions)

		// If no questions were returned, the interview may be complete immediately
		// (very simple task). Return the session so the TUI can handle this case.
		if len(tuiQuestions) == 0 {
			return tui.InterviewStartedMsg{Session: session}
		}

		// Return a batch of messages: first the session, then the questions
		return tea.Batch(
			func() tea.Msg {
				return tui.InterviewStartedMsg{Session: session}
			},
			func() tea.Msg {
				return tui.InterviewQuestionsMsg{
					Questions: tuiQuestions,
					Round:     session.CurrentRound,
				}
			},
		)()
	}
}

// ProcessAnswersCmd sends user answers to the interview session and returns
// either the next set of questions or the final requirements.
// Returns InterviewQuestionsMsg for more questions, InterviewCompleteMsg when
// done, or InterviewErrorMsg on failure.
func ProcessAnswersCmd(session *understand.InterviewSession, answers []tui.Answer) tea.Cmd {
	return func() tea.Msg {
		// Convert tui.Answer to understand.Answer
		understandAnswers := convertToUnderstandAnswers(answers)

		questions, isDone, reqs, err := session.ContinueInterview(understandAnswers)
		if err != nil {
			return tui.InterviewErrorMsg{Err: err}
		}

		if isDone {
			return tui.InterviewCompleteMsg{Requirements: reqs}
		}

		return tui.InterviewQuestionsMsg{
			Questions: convertQuestions(questions),
			Round:     session.CurrentRound,
		}
	}
}

// convertQuestions converts a slice of understand.Question to tui.Question.
func convertQuestions(questions []understand.Question) []tui.Question {
	result := make([]tui.Question, len(questions))
	for i, q := range questions {
		result[i] = tui.Question{
			ID:          q.ID,
			Text:        q.Text,
			Options:     convertOptions(q.Options),
			AllowCustom: q.AllowCustom,
			AllowHelp:   q.AllowHelp,
		}
	}
	return result
}

// convertOptions converts a slice of understand.Option to tui.Option.
func convertOptions(options []understand.Option) []tui.Option {
	result := make([]tui.Option, len(options))
	for i, o := range options {
		result[i] = tui.Option{
			Key:         o.Key,
			Label:       o.Label,
			Recommended: o.Recommended,
		}
	}
	return result
}

// convertToUnderstandAnswers converts a slice of tui.Answer to understand.Answer.
func convertToUnderstandAnswers(answers []tui.Answer) []understand.Answer {
	result := make([]understand.Answer, len(answers))
	for i, a := range answers {
		result[i] = understand.Answer{
			ID:    a.ID,
			Value: a.Value,
		}
	}
	return result
}
