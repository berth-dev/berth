package plan

import (
	"testing"
)

func TestParsePlan_ValidPlan(t *testing.T) {
	input := `# Add OAuth with Google

This plan implements Google OAuth for the application.

### bt-1: Add Google sign-in to auth store
- files: [src/stores/auth.ts]
- context: Implement Google sign-in using Firebase auth
- depends: none
- verify_extra: ["pnpm test -- --grep auth"]

### bt-2: Add Google login button
- files: [src/components/LoginButton.tsx, src/components/LoginButton.css]
- context: Create a styled Google login button component
- depends: bt-1
- verify_extra: none

### bt-3: Handle OAuth redirect
- files: [src/pages/callback.tsx]
- context: Process the OAuth redirect and store tokens
- depends: bt-1
`

	plan, err := ParsePlan(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.Title != "Add OAuth with Google" {
		t.Errorf("Title = %q, want %q", plan.Title, "Add OAuth with Google")
	}

	if len(plan.Beads) != 3 {
		t.Fatalf("expected 3 beads, got %d", len(plan.Beads))
	}

	// Verify first bead
	b0 := plan.Beads[0]
	if b0.ID != "bt-1" {
		t.Errorf("Beads[0].ID = %q, want %q", b0.ID, "bt-1")
	}
	if b0.Title != "Add Google sign-in to auth store" {
		t.Errorf("Beads[0].Title = %q, want %q", b0.Title, "Add Google sign-in to auth store")
	}
	if len(b0.Files) != 1 || b0.Files[0] != "src/stores/auth.ts" {
		t.Errorf("Beads[0].Files = %v, want [src/stores/auth.ts]", b0.Files)
	}
	if len(b0.DependsOn) != 0 {
		t.Errorf("Beads[0].DependsOn = %v, want empty", b0.DependsOn)
	}
	if len(b0.VerifyExtra) != 1 || b0.VerifyExtra[0] != "pnpm test -- --grep auth" {
		t.Errorf("Beads[0].VerifyExtra = %v, want [pnpm test -- --grep auth]", b0.VerifyExtra)
	}

	// Verify second bead has dependency
	b1 := plan.Beads[1]
	if len(b1.DependsOn) != 1 || b1.DependsOn[0] != "bt-1" {
		t.Errorf("Beads[1].DependsOn = %v, want [bt-1]", b1.DependsOn)
	}
	if len(b1.Files) != 2 {
		t.Errorf("Beads[1].Files count = %d, want 2", len(b1.Files))
	}

	// Verify third bead
	b2 := plan.Beads[2]
	if b2.ID != "bt-3" {
		t.Errorf("Beads[2].ID = %q, want %q", b2.ID, "bt-3")
	}
}

func TestParsePlan_NoBeads(t *testing.T) {
	input := `# Empty Plan

This plan has no beads defined.
Just some description text.
`

	_, err := ParsePlan(input)
	if err == nil {
		t.Error("expected error for plan with no beads, got nil")
	}
}

func TestParsePlan_EmptyInput(t *testing.T) {
	_, err := ParsePlan("")
	if err == nil {
		t.Error("expected error for empty input, got nil")
	}
}

func TestParsePlan_NoTitleFallback(t *testing.T) {
	input := `Some text without heading

### bt-1: Do something
- files: [main.go]
- context: Implement the thing
- depends: none
`

	plan, err := ParsePlan(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.Title != "Some text without heading" {
		t.Errorf("Title = %q, want %q", plan.Title, "Some text without heading")
	}
}

func TestParsePlan_MultipleDependencies(t *testing.T) {
	input := `# Test Plan

### bt-1: First task
- files: [a.go]
- context: Do first thing
- depends: none

### bt-2: Second task
- files: [b.go]
- context: Do second thing
- depends: none

### bt-3: Third task depends on both
- files: [c.go]
- context: Combine results
- depends: bt-1, bt-2
`

	plan, err := ParsePlan(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Beads) != 3 {
		t.Fatalf("expected 3 beads, got %d", len(plan.Beads))
	}

	b2 := plan.Beads[2]
	if len(b2.DependsOn) != 2 {
		t.Fatalf("Beads[2].DependsOn count = %d, want 2", len(b2.DependsOn))
	}
	if b2.DependsOn[0] != "bt-1" || b2.DependsOn[1] != "bt-2" {
		t.Errorf("Beads[2].DependsOn = %v, want [bt-1, bt-2]", b2.DependsOn)
	}
}

func TestParsePlan_NoSpaceAfterHash(t *testing.T) {
	input := `# Test

###bt-1: No space after hashes
- files: [x.go]
- context: test
- depends: none
`

	plan, err := ParsePlan(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Beads) != 1 {
		t.Fatalf("expected 1 bead, got %d", len(plan.Beads))
	}
	if plan.Beads[0].ID != "bt-1" {
		t.Errorf("Beads[0].ID = %q, want %q", plan.Beads[0].ID, "bt-1")
	}
}

func TestParseFilesList(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"[src/a.ts, src/b.ts]", []string{"src/a.ts", "src/b.ts"}},
		{"src/a.ts, src/b.ts", []string{"src/a.ts", "src/b.ts"}},
		{"[main.go]", []string{"main.go"}},
		{"main.go", []string{"main.go"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseFilesList(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("got %d files, want %d: %v", len(result), len(tt.expected), result)
			}
			for i, f := range tt.expected {
				if result[i] != f {
					t.Errorf("file[%d] = %q, want %q", i, result[i], f)
				}
			}
		})
	}
}

func TestParseDependsList(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"none", 0},
		{"", 0},
		{"[]", 0},
		{"n/a", 0},
		{"bt-1", 1},
		{"bt-1, bt-2", 2},
		{"bt-1, bt-2, bt-3", 3},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseDependsList(tt.input)
			if len(result) != tt.expected {
				t.Errorf("got %d deps, want %d: %v", len(result), tt.expected, result)
			}
		})
	}
}

func TestParseVerifyExtra(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"json array", `["pnpm test", "pnpm lint"]`, 2},
		{"none", "none", 0},
		{"empty brackets", "[]", 0},
		{"empty string", "", 0},
		{"single command no quotes", "pnpm test", 1},
		{"csv no quotes", "pnpm test, pnpm lint", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseVerifyExtra(tt.input)
			if len(result) != tt.expected {
				t.Errorf("got %d cmds, want %d: %v", len(result), tt.expected, result)
			}
		})
	}
}

func TestParsePlan_Description(t *testing.T) {
	input := `# My Plan

This is the plan description.
It spans multiple lines.

### bt-1: Only task
- files: [x.go]
- context: do it
- depends: none
`

	plan, err := ParsePlan(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.Description == "" {
		t.Error("expected non-empty description")
	}
}

func TestParsePlan_RawOutputPreserved(t *testing.T) {
	input := `# Plan

### bt-1: Task
- files: [x.go]
- context: do it
- depends: none
`

	plan, err := ParsePlan(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if plan.RawOutput != input {
		t.Error("RawOutput should preserve original input")
	}
}
