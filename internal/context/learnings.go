// learnings.go reads and appends to the learnings.md knowledge file.
package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// learningsFile is the path relative to .berth/ for accumulated learnings.
const learningsFile = "learnings.md"

// ReadLearnings reads .berth/learnings.md from the given directory and returns
// each learning entry as a string. Each line starting with "- " is treated as
// a learning entry (the "- " prefix is stripped). Returns an empty slice if
// the file does not exist.
func ReadLearnings(dir string) []string {
	path := filepath.Join(dir, ".berth", learningsFile)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	lines := strings.Split(string(data), "\n")
	var learnings []string
	for _, line := range lines {
		if strings.HasPrefix(line, "- ") {
			learnings = append(learnings, strings.TrimPrefix(line, "- "))
		}
	}

	return learnings
}

// AppendLearning appends a new learning entry to .berth/learnings.md in the
// given directory. Creates the file and .berth/ directory if they do not exist.
func AppendLearning(dir string, learning string) error {
	berthDir := filepath.Join(dir, ".berth")
	if err := os.MkdirAll(berthDir, 0755); err != nil {
		return fmt.Errorf("creating .berth directory: %w", err)
	}

	path := filepath.Join(berthDir, learningsFile)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening learnings file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing learnings file: %w", cerr)
		}
	}()

	if _, err := fmt.Fprintf(f, "- %s\n", learning); err != nil {
		return fmt.Errorf("writing learning: %w", err)
	}

	return nil
}
