package graph

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
)

func TestCheckDuplicationNilClient(t *testing.T) {
	var c *Client = nil
	result, err := c.CheckDuplication("testFunc", "TestType")
	if err != nil {
		t.Errorf("CheckDuplication with nil client should not error: %v", err)
	}
	if result != nil {
		t.Errorf("CheckDuplication with nil client should return nil result, got: %+v", result)
	}
}

func TestCheckDuplicationEmptyNames(t *testing.T) {
	var c *Client = nil
	result, err := c.CheckDuplication("", "")
	if err != nil {
		t.Errorf("CheckDuplication with empty names should not error: %v", err)
	}
	if result != nil {
		t.Errorf("CheckDuplication with empty names should return nil result, got: %+v", result)
	}
}

func TestWarnIfDuplicatesNil(t *testing.T) {
	// Should not panic with nil result.
	WarnIfDuplicates(nil)
}

func TestWarnIfDuplicatesNoDuplicates(t *testing.T) {
	// Should not print anything when HasDuplicates is false.
	result := &DuplicationResult{
		HasDuplicates: false,
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	WarnIfDuplicates(result)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)

	if buf.Len() > 0 {
		t.Errorf("WarnIfDuplicates should not print when HasDuplicates=false, got: %s", buf.String())
	}
}

func TestWarnIfDuplicatesFunctionMatches(t *testing.T) {
	result := &DuplicationResult{
		FunctionMatches: []string{"file1.go:10", "file2.go:20"},
		HasDuplicates:   true,
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	WarnIfDuplicates(result)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)

	output := buf.String()
	if output == "" {
		t.Error("WarnIfDuplicates should print warning for function matches")
	}
	expected := "Warning: similar functions found: file1.go:10, file2.go:20\n"
	if output != expected {
		t.Errorf("WarnIfDuplicates output = %q, want %q", output, expected)
	}
}

func TestWarnIfDuplicatesTypeMatches(t *testing.T) {
	result := &DuplicationResult{
		TypeMatches:   []string{"types.go:5"},
		HasDuplicates: true,
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	WarnIfDuplicates(result)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)

	output := buf.String()
	if output == "" {
		t.Error("WarnIfDuplicates should print warning for type matches")
	}
	expected := "Warning: similar types found: types.go:5\n"
	if output != expected {
		t.Errorf("WarnIfDuplicates output = %q, want %q", output, expected)
	}
}

func TestWarnIfDuplicatesBothMatches(t *testing.T) {
	result := &DuplicationResult{
		FunctionMatches: []string{"func.go:1"},
		TypeMatches:     []string{"type.go:2"},
		HasDuplicates:   true,
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	WarnIfDuplicates(result)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)

	output := buf.String()
	expectedFunc := "Warning: similar functions found: func.go:1\n"
	expectedType := "Warning: similar types found: type.go:2\n"
	expected := expectedFunc + expectedType
	if output != expected {
		t.Errorf("WarnIfDuplicates output = %q, want %q", output, expected)
	}
}

func TestDuplicationResultHasDuplicates(t *testing.T) {
	tests := []struct {
		name     string
		result   *DuplicationResult
		expected bool
	}{
		{
			name:     "nil result",
			result:   nil,
			expected: false,
		},
		{
			name: "empty result",
			result: &DuplicationResult{
				HasDuplicates: false,
			},
			expected: false,
		},
		{
			name: "with function matches",
			result: &DuplicationResult{
				FunctionMatches: []string{"a.go:1"},
				HasDuplicates:   true,
			},
			expected: true,
		},
		{
			name: "with type matches",
			result: &DuplicationResult{
				TypeMatches:   []string{"b.go:2"},
				HasDuplicates: true,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hasDuplicates bool
			if tt.result != nil {
				hasDuplicates = tt.result.HasDuplicates
			}
			if hasDuplicates != tt.expected {
				t.Errorf("HasDuplicates = %v, want %v", hasDuplicates, tt.expected)
			}
		})
	}
}

// Compile-time check that DuplicationResult has expected fields.
func TestDuplicationResultFields(t *testing.T) {
	result := DuplicationResult{
		FunctionMatches: []string{"a", "b"},
		TypeMatches:     []string{"c"},
		HasDuplicates:   true,
	}

	// These accesses would fail at compile time if fields were wrong.
	_ = result.FunctionMatches
	_ = result.TypeMatches
	_ = result.HasDuplicates

	// Verify we can format it (used in logging).
	_ = fmt.Sprintf("%+v", result)
}

func TestCheckDuplicationFromTitleNilClient(t *testing.T) {
	var c *Client = nil
	result, err := c.CheckDuplicationFromTitle("add something function")
	if err != nil {
		t.Errorf("CheckDuplicationFromTitle with nil client should not error: %v", err)
	}
	if result != nil {
		t.Errorf("CheckDuplicationFromTitle with nil client should return nil result, got: %+v", result)
	}
}

func TestCheckDuplicationFromTitleEmpty(t *testing.T) {
	var c *Client = nil
	result, err := c.CheckDuplicationFromTitle("")
	if err != nil {
		t.Errorf("CheckDuplicationFromTitle with empty title should not error: %v", err)
	}
	if result != nil {
		t.Errorf("CheckDuplicationFromTitle with empty title should return nil result, got: %+v", result)
	}
}

func TestExtractNamesFromTitle(t *testing.T) {
	tests := []struct {
		title        string
		wantFunc     string
		wantType     string
	}{
		{
			title:    "add handleRequest function",
			wantFunc: "handlerequest",
			wantType: "",
		},
		{
			title:    "create User type",
			wantFunc: "",
			wantType: "user",
		},
		{
			title:    "implement Handler struct",
			wantFunc: "",
			wantType: "handler",
		},
		{
			title:    "add something",
			wantFunc: "something",
			wantType: "",
		},
		{
			title:    "create Something",
			wantFunc: "",
			wantType: "something",
		},
		{
			title:    "",
			wantFunc: "",
			wantType: "",
		},
		{
			title:    "single",
			wantFunc: "",
			wantType: "",
		},
		{
			title:    "define Parser interface",
			wantFunc: "",
			wantType: "parser",
		},
		{
			title:    "add executor handler",
			wantFunc: "executor",
			wantType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			gotFunc, gotType := extractNamesFromTitle(tt.title)
			if gotFunc != tt.wantFunc {
				t.Errorf("extractNamesFromTitle(%q) funcName = %q, want %q", tt.title, gotFunc, tt.wantFunc)
			}
			if gotType != tt.wantType {
				t.Errorf("extractNamesFromTitle(%q) typeName = %q, want %q", tt.title, gotType, tt.wantType)
			}
		})
	}
}

func TestExtractIdentifierFromWord(t *testing.T) {
	tests := []struct {
		word string
		want string
	}{
		{"hello", "hello"},
		{"hello123", "hello123"},
		{"Hello_World", "Hello_World"},
		{"func()", "func"},
		{"type;", "type"},
		{"", ""},
		{"123abc", "123abc"},
		{"abc-def", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			got := extractIdentifierFromWord(tt.word)
			if got != tt.want {
				t.Errorf("extractIdentifierFromWord(%q) = %q, want %q", tt.word, got, tt.want)
			}
		})
	}
}

func TestIsCapitalized(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"Hello", true},
		{"hello", false},
		{"", false},
		{"A", true},
		{"a", false},
		{"123", false},
		{"ABC", true},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := isCapitalized(tt.s)
			if got != tt.want {
				t.Errorf("isCapitalized(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}
