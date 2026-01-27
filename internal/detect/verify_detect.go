// verify_detect.go auto-detects test, build, and lint commands for the target project.
package detect

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// DetectVerifyPipeline returns the verification pipeline commands for the given stack.
// Returns nil if no commands could be detected.
func DetectVerifyPipeline(dir string, stack StackInfo) []string {
	switch stack.Language {
	case "typescript", "javascript":
		return verifyJS(dir, stack)
	case "go":
		return verifyGo(dir)
	case "python":
		return verifyPython(dir)
	case "rust":
		return verifyRust()
	case "java", "kotlin":
		return verifyJava(stack)
	default:
		return nil
	}
}

// verifyJS reads package.json scripts and returns the ones that exist,
// prefixed with the detected package manager.
func verifyJS(dir string, stack StackInfo) []string {
	pkgPath := filepath.Join(dir, "package.json")
	data := readFile(pkgPath)
	if data == "" {
		return nil
	}

	var pkg packageJSON
	if err := json.Unmarshal([]byte(data), &pkg); err != nil {
		return nil
	}

	pm := stack.PackageManager
	if pm == "" {
		pm = "npm"
	}

	// Check for these scripts in order.
	scriptNames := []string{"typecheck", "lint", "test", "build"}
	var cmds []string
	for _, name := range scriptNames {
		if _, ok := pkg.Scripts[name]; ok {
			cmds = append(cmds, pm+" run "+name)
		}
	}
	return cmds
}

// verifyGo returns the Go verification pipeline.
func verifyGo(dir string) []string {
	var cmds []string

	if fileExists(filepath.Join(dir, ".golangci.yml")) ||
		fileExists(filepath.Join(dir, ".golangci.yaml")) {
		cmds = append(cmds, "golangci-lint run")
	}

	cmds = append(cmds, "go vet ./...", "go test ./...")
	return cmds
}

// verifyPython returns the Python verification pipeline.
func verifyPython(dir string) []string {
	var cmds []string

	hasRuff := fileExists(filepath.Join(dir, "ruff.toml"))
	if !hasRuff {
		pyproject := filepath.Join(dir, "pyproject.toml")
		if fileExists(pyproject) {
			content := readFile(pyproject)
			if strings.Contains(content, "[tool.ruff]") {
				hasRuff = true
			}
		}
	}
	if hasRuff {
		cmds = append(cmds, "ruff check .")
	}

	cmds = append(cmds, "pytest")
	return cmds
}

// verifyRust returns the Rust verification pipeline.
func verifyRust() []string {
	return []string{"cargo clippy", "cargo test", "cargo build"}
}

// verifyJava returns the Java/Kotlin verification pipeline.
func verifyJava(stack StackInfo) []string {
	if stack.PackageManager == "gradle" {
		return []string{"gradle test"}
	}
	return []string{"mvn test"}
}
