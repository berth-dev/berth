// stacks.go contains language and framework detection rules.
package detect

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// stackRuleFunc examines dir and returns a StackInfo + true if its indicator
// files are present. Returns zero StackInfo + false otherwise.
type stackRuleFunc func(dir string) (StackInfo, bool)

// stackRules is evaluated in order; first match wins.
var stackRules = []stackRuleFunc{
	detectTypeScript,
	detectGo,
	detectPython,
	detectRust,
	detectJava,
}

// ---------------------------------------------------------------------------
// TypeScript / JavaScript
// ---------------------------------------------------------------------------

// packageJSON is the minimal structure we parse from package.json.
type packageJSON struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Scripts         map[string]string `json:"scripts"`
}

func detectTypeScript(dir string) (StackInfo, bool) {
	pkgPath := filepath.Join(dir, "package.json")
	if !fileExists(pkgPath) {
		return StackInfo{}, false
	}

	var pkg packageJSON
	data := readFile(pkgPath)
	if data != "" {
		_ = json.Unmarshal([]byte(data), &pkg)
	}

	// Language
	lang := "javascript"
	if fileExists(filepath.Join(dir, "tsconfig.json")) {
		lang = "typescript"
	}

	// Framework
	framework := detectJSFramework(pkg)

	// Package manager
	pm := detectJSPackageManager(dir)

	return StackInfo{
		Language:       lang,
		Framework:      framework,
		PackageManager: pm,
		TestCmd:        pm + " test",
		BuildCmd:       pm + " build",
		LintCmd:        pm + " lint",
	}, true
}

func detectJSFramework(pkg packageJSON) string {
	hasDep := func(name string) bool {
		if _, ok := pkg.Dependencies[name]; ok {
			return true
		}
		if _, ok := pkg.DevDependencies[name]; ok {
			return true
		}
		return false
	}
	hasMainDep := func(name string) bool {
		_, ok := pkg.Dependencies[name]
		return ok
	}

	switch {
	case hasDep("wxt"):
		return "wxt"
	case hasMainDep("next"):
		return "next"
	case hasMainDep("react"):
		return "react"
	case hasMainDep("vue"):
		return "vue"
	case hasMainDep("svelte"):
		return "svelte"
	case hasMainDep("express"):
		return "express"
	default:
		return "node"
	}
}

func detectJSPackageManager(dir string) string {
	switch {
	case fileExists(filepath.Join(dir, "pnpm-lock.yaml")):
		return "pnpm"
	case fileExists(filepath.Join(dir, "yarn.lock")):
		return "yarn"
	case fileExists(filepath.Join(dir, "bun.lockb")):
		return "bun"
	default:
		return "npm"
	}
}

// ---------------------------------------------------------------------------
// Go
// ---------------------------------------------------------------------------

func detectGo(dir string) (StackInfo, bool) {
	modPath := filepath.Join(dir, "go.mod")
	if !fileExists(modPath) {
		return StackInfo{}, false
	}

	content := readFile(modPath)
	framework := detectGoFramework(content)

	lintCmd := "go vet ./..."
	if fileExists(filepath.Join(dir, ".golangci.yml")) ||
		fileExists(filepath.Join(dir, ".golangci.yaml")) {
		lintCmd = "golangci-lint run"
	}

	return StackInfo{
		Language:       "go",
		Framework:      framework,
		PackageManager: "go",
		TestCmd:        "go test ./...",
		BuildCmd:       "go build ./...",
		LintCmd:        lintCmd,
	}, true
}

func detectGoFramework(modContent string) string {
	switch {
	case strings.Contains(modContent, "github.com/gin-gonic/gin"):
		return "gin"
	case strings.Contains(modContent, "github.com/labstack/echo"):
		return "echo"
	case strings.Contains(modContent, "github.com/gofiber/fiber"):
		return "fiber"
	case strings.Contains(modContent, "github.com/go-chi/chi"):
		return "chi"
	default:
		return "stdlib"
	}
}

// ---------------------------------------------------------------------------
// Python
// ---------------------------------------------------------------------------

func detectPython(dir string) (StackInfo, bool) {
	hasPyproject := fileExists(filepath.Join(dir, "pyproject.toml"))
	hasRequirements := fileExists(filepath.Join(dir, "requirements.txt"))
	hasSetupPy := fileExists(filepath.Join(dir, "setup.py"))

	if !hasPyproject && !hasRequirements && !hasSetupPy {
		return StackInfo{}, false
	}

	// Gather content for framework detection.
	var combined string
	if hasPyproject {
		combined += readFile(filepath.Join(dir, "pyproject.toml"))
	}
	if hasRequirements {
		combined += readFile(filepath.Join(dir, "requirements.txt"))
	}
	if hasSetupPy {
		combined += readFile(filepath.Join(dir, "setup.py"))
	}

	framework := detectPythonFramework(combined)
	pm := detectPythonPackageManager(dir, hasPyproject)

	lintCmd := "flake8"
	if fileExists(filepath.Join(dir, "ruff.toml")) ||
		(hasPyproject && strings.Contains(readFile(filepath.Join(dir, "pyproject.toml")), "[tool.ruff]")) {
		lintCmd = "ruff check ."
	}

	return StackInfo{
		Language:       "python",
		Framework:      framework,
		PackageManager: pm,
		TestCmd:        "pytest",
		BuildCmd:       "",
		LintCmd:        lintCmd,
	}, true
}

func detectPythonFramework(content string) string {
	lower := strings.ToLower(content)
	switch {
	case strings.Contains(lower, "django"):
		return "django"
	case strings.Contains(lower, "flask"):
		return "flask"
	case strings.Contains(lower, "fastapi"):
		return "fastapi"
	default:
		return "script"
	}
}

func detectPythonPackageManager(dir string, hasPyproject bool) string {
	if hasPyproject {
		content := readFile(filepath.Join(dir, "pyproject.toml"))
		if strings.Contains(content, "[tool.poetry]") {
			return "poetry"
		}
	}
	if fileExists(filepath.Join(dir, "uv.lock")) {
		return "uv"
	}
	return "pip"
}

// ---------------------------------------------------------------------------
// Rust
// ---------------------------------------------------------------------------

func detectRust(dir string) (StackInfo, bool) {
	cargoPath := filepath.Join(dir, "Cargo.toml")
	if !fileExists(cargoPath) {
		return StackInfo{}, false
	}

	content := readFile(cargoPath)
	framework := detectRustFramework(content)

	return StackInfo{
		Language:       "rust",
		Framework:      framework,
		PackageManager: "cargo",
		TestCmd:        "cargo test",
		BuildCmd:       "cargo build",
		LintCmd:        "cargo clippy",
	}, true
}

func detectRustFramework(content string) string {
	switch {
	case strings.Contains(content, "actix-web"):
		return "actix-web"
	case strings.Contains(content, "axum"):
		return "axum"
	case strings.Contains(content, "rocket"):
		return "rocket"
	default:
		return "lib"
	}
}

// ---------------------------------------------------------------------------
// Java / Kotlin
// ---------------------------------------------------------------------------

func detectJava(dir string) (StackInfo, bool) {
	hasPom := fileExists(filepath.Join(dir, "pom.xml"))
	hasGradle := fileExists(filepath.Join(dir, "build.gradle"))
	hasGradleKts := fileExists(filepath.Join(dir, "build.gradle.kts"))

	if !hasPom && !hasGradle && !hasGradleKts {
		return StackInfo{}, false
	}

	// Language
	lang := "java"
	if hasGradleKts {
		lang = "kotlin"
	}

	// Build system
	pm := "maven"
	if hasGradle || hasGradleKts {
		pm = "gradle"
	}

	// Framework detection
	var content string
	if hasPom {
		content = readFile(filepath.Join(dir, "pom.xml"))
	} else if hasGradleKts {
		content = readFile(filepath.Join(dir, "build.gradle.kts"))
	} else if hasGradle {
		content = readFile(filepath.Join(dir, "build.gradle"))
	}
	framework := detectJavaFramework(content)

	testCmd := "mvn test"
	buildCmd := "mvn package"
	if pm == "gradle" {
		testCmd = "gradle test"
		buildCmd = "gradle build"
	}

	return StackInfo{
		Language:       lang,
		Framework:      framework,
		PackageManager: pm,
		TestCmd:        testCmd,
		BuildCmd:       buildCmd,
		LintCmd:        "",
	}, true
}

func detectJavaFramework(content string) string {
	lower := strings.ToLower(content)
	switch {
	case strings.Contains(lower, "spring-boot"):
		return "spring-boot"
	case strings.Contains(lower, "quarkus"):
		return "quarkus"
	default:
		return "java"
	}
}
