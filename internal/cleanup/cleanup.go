// Package cleanup implements pruning of old berth run directories.
package cleanup

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// runTimestampLayout is the format used for run directory names.
const runTimestampLayout = "20060102-150405"

// PruneByAge removes run directories older than maxAgeDays.
// If dryRun is true, no directories are deleted; the function only returns
// the names that would be removed. Returns the list of pruned directory names.
func PruneByAge(runsDir string, maxAgeDays int, dryRun bool) ([]string, error) {
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading runs directory: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)
	var pruned []string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		t, parseErr := time.Parse(runTimestampLayout, entry.Name())
		if parseErr != nil {
			// Skip directories that don't match the timestamp format.
			continue
		}

		if t.Before(cutoff) {
			if !dryRun {
				path := filepath.Join(runsDir, entry.Name())
				if rmErr := os.RemoveAll(path); rmErr != nil {
					return pruned, fmt.Errorf("removing %s: %w", entry.Name(), rmErr)
				}
			}
			pruned = append(pruned, entry.Name())
		}
	}

	return pruned, nil
}

// PruneKeepRecent removes all run directories except the most recent keep
// directories. If dryRun is true, no directories are deleted. Returns the
// list of pruned directory names.
func PruneKeepRecent(runsDir string, keep int, dryRun bool) ([]string, error) {
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading runs directory: %w", err)
	}

	// Filter to only timestamp-named directories.
	var dirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, parseErr := time.Parse(runTimestampLayout, entry.Name()); parseErr == nil {
			dirs = append(dirs, entry.Name())
		}
	}

	// Sort chronologically (timestamp names sort lexicographically).
	sort.Strings(dirs)

	if len(dirs) <= keep {
		return nil, nil
	}

	toRemove := dirs[:len(dirs)-keep]
	var pruned []string

	for _, name := range toRemove {
		if !dryRun {
			path := filepath.Join(runsDir, name)
			if rmErr := os.RemoveAll(path); rmErr != nil {
				return pruned, fmt.Errorf("removing %s: %w", name, rmErr)
			}
		}
		pruned = append(pruned, name)
	}

	return pruned, nil
}
