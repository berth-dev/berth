package detect

import (
	"testing"

	"github.com/berth-dev/berth/internal/testutil"
)

func TestDetectVerifyPipeline_TypeScript(t *testing.T) {
	dir := testutil.TempProject(t, testutil.TypeScriptProject())
	stack := DetectStack(dir)
	pipeline := DetectVerifyPipeline(dir, stack)

	if len(pipeline) == 0 {
		t.Fatal("expected non-empty pipeline for TypeScript project")
	}

	// TypeScript project with typecheck, lint, test, build scripts should
	// produce pipeline entries for each.
	expected := map[string]bool{
		"pnpm run typecheck": true,
		"pnpm run lint":      true,
		"pnpm run test":      true,
		"pnpm run build":     true,
	}

	for _, cmd := range pipeline {
		if !expected[cmd] {
			t.Errorf("unexpected pipeline command: %q", cmd)
		}
		delete(expected, cmd)
	}

	for cmd := range expected {
		t.Errorf("missing expected pipeline command: %q", cmd)
	}
}

func TestDetectVerifyPipeline_Go(t *testing.T) {
	dir := testutil.TempProject(t, testutil.GoProject())
	stack := DetectStack(dir)
	pipeline := DetectVerifyPipeline(dir, stack)

	if len(pipeline) < 2 {
		t.Fatalf("expected at least 2 commands for Go project, got %d", len(pipeline))
	}

	// Should have go vet and go test
	hasVet := false
	hasTest := false
	for _, cmd := range pipeline {
		if cmd == "go vet ./..." {
			hasVet = true
		}
		if cmd == "go test ./..." {
			hasTest = true
		}
	}

	if !hasVet {
		t.Error("missing 'go vet ./...' in pipeline")
	}
	if !hasTest {
		t.Error("missing 'go test ./...' in pipeline")
	}
}

func TestDetectVerifyPipeline_GoWithLinter(t *testing.T) {
	dir := testutil.TempProject(t, testutil.GoProjectWithLinter())
	stack := DetectStack(dir)
	pipeline := DetectVerifyPipeline(dir, stack)

	hasLint := false
	for _, cmd := range pipeline {
		if cmd == "golangci-lint run" {
			hasLint = true
			break
		}
	}
	if !hasLint {
		t.Error("missing 'golangci-lint run' in pipeline for Go project with .golangci.yml")
	}
}

func TestDetectVerifyPipeline_Python(t *testing.T) {
	dir := testutil.TempProject(t, testutil.PythonProject())
	stack := DetectStack(dir)
	pipeline := DetectVerifyPipeline(dir, stack)

	if len(pipeline) == 0 {
		t.Fatal("expected non-empty pipeline for Python project")
	}

	hasRuff := false
	hasPytest := false
	for _, cmd := range pipeline {
		if cmd == "ruff check ." {
			hasRuff = true
		}
		if cmd == "pytest" {
			hasPytest = true
		}
	}
	if !hasRuff {
		t.Error("missing 'ruff check .' in pipeline for Python project with ruff config")
	}
	if !hasPytest {
		t.Error("missing 'pytest' in pipeline")
	}
}

func TestDetectVerifyPipeline_Rust(t *testing.T) {
	dir := testutil.TempProject(t, testutil.RustProject())
	stack := DetectStack(dir)
	pipeline := DetectVerifyPipeline(dir, stack)

	expected := []string{"cargo clippy", "cargo test", "cargo build"}
	if len(pipeline) != len(expected) {
		t.Fatalf("expected %d commands, got %d: %v", len(expected), len(pipeline), pipeline)
	}
	for i, cmd := range expected {
		if pipeline[i] != cmd {
			t.Errorf("pipeline[%d] = %q, want %q", i, pipeline[i], cmd)
		}
	}
}

func TestDetectVerifyPipeline_JavaMaven(t *testing.T) {
	dir := testutil.TempProject(t, testutil.JavaMavenProject())
	stack := DetectStack(dir)
	pipeline := DetectVerifyPipeline(dir, stack)

	if len(pipeline) != 1 || pipeline[0] != "mvn test" {
		t.Errorf("expected [mvn test], got %v", pipeline)
	}
}

func TestDetectVerifyPipeline_JavaGradle(t *testing.T) {
	dir := testutil.TempProject(t, testutil.JavaGradleKtsProject())
	stack := DetectStack(dir)
	pipeline := DetectVerifyPipeline(dir, stack)

	if len(pipeline) != 1 || pipeline[0] != "gradle test" {
		t.Errorf("expected [gradle test], got %v", pipeline)
	}
}

func TestDetectVerifyPipeline_Greenfield(t *testing.T) {
	dir := testutil.TempProject(t, testutil.EmptyProject())
	stack := DetectStack(dir)
	pipeline := DetectVerifyPipeline(dir, stack)

	if pipeline != nil {
		t.Errorf("expected nil pipeline for greenfield, got %v", pipeline)
	}
}
