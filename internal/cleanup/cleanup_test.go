package cleanup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// createMockRun creates a directory with the given timestamp-based name.
func createMockRun(t *testing.T, runsDir string, ts time.Time) string {
	t.Helper()
	name := ts.Format(runTimestampLayout)
	path := filepath.Join(runsDir, name)
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("creating mock run %s: %v", name, err)
	}
	return name
}

func TestPruneByAge_RemovesOldRuns(t *testing.T) {
	runsDir := t.TempDir()

	now := time.Now()
	old := createMockRun(t, runsDir, now.AddDate(0, 0, -60))
	recent := createMockRun(t, runsDir, now.AddDate(0, 0, -5))

	pruned, err := PruneByAge(runsDir, 30, false)
	if err != nil {
		t.Fatalf("PruneByAge failed: %v", err)
	}

	if len(pruned) != 1 || pruned[0] != old {
		t.Errorf("expected pruned=[%s], got %v", old, pruned)
	}

	// Old directory should be gone.
	if _, err := os.Stat(filepath.Join(runsDir, old)); !os.IsNotExist(err) {
		t.Errorf("expected %s to be deleted", old)
	}

	// Recent directory should still exist.
	if _, err := os.Stat(filepath.Join(runsDir, recent)); err != nil {
		t.Errorf("expected %s to still exist: %v", recent, err)
	}
}

func TestPruneByAge_DryRun(t *testing.T) {
	runsDir := t.TempDir()

	now := time.Now()
	old := createMockRun(t, runsDir, now.AddDate(0, 0, -60))

	pruned, err := PruneByAge(runsDir, 30, true)
	if err != nil {
		t.Fatalf("PruneByAge dry-run failed: %v", err)
	}

	if len(pruned) != 1 || pruned[0] != old {
		t.Errorf("expected pruned=[%s], got %v", old, pruned)
	}

	// Directory should still exist in dry-run mode.
	if _, err := os.Stat(filepath.Join(runsDir, old)); err != nil {
		t.Errorf("expected %s to still exist in dry-run: %v", old, err)
	}
}

func TestPruneByAge_SkipsNonTimestampDirs(t *testing.T) {
	runsDir := t.TempDir()

	// Create a non-timestamp directory.
	if err := os.MkdirAll(filepath.Join(runsDir, "not-a-timestamp"), 0755); err != nil {
		t.Fatalf("creating mock dir: %v", err)
	}

	pruned, err := PruneByAge(runsDir, 1, false)
	if err != nil {
		t.Fatalf("PruneByAge failed: %v", err)
	}

	if len(pruned) != 0 {
		t.Errorf("expected no pruned dirs, got %v", pruned)
	}
}

func TestPruneByAge_NonexistentDir(t *testing.T) {
	pruned, err := PruneByAge("/nonexistent/path", 30, false)
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got: %v", err)
	}
	if len(pruned) != 0 {
		t.Errorf("expected empty pruned list, got %v", pruned)
	}
}

func TestPruneKeepRecent_KeepsCorrectCount(t *testing.T) {
	runsDir := t.TempDir()

	now := time.Now()
	d1 := createMockRun(t, runsDir, now.AddDate(0, 0, -4))
	d2 := createMockRun(t, runsDir, now.AddDate(0, 0, -3))
	_ = createMockRun(t, runsDir, now.AddDate(0, 0, -2))
	_ = createMockRun(t, runsDir, now.AddDate(0, 0, -1))

	pruned, err := PruneKeepRecent(runsDir, 2, false)
	if err != nil {
		t.Fatalf("PruneKeepRecent failed: %v", err)
	}

	if len(pruned) != 2 {
		t.Fatalf("expected 2 pruned, got %d: %v", len(pruned), pruned)
	}

	// The two oldest should be removed.
	if pruned[0] != d1 || pruned[1] != d2 {
		t.Errorf("expected pruned=[%s, %s], got %v", d1, d2, pruned)
	}

	// Check filesystem state.
	entries, _ := os.ReadDir(runsDir)
	if len(entries) != 2 {
		t.Errorf("expected 2 remaining dirs, got %d", len(entries))
	}
}

func TestPruneKeepRecent_KeepMoreThanExist(t *testing.T) {
	runsDir := t.TempDir()

	now := time.Now()
	createMockRun(t, runsDir, now.AddDate(0, 0, -1))

	pruned, err := PruneKeepRecent(runsDir, 5, false)
	if err != nil {
		t.Fatalf("PruneKeepRecent failed: %v", err)
	}

	if len(pruned) != 0 {
		t.Errorf("expected no pruned dirs, got %v", pruned)
	}
}

func TestPruneKeepRecent_DryRun(t *testing.T) {
	runsDir := t.TempDir()

	now := time.Now()
	d1 := createMockRun(t, runsDir, now.AddDate(0, 0, -3))
	createMockRun(t, runsDir, now.AddDate(0, 0, -1))

	pruned, err := PruneKeepRecent(runsDir, 1, true)
	if err != nil {
		t.Fatalf("PruneKeepRecent dry-run failed: %v", err)
	}

	if len(pruned) != 1 || pruned[0] != d1 {
		t.Errorf("expected pruned=[%s], got %v", d1, pruned)
	}

	// Both should still exist in dry-run.
	entries, _ := os.ReadDir(runsDir)
	if len(entries) != 2 {
		t.Errorf("expected 2 dirs to remain in dry-run, got %d", len(entries))
	}
}

func TestPruneKeepRecent_NonexistentDir(t *testing.T) {
	pruned, err := PruneKeepRecent("/nonexistent/path", 5, false)
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got: %v", err)
	}
	if len(pruned) != 0 {
		t.Errorf("expected empty pruned list, got %v", pruned)
	}
}
