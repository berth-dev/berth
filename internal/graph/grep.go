// grep.go provides a grep-based fallback for non-TypeScript projects
// where the full Knowledge Graph MCP is not available.
package graph

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Match represents a grep search result.
type Match struct {
	File    string
	Line    int
	Content string
}

// Symbol represents a detected code symbol.
type Symbol struct {
	Name string
	Kind string // "function" | "type" | "class"
	File string
	Line int
}

// Import represents a detected import statement.
type Import struct {
	SourceFile string
	TargetPath string
	Names      []string
}

// rgMessage represents a single JSON message from ripgrep's --json output.
type rgMessage struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
		Submatches []struct {
			Match struct {
				Text string `json:"text"`
			} `json:"match"`
		} `json:"submatches"`
	} `json:"data"`
}

// GrepFallback runs ripgrep with the given pattern and returns matching lines.
// Returns an error if rg is not installed.
func GrepFallback(dir, pattern string) ([]Match, error) {
	rgPath, err := exec.LookPath("rg")
	if err != nil {
		return nil, fmt.Errorf("graph: ripgrep (rg) not found in PATH: %w", err)
	}

	cmd := exec.Command(rgPath, "--json", pattern, dir)
	output, err := cmd.Output()
	if err != nil {
		// Exit code 1 means no matches, which is not an error.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("graph: running ripgrep: %w", err)
	}

	return parseRgOutput(output)
}

// parseRgOutput parses ripgrep JSON output into Match slices.
func parseRgOutput(output []byte) ([]Match, error) {
	var matches []Match

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}

		var msg rgMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue // Skip malformed lines.
		}

		if msg.Type != "match" {
			continue
		}

		matches = append(matches, Match{
			File:    msg.Data.Path.Text,
			Line:    msg.Data.LineNumber,
			Content: strings.TrimRight(msg.Data.Lines.Text, "\n"),
		})
	}

	return matches, nil
}

// GrepFunctions searches for function definitions in the given language.
func GrepFunctions(dir, lang string) ([]Symbol, error) {
	patterns := funcPatterns(lang)
	if len(patterns) == 0 {
		return nil, fmt.Errorf("graph: unsupported language for function grep: %s", lang)
	}

	var all []Symbol
	for _, p := range patterns {
		matches, err := grepWithPattern(dir, p.pattern, p.globs)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			name := extractName(m.Content, p.pattern)
			if name == "" {
				continue
			}
			all = append(all, Symbol{
				Name: name,
				Kind: "function",
				File: m.File,
				Line: m.Line,
			})
		}
	}

	return all, nil
}

// GrepImports searches for import statements in the given language.
func GrepImports(dir, lang string) ([]Import, error) {
	patterns := importPatterns(lang)
	if len(patterns) == 0 {
		return nil, fmt.Errorf("graph: unsupported language for import grep: %s", lang)
	}

	var all []Import
	for _, p := range patterns {
		matches, err := grepWithPattern(dir, p.pattern, p.globs)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			imp := parseImportLine(m.Content, lang)
			if imp.TargetPath == "" {
				continue
			}
			imp.SourceFile = m.File
			all = append(all, imp)
		}
	}

	return all, nil
}

// GrepTypes searches for type definitions in the given language.
func GrepTypes(dir, lang string) ([]Symbol, error) {
	patterns := typePatterns(lang)
	if len(patterns) == 0 {
		return nil, fmt.Errorf("graph: unsupported language for type grep: %s", lang)
	}

	var all []Symbol
	for _, p := range patterns {
		matches, err := grepWithPattern(dir, p.pattern, p.globs)
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			name := extractName(m.Content, p.pattern)
			if name == "" {
				continue
			}
			all = append(all, Symbol{
				Name: name,
				Kind: p.kind,
				File: m.File,
				Line: m.Line,
			})
		}
	}

	return all, nil
}

// langPattern holds a regex pattern with associated file globs and symbol kind.
type langPattern struct {
	pattern string
	globs   []string
	kind    string // Used for type detection: "type", "class", etc.
}

// funcPatterns returns language-specific regex patterns for function definitions.
func funcPatterns(lang string) []langPattern {
	switch lang {
	case "go":
		return []langPattern{
			{pattern: `^func\s+(\w+)`, globs: []string{"*.go"}},
			{pattern: `^func\s+\([^)]+\)\s+(\w+)`, globs: []string{"*.go"}},
		}
	case "python":
		return []langPattern{
			{pattern: `^def\s+(\w+)`, globs: []string{"*.py"}},
			{pattern: `^class\s+(\w+)`, globs: []string{"*.py"}},
		}
	case "rust":
		return []langPattern{
			{pattern: `^pub\s+fn\s+(\w+)`, globs: []string{"*.rs"}},
			{pattern: `^fn\s+(\w+)`, globs: []string{"*.rs"}},
		}
	case "java":
		return []langPattern{
			{pattern: `(public|private|protected)\s+\w+\s+(\w+)\s*\(`, globs: []string{"*.java"}},
		}
	default:
		return nil
	}
}

// importPatterns returns language-specific regex patterns for import statements.
func importPatterns(lang string) []langPattern {
	switch lang {
	case "go":
		return []langPattern{
			{pattern: `^import\s+`, globs: []string{"*.go"}},
		}
	case "python":
		return []langPattern{
			{pattern: `^import\s+`, globs: []string{"*.py"}},
			{pattern: `^from\s+`, globs: []string{"*.py"}},
		}
	case "rust":
		return []langPattern{
			{pattern: `^use\s+`, globs: []string{"*.rs"}},
		}
	case "java":
		return []langPattern{
			{pattern: `^import\s+`, globs: []string{"*.java"}},
		}
	default:
		return nil
	}
}

// typePatterns returns language-specific regex patterns for type definitions.
func typePatterns(lang string) []langPattern {
	switch lang {
	case "go":
		return []langPattern{
			{pattern: `^type\s+(\w+)\s+struct`, globs: []string{"*.go"}, kind: "type"},
			{pattern: `^type\s+(\w+)\s+interface`, globs: []string{"*.go"}, kind: "type"},
		}
	case "python":
		return []langPattern{
			{pattern: `^class\s+(\w+)`, globs: []string{"*.py"}, kind: "class"},
		}
	case "rust":
		return []langPattern{
			{pattern: `^(pub\s+)?struct\s+(\w+)`, globs: []string{"*.rs"}, kind: "type"},
			{pattern: `^(pub\s+)?enum\s+(\w+)`, globs: []string{"*.rs"}, kind: "type"},
		}
	case "java":
		return []langPattern{
			{pattern: `(public\s+)?class\s+(\w+)`, globs: []string{"*.java"}, kind: "class"},
		}
	default:
		return nil
	}
}

// grepWithPattern runs ripgrep with the given pattern and file globs.
func grepWithPattern(dir, pattern string, globs []string) ([]Match, error) {
	rgPath, err := exec.LookPath("rg")
	if err != nil {
		return nil, fmt.Errorf("graph: ripgrep (rg) not found in PATH: %w", err)
	}

	args := []string{"--json", pattern}
	for _, g := range globs {
		args = append(args, "--glob", g)
	}
	args = append(args, dir)

	cmd := exec.Command(rgPath, args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("graph: running ripgrep: %w", err)
	}

	return parseRgOutput(output)
}

// extractName extracts the first captured group name from a line of code.
// This is a simplified extraction that looks for the identifier after the
// keyword pattern. It uses simple string parsing rather than full regex
// to avoid importing regexp.
func extractName(content, pattern string) string {
	// Trim whitespace.
	content = strings.TrimSpace(content)

	// Simple extraction strategies based on common patterns.
	// For patterns like "^func\s+(\w+)", extract word after "func ".
	keywords := []string{"func ", "def ", "class ", "fn ", "struct ", "enum ", "type ", "interface "}
	for _, kw := range keywords {
		if idx := strings.Index(content, kw); idx >= 0 {
			rest := strings.TrimSpace(content[idx+len(kw):])
			// Skip receiver in Go methods: "(receiver) Name"
			if strings.HasPrefix(rest, "(") {
				closeIdx := strings.Index(rest, ")")
				if closeIdx >= 0 {
					rest = strings.TrimSpace(rest[closeIdx+1:])
				}
			}
			return extractIdentifier(rest)
		}
	}

	// For Java-style patterns: "access_modifier return_type name("
	if idx := strings.Index(content, "("); idx > 0 {
		before := strings.TrimSpace(content[:idx])
		parts := strings.Fields(before)
		if len(parts) >= 2 {
			return parts[len(parts)-1]
		}
	}

	return ""
}

// extractIdentifier extracts a valid identifier (word characters) from the
// start of the string.
func extractIdentifier(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			break
		}
	}
	return b.String()
}

// parseImportLine extracts import information from a line of code.
func parseImportLine(content, lang string) Import {
	content = strings.TrimSpace(content)

	switch lang {
	case "go":
		return parseGoImport(content)
	case "python":
		return parsePythonImport(content)
	case "rust":
		return parseRustImport(content)
	case "java":
		return parseJavaImport(content)
	default:
		return Import{}
	}
}

// parseGoImport extracts import path from a Go import line.
func parseGoImport(line string) Import {
	// Handle: import "path" or import alias "path"
	line = strings.TrimPrefix(line, "import")
	line = strings.TrimSpace(line)

	// Strip parentheses for block imports.
	line = strings.Trim(line, "()")
	line = strings.TrimSpace(line)

	// Extract quoted path.
	target := extractQuoted(line, '"')
	if target == "" {
		return Import{}
	}

	return Import{TargetPath: target}
}

// parsePythonImport extracts import path from a Python import line.
func parsePythonImport(line string) Import {
	if strings.HasPrefix(line, "from ") {
		// "from module import name1, name2"
		rest := strings.TrimPrefix(line, "from ")
		parts := strings.SplitN(rest, " import ", 2)
		if len(parts) != 2 {
			return Import{}
		}
		target := strings.TrimSpace(parts[0])
		namesStr := strings.TrimSpace(parts[1])
		names := splitAndTrim(namesStr, ",")
		return Import{TargetPath: target, Names: names}
	}

	if strings.HasPrefix(line, "import ") {
		rest := strings.TrimPrefix(line, "import ")
		target := strings.TrimSpace(rest)
		// Handle "import a, b"
		targets := splitAndTrim(target, ",")
		if len(targets) > 0 {
			return Import{TargetPath: targets[0]}
		}
	}

	return Import{}
}

// parseRustImport extracts import path from a Rust use statement.
func parseRustImport(line string) Import {
	// "use std::collections::HashMap;"
	rest := strings.TrimPrefix(line, "use ")
	rest = strings.TrimSuffix(strings.TrimSpace(rest), ";")
	return Import{TargetPath: rest}
}

// parseJavaImport extracts import path from a Java import statement.
func parseJavaImport(line string) Import {
	// "import java.util.List;"
	rest := strings.TrimPrefix(line, "import ")
	rest = strings.TrimPrefix(rest, "static ")
	rest = strings.TrimSuffix(strings.TrimSpace(rest), ";")
	return Import{TargetPath: rest}
}

// extractQuoted extracts the content between the first pair of the given
// quote character.
func extractQuoted(s string, quote byte) string {
	start := strings.IndexByte(s, quote)
	if start < 0 {
		return ""
	}
	end := strings.IndexByte(s[start+1:], quote)
	if end < 0 {
		return ""
	}
	return s[start+1 : start+1+end]
}

// splitAndTrim splits a string by sep and trims whitespace from each part.
func splitAndTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

