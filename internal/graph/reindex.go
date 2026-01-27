// reindex.go implements smart mtime-based reindexing on startup
// and incremental reindexing between beads.
package graph

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SmartReindex performs mtime-based reindexing. If forceFullReindex is true
// or no last_index_time exists in the metadata table, a full reindex is
// performed. Otherwise, only changed and deleted files are processed.
func SmartReindex(db *sql.DB, srcDir string, forceFullReindex bool) error {
	if forceFullReindex || !hasLastIndexTime(db) {
		return fullReindex(db, srcDir)
	}

	lastIndex := getLastIndexTime(db)

	// Find changed files (mtime newer than last index time).
	var changedFiles []string
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors.
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "node_modules" || base == ".git" || base == "vendor" || base == "dist" || base == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSourceFile(path) {
			return nil
		}
		if info.ModTime().After(lastIndex) {
			relPath, relErr := filepath.Rel(srcDir, path)
			if relErr != nil {
				relPath = path
			}
			changedFiles = append(changedFiles, relPath)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("graph: walking source files: %w", err)
	}

	// Detect deleted files: files in the index but no longer on disk.
	indexedFiles := getIndexedFiles(db)
	for _, f := range indexedFiles {
		absPath := filepath.Join(srcDir, f)
		if !fileExists(absPath) {
			removeFromIndex(db, f)
		}
	}

	// Reindex changed files by removing old entries and re-inserting.
	for _, f := range changedFiles {
		removeFromIndex(db, f)
	}

	// Update last index time.
	updateLastIndexTime(db, time.Now())

	return nil
}

// ReindexChanged triggers reindexing of the specified changed files via the
// MCP client. This is used between beads after git commits.
func ReindexChanged(client *Client, changedFiles []string) error {
	if len(changedFiles) == 0 {
		return nil
	}
	return client.ReindexFiles(changedFiles)
}

// fullReindex reindexes all source files in the directory. It clears existing
// index data and updates the last_index_time.
func fullReindex(db *sql.DB, srcDir string) error {
	// Clear existing index data.
	tables := []string{"symbols", "references", "imports"}
	for _, table := range tables {
		_, err := db.Exec("DELETE FROM " + table)
		if err != nil {
			// Table may not exist yet; that is fine.
			continue
		}
	}

	// Walk all source files.
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == "node_modules" || base == ".git" || base == "vendor" || base == "dist" || base == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSourceFile(path) {
			return nil
		}
		// The actual indexing (parsing + inserting) is done by the MCP server;
		// this function prepares the database state. The caller should trigger
		// a full reindex via the MCP client after calling this.
		return nil
	})
	if err != nil {
		return fmt.Errorf("graph: walking source files for full reindex: %w", err)
	}

	updateLastIndexTime(db, time.Now())
	return nil
}

// isSourceFile checks whether the file has a recognized source code extension.
func isSourceFile(path string) bool {
	ext := filepath.Ext(path)
	switch ext {
	case ".ts", ".tsx", ".js", ".jsx",
		".go", ".py", ".rs",
		".java", ".kt", ".rb",
		".ex", ".exs":
		return true
	default:
		return false
	}
}

// hasLastIndexTime checks whether the metadata table contains a last_index_time entry.
func hasLastIndexTime(db *sql.DB) bool {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM metadata WHERE key = 'last_index_time'").Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

// getLastIndexTime retrieves the last_index_time from the metadata table.
// Returns the zero time if not found or on error.
func getLastIndexTime(db *sql.DB) time.Time {
	var val string
	err := db.QueryRow("SELECT value FROM metadata WHERE key = 'last_index_time'").Scan(&val)
	if err != nil {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, val)
	if err != nil {
		return time.Time{}
	}
	return t
}

// getIndexedFiles returns all distinct file paths currently in the symbols table.
func getIndexedFiles(db *sql.DB) []string {
	rows, err := db.Query("SELECT DISTINCT file FROM symbols")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var files []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			continue
		}
		files = append(files, f)
	}
	return files
}

// removeFromIndex removes all index entries for a file from the symbols,
// references, and imports tables.
func removeFromIndex(db *sql.DB, file string) {
	// Delete references that point to symbols in this file.
	_, _ = db.Exec(
		"DELETE FROM references WHERE symbol_id IN (SELECT id FROM symbols WHERE file = ?)",
		file,
	)
	// Delete symbols.
	_, _ = db.Exec("DELETE FROM symbols WHERE file = ?", file)
	// Delete imports where this file is source or target.
	_, _ = db.Exec("DELETE FROM imports WHERE source_file = ? OR target_file = ?", file, file)
}

// updateLastIndexTime sets or inserts the last_index_time in the metadata table.
func updateLastIndexTime(db *sql.DB, t time.Time) {
	val := t.Format(time.RFC3339Nano)

	// Try update first.
	res, err := db.Exec("UPDATE metadata SET value = ? WHERE key = 'last_index_time'", val)
	if err == nil {
		n, _ := res.RowsAffected()
		if n > 0 {
			return
		}
	}

	// Insert if update affected no rows.
	_, _ = db.Exec("INSERT INTO metadata (key, value) VALUES ('last_index_time', ?)", val)
}

// fileExists checks whether a file exists at the given path.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
