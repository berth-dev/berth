// Package commands provides Bubble Tea commands for TUI operations.
package commands

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

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

		// Return a single composite message containing both session and questions.
		// This avoids the tea.Batch()() anti-pattern which caused context
		// cancellation issues by immediately invoking the batch instead of
		// returning it to the Bubble Tea runtime.
		return tui.InterviewReadyMsg{
			Session:   session,
			Questions: tuiQuestions,
			Round:     session.CurrentRound,
		}
	}
}

// ProcessAnswersCmd sends user answers to the interview session and returns
// either the next set of questions or the final requirements.
// Returns InterviewQuestionsMsg for more questions, InterviewCompleteMsg when
// done, or InterviewErrorMsg on failure.
func ProcessAnswersCmd(session *understand.InterviewSession, answers []tui.Answer) tea.Cmd {
	return func() tea.Msg {
		// Validate answers before processing
		if len(answers) == 0 {
			return tui.InterviewErrorMsg{Err: fmt.Errorf("no answers provided")}
		}
		for i, a := range answers {
			// Check if answer has either a single value or multi-select values
			hasValue := strings.TrimSpace(a.Value) != ""
			hasValues := len(a.Values) > 0
			if !hasValue && !hasValues {
				return tui.InterviewErrorMsg{Err: fmt.Errorf("answer %d (ID: %s) is empty", i+1, a.ID)}
			}
			if strings.TrimSpace(a.ID) == "" {
				return tui.InterviewErrorMsg{Err: fmt.Errorf("answer %d has no question ID", i+1)}
			}
		}

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
			ShortLabel:  q.ShortLabel,
			Options:     convertOptions(q.Options),
			AllowCustom: q.AllowCustom,
			AllowHelp:   q.AllowHelp,
			MultiSelect: q.MultiSelect,
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
			Description: o.Description,
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
			ID:     a.ID,
			Value:  a.Value,
			Values: a.Values,
		}
	}
	return result
}
