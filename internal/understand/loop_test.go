package understand

import "testing"

func TestCleanJSONOutput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain JSON",
			input: `{"done": true, "requirements_md": "# Title"}`,
			want:  `{"done": true, "requirements_md": "# Title"}`,
		},
		{
			name:  "JSON with leading text",
			input: "Here is the response:\n" + `{"done": true}`,
			want:  `{"done": true}`,
		},
		{
			name:  "JSON with trailing text",
			input: `{"done": true}` + "\nThat's all!",
			want:  `{"done": true}`,
		},
		{
			name: "JSON with embedded code fences in requirements_md",
			input: `{"done": true, "requirements_md": "# Example\n\n` + "```" + `go\nfunc main() {}\n` + "```" + `\n\nMore text."}`,
			want:  `{"done": true, "requirements_md": "# Example\n\n` + "```" + `go\nfunc main() {}\n` + "```" + `\n\nMore text."}`,
		},
		{
			name: "JSON with multiple embedded code fences",
			input: `{"done": true, "requirements_md": "` + "```" + `js\nconst x = {};\n` + "```" + `\n\n` + "```" + `py\ndef f(): pass\n` + "```" + `"}`,
			want:  `{"done": true, "requirements_md": "` + "```" + `js\nconst x = {};\n` + "```" + `\n\n` + "```" + `py\ndef f(): pass\n` + "```" + `"}`,
		},
		{
			name:  "JSON with nested braces in string",
			input: `{"done": true, "requirements_md": "Use {placeholder} syntax"}`,
			want:  `{"done": true, "requirements_md": "Use {placeholder} syntax"}`,
		},
		{
			name:  "no JSON object",
			input: "Just some text",
			want:  "Just some text",
		},
		{
			name:  "whitespace around JSON",
			input: "  \n  " + `{"done": false}` + "  \n  ",
			want:  `{"done": false}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanJSONOutput(tt.input)
			if got != tt.want {
				t.Errorf("cleanJSONOutput() = %q, want %q", got, tt.want)
			}
		})
	}
}
