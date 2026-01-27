package execute

import (
	"testing"
)

func TestParseClaudeOutput_Valid(t *testing.T) {
	raw := []byte(`{
		"type": "result",
		"result": "Created auth.go with login function",
		"cost_usd": 0.042,
		"duration_ms": 15000,
		"session_id": "sess-abc123",
		"is_error": false,
		"num_turns": 3
	}`)

	out, err := ParseClaudeOutput(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.Result != "Created auth.go with login function" {
		t.Errorf("Result = %q, want %q", out.Result, "Created auth.go with login function")
	}
	if out.CostUSD != 0.042 {
		t.Errorf("CostUSD = %f, want %f", out.CostUSD, 0.042)
	}
	if out.DurationMS != 15000 {
		t.Errorf("DurationMS = %d, want %d", out.DurationMS, 15000)
	}
	if out.SessionID != "sess-abc123" {
		t.Errorf("SessionID = %q, want %q", out.SessionID, "sess-abc123")
	}
	if out.IsError {
		t.Error("IsError = true, want false")
	}
}

func TestParseClaudeOutput_ErrorResult(t *testing.T) {
	raw := []byte(`{
		"type": "result",
		"result": "Failed to complete task",
		"cost_usd": 0.01,
		"duration_ms": 5000,
		"session_id": "sess-err",
		"is_error": true,
		"num_turns": 1
	}`)

	out, err := ParseClaudeOutput(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !out.IsError {
		t.Error("IsError = false, want true")
	}
	if out.Result != "Failed to complete task" {
		t.Errorf("Result = %q, want %q", out.Result, "Failed to complete task")
	}
}

func TestParseClaudeOutput_Empty(t *testing.T) {
	_, err := ParseClaudeOutput([]byte{})
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
}

func TestParseClaudeOutput_InvalidJSON(t *testing.T) {
	_, err := ParseClaudeOutput([]byte(`not json at all`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestParseClaudeOutput_WrongType(t *testing.T) {
	raw := []byte(`{
		"type": "error",
		"result": "something broke",
		"cost_usd": 0,
		"duration_ms": 0,
		"session_id": "",
		"is_error": true,
		"num_turns": 0
	}`)

	_, err := ParseClaudeOutput(raw)
	if err == nil {
		t.Error("expected error for wrong type, got nil")
	}
}

func TestParseClaudeOutput_MinimalValid(t *testing.T) {
	raw := []byte(`{"type":"result","result":"ok"}`)

	out, err := ParseClaudeOutput(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Result != "ok" {
		t.Errorf("Result = %q, want %q", out.Result, "ok")
	}
	if out.CostUSD != 0 {
		t.Errorf("CostUSD = %f, want 0", out.CostUSD)
	}
}

func TestParseClaudeOutput_ZeroCost(t *testing.T) {
	raw := []byte(`{
		"type": "result",
		"result": "cached response",
		"cost_usd": 0,
		"duration_ms": 100,
		"session_id": "sess-cached",
		"is_error": false,
		"num_turns": 0
	}`)

	out, err := ParseClaudeOutput(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.CostUSD != 0 {
		t.Errorf("CostUSD = %f, want 0", out.CostUSD)
	}
}
