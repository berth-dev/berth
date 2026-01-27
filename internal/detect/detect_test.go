package detect

import (
	"testing"

	"github.com/berth-dev/berth/internal/testutil"
)

func TestHasExistingCode_Brownfield(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
	}{
		{"go project", testutil.GoProject()},
		{"typescript project", testutil.TypeScriptProject()},
		{"python project", testutil.PythonProject()},
		{"rust project", testutil.RustProject()},
		{"java project", testutil.JavaMavenProject()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := testutil.TempProject(t, tt.files)
			if !HasExistingCode(dir) {
				t.Errorf("HasExistingCode() = false, want true for %s", tt.name)
			}
		})
	}
}

func TestHasExistingCode_Greenfield(t *testing.T) {
	dir := testutil.TempProject(t, testutil.EmptyProject())
	if HasExistingCode(dir) {
		t.Error("HasExistingCode() = true, want false for empty directory")
	}
}

func TestDetectStack_TypeScript(t *testing.T) {
	dir := testutil.TempProject(t, testutil.TypeScriptProject())
	info := DetectStack(dir)

	if info.Language != "typescript" {
		t.Errorf("Language = %q, want %q", info.Language, "typescript")
	}
	if info.Framework != "react" {
		t.Errorf("Framework = %q, want %q", info.Framework, "react")
	}
	if info.PackageManager != "pnpm" {
		t.Errorf("PackageManager = %q, want %q", info.PackageManager, "pnpm")
	}
}

func TestDetectStack_TypeScriptNext(t *testing.T) {
	dir := testutil.TempProject(t, testutil.NextJSProject())
	info := DetectStack(dir)

	if info.Language != "typescript" {
		t.Errorf("Language = %q, want %q", info.Language, "typescript")
	}
	if info.Framework != "next" {
		t.Errorf("Framework = %q, want %q", info.Framework, "next")
	}
	if info.PackageManager != "yarn" {
		t.Errorf("PackageManager = %q, want %q", info.PackageManager, "yarn")
	}
}

func TestDetectStack_WXT(t *testing.T) {
	dir := testutil.TempProject(t, testutil.WxtProject())
	info := DetectStack(dir)

	if info.Language != "typescript" {
		t.Errorf("Language = %q, want %q", info.Language, "typescript")
	}
	if info.Framework != "wxt" {
		t.Errorf("Framework = %q, want %q", info.Framework, "wxt")
	}
}

func TestDetectStack_Go(t *testing.T) {
	dir := testutil.TempProject(t, testutil.GoProject())
	info := DetectStack(dir)

	if info.Language != "go" {
		t.Errorf("Language = %q, want %q", info.Language, "go")
	}
	if info.Framework != "stdlib" {
		t.Errorf("Framework = %q, want %q", info.Framework, "stdlib")
	}
	if info.PackageManager != "go" {
		t.Errorf("PackageManager = %q, want %q", info.PackageManager, "go")
	}
	if info.TestCmd != "go test ./..." {
		t.Errorf("TestCmd = %q, want %q", info.TestCmd, "go test ./...")
	}
}

func TestDetectStack_GoGin(t *testing.T) {
	dir := testutil.TempProject(t, testutil.GoProjectWithGin())
	info := DetectStack(dir)

	if info.Framework != "gin" {
		t.Errorf("Framework = %q, want %q", info.Framework, "gin")
	}
}

func TestDetectStack_GoWithLinter(t *testing.T) {
	dir := testutil.TempProject(t, testutil.GoProjectWithLinter())
	info := DetectStack(dir)

	if info.LintCmd != "golangci-lint run" {
		t.Errorf("LintCmd = %q, want %q", info.LintCmd, "golangci-lint run")
	}
}

func TestDetectStack_Python(t *testing.T) {
	dir := testutil.TempProject(t, testutil.PythonProject())
	info := DetectStack(dir)

	if info.Language != "python" {
		t.Errorf("Language = %q, want %q", info.Language, "python")
	}
	if info.Framework != "flask" {
		t.Errorf("Framework = %q, want %q", info.Framework, "flask")
	}
	if info.LintCmd != "ruff check ." {
		t.Errorf("LintCmd = %q, want %q", info.LintCmd, "ruff check .")
	}
}

func TestDetectStack_PythonDjango(t *testing.T) {
	dir := testutil.TempProject(t, testutil.PythonDjangoProject())
	info := DetectStack(dir)

	if info.Framework != "django" {
		t.Errorf("Framework = %q, want %q", info.Framework, "django")
	}
}

func TestDetectStack_Rust(t *testing.T) {
	dir := testutil.TempProject(t, testutil.RustProject())
	info := DetectStack(dir)

	if info.Language != "rust" {
		t.Errorf("Language = %q, want %q", info.Language, "rust")
	}
	if info.Framework != "axum" {
		t.Errorf("Framework = %q, want %q", info.Framework, "axum")
	}
	if info.PackageManager != "cargo" {
		t.Errorf("PackageManager = %q, want %q", info.PackageManager, "cargo")
	}
}

func TestDetectStack_JavaMaven(t *testing.T) {
	dir := testutil.TempProject(t, testutil.JavaMavenProject())
	info := DetectStack(dir)

	if info.Language != "java" {
		t.Errorf("Language = %q, want %q", info.Language, "java")
	}
	if info.Framework != "spring-boot" {
		t.Errorf("Framework = %q, want %q", info.Framework, "spring-boot")
	}
	if info.PackageManager != "maven" {
		t.Errorf("PackageManager = %q, want %q", info.PackageManager, "maven")
	}
	if info.TestCmd != "mvn test" {
		t.Errorf("TestCmd = %q, want %q", info.TestCmd, "mvn test")
	}
}

func TestDetectStack_KotlinGradle(t *testing.T) {
	dir := testutil.TempProject(t, testutil.JavaGradleKtsProject())
	info := DetectStack(dir)

	if info.Language != "kotlin" {
		t.Errorf("Language = %q, want %q", info.Language, "kotlin")
	}
	if info.PackageManager != "gradle" {
		t.Errorf("PackageManager = %q, want %q", info.PackageManager, "gradle")
	}
}

func TestDetectStack_Greenfield(t *testing.T) {
	dir := testutil.TempProject(t, testutil.EmptyProject())
	info := DetectStack(dir)

	if info.Language != "" {
		t.Errorf("Language = %q, want empty for greenfield", info.Language)
	}
}

func TestDetectStack_JavaScriptNoTS(t *testing.T) {
	files := map[string]string{
		"package.json": `{"name":"test","dependencies":{"express":"^4.0.0"}}`,
	}
	dir := testutil.TempProject(t, files)
	info := DetectStack(dir)

	if info.Language != "javascript" {
		t.Errorf("Language = %q, want %q", info.Language, "javascript")
	}
	if info.Framework != "express" {
		t.Errorf("Framework = %q, want %q", info.Framework, "express")
	}
	if info.PackageManager != "npm" {
		t.Errorf("PackageManager = %q, want %q", info.PackageManager, "npm")
	}
}
