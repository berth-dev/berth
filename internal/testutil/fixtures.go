// Package testutil provides test helper utilities for berth tests.
package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TempProject creates a temporary directory with the given files and returns its path.
// Files is a map of relative path -> content. Directories are created as needed.
// The directory is automatically cleaned up when the test finishes.
func TempProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()

	for relPath, content := range files {
		absPath := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			t.Fatalf("creating directory for %s: %v", relPath, err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
			t.Fatalf("writing %s: %v", relPath, err)
		}
	}

	return dir
}

// TypeScriptProject returns file contents for a minimal TypeScript project.
func TypeScriptProject() map[string]string {
	pkg := map[string]interface{}{
		"name":    "test-project",
		"version": "1.0.0",
		"dependencies": map[string]string{
			"react": "^18.0.0",
		},
		"devDependencies": map[string]string{
			"typescript": "^5.0.0",
		},
		"scripts": map[string]string{
			"test":      "jest",
			"build":     "tsc",
			"lint":      "eslint .",
			"typecheck": "tsc --noEmit",
		},
	}
	pkgJSON, _ := json.MarshalIndent(pkg, "", "  ")

	return map[string]string{
		"package.json":    string(pkgJSON),
		"tsconfig.json":   `{"compilerOptions": {"strict": true}}`,
		"pnpm-lock.yaml":  "lockfileVersion: 5.4",
		"src/index.ts":    `export const main = () => console.log("hello");`,
	}
}

// GoProject returns file contents for a minimal Go project.
func GoProject() map[string]string {
	return map[string]string{
		"go.mod":  "module example.com/test\n\ngo 1.23\n",
		"main.go": "package main\n\nfunc main() {}\n",
	}
}

// GoProjectWithGin returns file contents for a Go project using Gin.
func GoProjectWithGin() map[string]string {
	return map[string]string{
		"go.mod":  "module example.com/test\n\ngo 1.23\n\nrequire github.com/gin-gonic/gin v1.9.0\n",
		"main.go": "package main\n\nfunc main() {}\n",
	}
}

// GoProjectWithLinter returns file contents for a Go project with golangci-lint.
func GoProjectWithLinter() map[string]string {
	return map[string]string{
		"go.mod":          "module example.com/test\n\ngo 1.23\n",
		"main.go":         "package main\n\nfunc main() {}\n",
		".golangci.yml":   "linters:\n  enable:\n    - errcheck\n",
	}
}

// PythonProject returns file contents for a minimal Python project.
func PythonProject() map[string]string {
	return map[string]string{
		"pyproject.toml":  "[tool.poetry]\nname = \"test\"\n\n[tool.ruff]\nline-length = 88\n",
		"requirements.txt": "flask>=2.0\n",
		"app.py":           "from flask import Flask\napp = Flask(__name__)\n",
	}
}

// PythonDjangoProject returns file contents for a Django project.
func PythonDjangoProject() map[string]string {
	return map[string]string{
		"requirements.txt": "django>=4.0\n",
		"manage.py":        "#!/usr/bin/env python\nimport django\n",
	}
}

// RustProject returns file contents for a minimal Rust project.
func RustProject() map[string]string {
	return map[string]string{
		"Cargo.toml": "[package]\nname = \"test\"\nversion = \"0.1.0\"\n\n[dependencies]\naxum = \"0.7\"\n",
		"src/main.rs": "fn main() {}\n",
	}
}

// JavaMavenProject returns file contents for a Maven-based Java project.
func JavaMavenProject() map[string]string {
	return map[string]string{
		"pom.xml": `<project>
  <modelVersion>4.0.0</modelVersion>
  <groupId>com.test</groupId>
  <artifactId>test</artifactId>
  <parent>
    <groupId>org.springframework.boot</groupId>
    <artifactId>spring-boot-starter-parent</artifactId>
  </parent>
</project>`,
		"src/main/java/App.java": "public class App { public static void main(String[] args) {} }",
	}
}

// JavaGradleKtsProject returns file contents for a Gradle Kotlin-based project.
func JavaGradleKtsProject() map[string]string {
	return map[string]string{
		"build.gradle.kts": `plugins {
    id("org.springframework.boot") version "3.0.0"
}`,
		"src/main/kotlin/App.kt": "fun main() {}",
	}
}

// NextJSProject returns file contents for a Next.js project.
func NextJSProject() map[string]string {
	pkg := map[string]interface{}{
		"name":    "next-app",
		"version": "1.0.0",
		"dependencies": map[string]string{
			"next":  "^14.0.0",
			"react": "^18.0.0",
		},
		"scripts": map[string]string{
			"build": "next build",
			"test":  "jest",
			"lint":  "next lint",
		},
	}
	pkgJSON, _ := json.MarshalIndent(pkg, "", "  ")

	return map[string]string{
		"package.json":       string(pkgJSON),
		"tsconfig.json":      `{}`,
		"yarn.lock":          "",
		"pages/index.tsx":    "export default function Home() { return <div />; }",
	}
}

// EmptyProject returns an empty directory with no files.
func EmptyProject() map[string]string {
	return map[string]string{}
}

// WxtProject returns file contents for a WXT browser extension project.
func WxtProject() map[string]string {
	pkg := map[string]interface{}{
		"name":    "wxt-ext",
		"version": "1.0.0",
		"dependencies": map[string]string{
			"react": "^18.0.0",
		},
		"devDependencies": map[string]string{
			"wxt":        "^0.18.0",
			"typescript": "^5.0.0",
		},
	}
	pkgJSON, _ := json.MarshalIndent(pkg, "", "  ")

	return map[string]string{
		"package.json":   string(pkgJSON),
		"tsconfig.json":  `{}`,
		"pnpm-lock.yaml": "lockfileVersion: 5.4",
	}
}
