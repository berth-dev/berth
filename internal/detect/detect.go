// Package detect handles brownfield/greenfield detection.
// This file provides HasExistingCode and DetectStack for analyzing project state.
package detect

import (
	"os"
	"path/filepath"
)

// StackInfo holds the detected project stack information.
type StackInfo struct {
	Language       string // "typescript", "python", "go", "rust", "java", etc.
	Framework      string // "wxt", "react", "next", "django", "flask", "gin", etc.
	PackageManager string // "pnpm", "npm", "yarn", "pip", "cargo", "go", etc.
	TestCmd        string // "pnpm test", "pytest", "go test ./...", etc.
	BuildCmd       string // "pnpm build", "go build ./...", etc.
	LintCmd        string // "pnpm lint", "golangci-lint run", etc.
}

// projectIndicators lists files whose presence signals an existing codebase.
var projectIndicators = []string{
	"package.json",
	"go.mod",
	"Cargo.toml",
	"pyproject.toml",
	"requirements.txt",
	"setup.py",
	"pom.xml",
	"build.gradle",
	"build.gradle.kts",
	"Makefile",
	"Gemfile",
	"composer.json",
	"mix.exs",
	"CMakeLists.txt",
}

// HasExistingCode checks whether dir contains an existing codebase (brownfield)
// or is empty/near-empty (greenfield).
// Returns true if dir contains any recognized project file.
func HasExistingCode(dir string) bool {
	for _, indicator := range projectIndicators {
		if fileExists(filepath.Join(dir, indicator)) {
			return true
		}
	}
	return false
}

// DetectStack scans dir for project files and returns detected stack info.
// Returns a zero StackInfo if nothing is detected (greenfield).
func DetectStack(dir string) StackInfo {
	for _, rule := range stackRules {
		if info, ok := rule(dir); ok {
			return info
		}
	}
	return StackInfo{}
}

// fileExists returns true if path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// readFile reads the file at path and returns its contents.
// Returns an empty string if the file cannot be read.
func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
